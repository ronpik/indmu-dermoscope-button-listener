// Dermoscope helper for Windows (approach C, single-click).
//
// Owns the HT-B30S dermoscope camera via DirectShow, serves an MJPEG live
// preview and a full-resolution still-capture endpoint over HTTP, and emits
// one F9 keystroke per hardware-button press via SendInput.
//
// Endpoints (default port 8080):
//   GET /          -> minimal HTML test page (preview + capture + F9 handler)
//   GET /preview   -> multipart/x-mixed-replace MJPEG stream (1600x1200 live)
//   GET /still     -> image/jpeg of the most recent button-triggered still
//
// Web-app integration contract:
//   - <img src="http://localhost:8080/preview"> for live video
//   - listen for F9 keydown; on receipt, fetch('/still') for the full-res JPEG
//
// DirectShow graph:
//   SourceFilter (HT-B30S)
//     ├── Capture pin (MJPG 1600x1200) -> SampleGrabber (PreviewCB) -> NullRenderer
//     │     -- feeds /preview (live) AND serves as the source of /still
//     └── Still pin   (MJPG 320x240)   -> SampleGrabber (StillCB)   -> NullRenderer
//           -- used ONLY as the hardware-button trigger; bytes are discarded.
//              On trigger, we snapshot the latest Capture-pin preview frame
//              into the /still buffer so /still is always a 1600x1200 JPEG.
//
// Why single-click only: the device has a USB alt-setting threshold between
// 320x240 and 640x480 on the Capture pin. Above 320x240 the bandwidth
// reservation crowds out Still-pin IRPs, so clicks 2/3 of a rapid burst are
// dropped by the driver before reaching us. Multi-click detection is
// mechanically unreliable on this hardware; clear/undo gestures live in the
// web app UI instead. Full investigation: docs/INVESTIGATION.md.

#define WIN32_LEAN_AND_MEAN
#define _WIN32_WINNT 0x0601
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <dshow.h>
#include <stdio.h>
#include <string.h>
#include <wchar.h>
#include <wctype.h>
#include <thread>
#include <mutex>
#include <condition_variable>
#include <vector>
#include <atomic>
#include <chrono>

#pragma comment(lib, "ws2_32.lib")

// Software debounce for the hardware button. The device's own firmware has
// a multi-second cooldown between stills at these settings, so this is
// defense-in-depth only against spurious rapid triggers. Override via argv[2].
static DWORD g_debounce_ms = 300;

// ---- DirectShow CLSIDs / IIDs not always in mingw headers ----
static const CLSID CLSID_SampleGrabber_local =
    { 0xC1F400A0, 0x3F08, 0x11D3, { 0x9F, 0x0B, 0x00, 0x60, 0x08, 0x03, 0x9E, 0x37 } };
static const CLSID CLSID_NullRenderer_local =
    { 0xC1F400A4, 0x3F08, 0x11D3, { 0x9F, 0x0B, 0x00, 0x60, 0x08, 0x03, 0x9E, 0x37 } };

struct ISampleGrabberCB_local : public IUnknown {
    virtual HRESULT STDMETHODCALLTYPE SampleCB(double SampleTime, IMediaSample *pSample) = 0;
    virtual HRESULT STDMETHODCALLTYPE BufferCB(double SampleTime, BYTE *pBuffer, long BufferLen) = 0;
};
static const IID IID_ISampleGrabberCB_local =
    { 0x0579154A, 0x2B53, 0x4994, { 0xB0, 0xD0, 0xE7, 0x73, 0x14, 0x8E, 0xFF, 0x85 } };

struct ISampleGrabber_local : public IUnknown {
    virtual HRESULT STDMETHODCALLTYPE SetOneShot(BOOL OneShot) = 0;
    virtual HRESULT STDMETHODCALLTYPE SetMediaType(const AM_MEDIA_TYPE *pType) = 0;
    virtual HRESULT STDMETHODCALLTYPE GetConnectedMediaType(AM_MEDIA_TYPE *pType) = 0;
    virtual HRESULT STDMETHODCALLTYPE SetBufferSamples(BOOL BufferThem) = 0;
    virtual HRESULT STDMETHODCALLTYPE GetCurrentBuffer(long *pBufferSize, long *pBuffer) = 0;
    virtual HRESULT STDMETHODCALLTYPE GetCurrentSample(IMediaSample **ppSample) = 0;
    virtual HRESULT STDMETHODCALLTYPE SetCallback(ISampleGrabberCB_local *pCallback, long WhichMethodToCallback) = 0;
};
static const IID IID_ISampleGrabber_local =
    { 0x6B652FFF, 0x11FE, 0x4FCE, { 0x92, 0xAD, 0x02, 0x66, 0xB5, 0xD7, 0xC7, 0x8F } };

// ---- Shared state ----
static std::mutex g_previewMutex;
static std::condition_variable g_previewCV;
static std::vector<BYTE> g_latestPreview;
static std::atomic<long long> g_previewSeq{0};

static std::mutex g_stillMutex;
static std::vector<BYTE> g_latestStill;
static std::atomic<long long> g_stillSeq{0};

static std::atomic<bool> g_running{true};

static void log_ts(const char *fmt, ...) {
    SYSTEMTIME st; GetLocalTime(&st);
    char ts[16];
    snprintf(ts, sizeof(ts), "%02d:%02d:%02d.%03d",
             st.wHour, st.wMinute, st.wSecond, st.wMilliseconds);
    fprintf(stderr, "[%s] ", ts);
    va_list ap; va_start(ap, fmt);
    vfprintf(stderr, fmt, ap);
    va_end(ap);
    fprintf(stderr, "\n");
    fflush(stderr);
}

// ---- Sample Grabber callbacks ----
class PreviewCB : public ISampleGrabberCB_local {
public:
    LONG ref = 1;
    LONG frame_count = 0;
    static const LONG FRAME_SKIP = 2;
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID riid, void **ppv) override {
        if (!ppv) return E_POINTER;
        if (riid == IID_IUnknown || riid == IID_ISampleGrabberCB_local) {
            *ppv = static_cast<ISampleGrabberCB_local*>(this); AddRef(); return S_OK;
        }
        *ppv = NULL; return E_NOINTERFACE;
    }
    ULONG STDMETHODCALLTYPE AddRef() override  { return InterlockedIncrement(&ref); }
    ULONG STDMETHODCALLTYPE Release() override { return InterlockedDecrement(&ref); }
    HRESULT STDMETHODCALLTYPE SampleCB(double, IMediaSample*) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE BufferCB(double, BYTE *pBuf, long len) override {
        if (len <= 0 || !pBuf) return S_OK;
        if ((InterlockedIncrement(&frame_count) % FRAME_SKIP) != 0) return S_OK;
        std::unique_lock<std::mutex> lk(g_previewMutex, std::try_to_lock);
        if (!lk.owns_lock()) return S_OK;
        if (g_latestPreview.size() != (size_t)len) g_latestPreview.resize((size_t)len);
        memcpy(g_latestPreview.data(), pBuf, (size_t)len);
        g_previewSeq.fetch_add(1);
        g_previewCV.notify_all();
        return S_OK;
    }
};

class StillCB : public ISampleGrabberCB_local {
public:
    LONG ref = 1;
    std::mutex tick_mu;
    DWORD last_press_tick = 0;

    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID riid, void **ppv) override {
        if (!ppv) return E_POINTER;
        if (riid == IID_IUnknown || riid == IID_ISampleGrabberCB_local) {
            *ppv = static_cast<ISampleGrabberCB_local*>(this); AddRef(); return S_OK;
        }
        *ppv = NULL; return E_NOINTERFACE;
    }
    ULONG STDMETHODCALLTYPE AddRef() override  { return InterlockedIncrement(&ref); }
    ULONG STDMETHODCALLTYPE Release() override { return InterlockedDecrement(&ref); }
    HRESULT STDMETHODCALLTYPE SampleCB(double, IMediaSample*) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE BufferCB(double, BYTE *pBuf, long len) override {
        (void)pBuf;  // Still-pin bytes are discarded; we snapshot the preview.
        DWORD now = GetTickCount();
        {
            std::lock_guard<std::mutex> lk(tick_mu);
            DWORD since = now - last_press_tick;
            if (last_press_tick != 0 && since < g_debounce_ms) {
                log_ts("  still trigger (%ld bytes, +%lums) DEBOUNCED", len, since);
                return S_OK;
            }
            last_press_tick = now;
        }
        // Snapshot the latest high-res preview frame into /still.
        std::vector<BYTE> snapshot;
        {
            std::lock_guard<std::mutex> plk(g_previewMutex);
            snapshot = g_latestPreview;
        }
        if (!snapshot.empty()) {
            size_t sz = snapshot.size();
            std::lock_guard<std::mutex> slk(g_stillMutex);
            g_latestStill = std::move(snapshot);
            g_stillSeq.fetch_add(1);
            log_ts("  still trigger -> /still %zu bytes, sending F9", sz);
        } else {
            log_ts("  still trigger -> preview not ready; /still unchanged, sending F9");
        }
        // Send a single F9 keystroke.
        INPUT inp[2] = {0};
        inp[0].type = INPUT_KEYBOARD;
        inp[0].ki.wVk = VK_F9;
        inp[1] = inp[0];
        inp[1].ki.dwFlags = KEYEVENTF_KEYUP;
        SendInput(2, inp, sizeof(INPUT));
        return S_OK;
    }
};

// ---- DirectShow setup ----
static IBaseFilter* find_dermoscope(const char *substr) {
    ICreateDevEnum *pSysDevEnum = NULL;
    if (FAILED(CoCreateInstance(CLSID_SystemDeviceEnum, NULL, CLSCTX_INPROC_SERVER,
                                IID_ICreateDevEnum, (void**)&pSysDevEnum))) return NULL;
    IEnumMoniker *pEnumCat = NULL;
    HRESULT hr = pSysDevEnum->CreateClassEnumerator(CLSID_VideoInputDeviceCategory, &pEnumCat, 0);
    pSysDevEnum->Release();
    if (hr != S_OK) return NULL;
    IBaseFilter *pSrc = NULL;
    IMoniker *pMon = NULL;
    while (pEnumCat->Next(1, &pMon, NULL) == S_OK) {
        IPropertyBag *pBag = NULL;
        if (SUCCEEDED(pMon->BindToStorage(0, 0, IID_IPropertyBag, (void**)&pBag))) {
            VARIANT vName, vDev; VariantInit(&vName); VariantInit(&vDev);
            pBag->Read(L"FriendlyName", &vName, 0);
            pBag->Read(L"DevicePath",   &vDev,  0);
            if (!pSrc && vDev.bstrVal) {
                wchar_t lo[1024]={0};
                wcsncpy(lo, vDev.bstrVal, 1023);
                for (wchar_t *p=lo; *p; ++p) *p = towlower(*p);
                wchar_t target[64]={0};
                for (size_t i=0; substr[i] && i<63; ++i) target[i] = (wchar_t)substr[i];
                if (wcsstr(lo, target)) {
                    pMon->BindToObject(0,0,IID_IBaseFilter,(void**)&pSrc);
                    log_ts("Selected device: %S", vName.bstrVal ? vName.bstrVal : L"(unnamed)");
                }
            }
            VariantClear(&vName); VariantClear(&vDev);
            pBag->Release();
        }
        pMon->Release();
    }
    pEnumCat->Release();
    return pSrc;
}

// Picks an MJPG media type on the given pin category. If wantW/wantH is very
// large (e.g. 9999), falls back to the highest-resolution MJPG available.
static void configure_format(IBaseFilter *pSrc, ICaptureGraphBuilder2 *pBuilder,
                             const GUID *pinCategory, int wantW, int wantH) {
    IAMStreamConfig *pCfg = NULL;
    HRESULT hr = pBuilder->FindInterface(pinCategory, &MEDIATYPE_Video,
                                         pSrc, IID_IAMStreamConfig, (void**)&pCfg);
    if (FAILED(hr) || !pCfg) return;

    int count=0, sz=0;
    pCfg->GetNumberOfCapabilities(&count, &sz);
    BYTE *caps = (BYTE*)malloc(sz);
    AM_MEDIA_TYPE *bestMT = NULL;
    int bestW = 0, bestH = 0;
    for (int i = 0; i < count; ++i) {
        AM_MEDIA_TYPE *mt = NULL;
        if (SUCCEEDED(pCfg->GetStreamCaps(i, &mt, caps)) && mt) {
            if (mt->subtype == MEDIASUBTYPE_MJPG &&
                mt->formattype == FORMAT_VideoInfo && mt->pbFormat) {
                VIDEOINFOHEADER *vih = (VIDEOINFOHEADER*)mt->pbFormat;
                int w = vih->bmiHeader.biWidth;
                int h = vih->bmiHeader.biHeight;
                bool better = (!bestMT) ||
                    (w >= wantW && h >= wantH && (bestW < wantW || bestH < wantH || w*h < bestW*bestH)) ||
                    (bestW < wantW && bestH < wantH && w*h > bestW*bestH);
                if (better) {
                    if (bestMT) {
                        if (bestMT->cbFormat) CoTaskMemFree(bestMT->pbFormat);
                        CoTaskMemFree(bestMT);
                    }
                    bestMT = mt; bestW = w; bestH = h;
                    continue;
                }
            }
            if (mt->cbFormat) CoTaskMemFree(mt->pbFormat);
            CoTaskMemFree(mt);
        }
    }
    free(caps);
    if (bestMT) {
        log_ts("Setting format MJPG %dx%d on pin", bestW, bestH);
        pCfg->SetFormat(bestMT);
        if (bestMT->cbFormat) CoTaskMemFree(bestMT->pbFormat);
        CoTaskMemFree(bestMT);
    }
    pCfg->Release();
}

// ---- HTTP server ----
static const char INDEX_HTML[] =
"<!DOCTYPE html>\n"
"<html><head><meta charset=\"utf-8\"><title>Dermoscope helper</title>\n"
"<style>\n"
"  body { font-family: system-ui, sans-serif; margin: 16px; }\n"
"  .row { display: flex; gap: 16px; align-items: flex-start; flex-wrap: wrap; }\n"
"  .col { flex: 1; min-width: 320px; }\n"
"  img, canvas { max-width: 100%; border: 1px solid #888; background: #111; }\n"
"  h2 { margin: 0 0 8px; }\n"
"  #status { margin-top: 8px; font-family: monospace; }\n"
"  button { margin-top: 8px; padding: 6px 14px; font-size: 14px; }\n"
"</style></head><body>\n"
"<h1>Dermoscope helper - test page</h1>\n"
"<p>Press the hardware button (or F9) to capture a full-res still from <code>/still</code>.</p>\n"
"<div class=\"row\">\n"
"  <div class=\"col\"><h2>Live preview</h2><img id=\"live\" src=\"/preview\" /></div>\n"
"  <div class=\"col\"><h2>Last capture</h2><canvas id=\"captured\"></canvas>\n"
"    <div><button id=\"clear\">Clear</button></div></div>\n"
"</div>\n"
"<div id=\"status\">Waiting for capture...</div>\n"
"<script>\n"
"let captureNum = 0;\n"
"const statusEl = () => document.getElementById('status');\n"
"function setStatus(t) { statusEl().textContent = t; }\n"
"async function capture() {\n"
"  try {\n"
"    const resp = await fetch('/still', {cache:'no-store'});\n"
"    if (!resp.ok) { setStatus(`/still returned ${resp.status}`); return; }\n"
"    const blob = await resp.blob();\n"
"    const url  = URL.createObjectURL(blob);\n"
"    const img  = new Image();\n"
"    img.onload = () => {\n"
"      const canvas = document.getElementById('captured');\n"
"      canvas.width  = img.naturalWidth;\n"
"      canvas.height = img.naturalHeight;\n"
"      canvas.getContext('2d').drawImage(img, 0, 0);\n"
"      URL.revokeObjectURL(url);\n"
"      captureNum++;\n"
"      setStatus(`Capture #${captureNum} at ${new Date().toLocaleTimeString()} (${img.naturalWidth}x${img.naturalHeight})`);\n"
"    };\n"
"    img.src = url;\n"
"  } catch(e) { setStatus(`fetch /still error: ${e.message}`); }\n"
"}\n"
"document.addEventListener('keydown', e => {\n"
"  if (e.key === 'F9' || e.code === 'F9') { e.preventDefault(); capture(); }\n"
"});\n"
"document.getElementById('live').addEventListener('click', capture);\n"
"document.getElementById('clear').addEventListener('click', () => {\n"
"  const canvas = document.getElementById('captured');\n"
"  canvas.getContext('2d').clearRect(0, 0, canvas.width, canvas.height);\n"
"  setStatus('Cleared');\n"
"});\n"
"</script>\n"
"</body></html>\n";

static void send_all(SOCKET s, const char *buf, int len) {
    while (len > 0) {
        int n = send(s, buf, len, 0);
        if (n <= 0) return;
        buf += n; len -= n;
    }
}

static void serve_client(SOCKET sock) {
    char buf[4096] = {0};
    int n = recv(sock, buf, sizeof(buf)-1, 0);
    if (n <= 0) { closesocket(sock); return; }
    buf[n] = 0;

    char method[16] = {0}, path[256] = {0};
    sscanf(buf, "%15s %255s", method, path);

    if (strcmp(path, "/") == 0 || strcmp(path, "/index.html") == 0) {
        char hdr[256];
        int hlen = snprintf(hdr, sizeof(hdr),
            "HTTP/1.0 200 OK\r\n"
            "Content-Type: text/html; charset=utf-8\r\n"
            "Content-Length: %zu\r\n"
            "Connection: close\r\n\r\n",
            sizeof(INDEX_HTML) - 1);
        send_all(sock, hdr, hlen);
        send_all(sock, INDEX_HTML, sizeof(INDEX_HTML) - 1);
        closesocket(sock); return;
    }

    if (strcmp(path, "/preview") == 0) {
        const char *hdr =
            "HTTP/1.0 200 OK\r\n"
            "Connection: close\r\n"
            "Cache-Control: no-cache, private\r\n"
            "Pragma: no-cache\r\n"
            "Access-Control-Allow-Origin: *\r\n"
            "Content-Type: multipart/x-mixed-replace; boundary=frame\r\n\r\n";
        send_all(sock, hdr, (int)strlen(hdr));

        long long lastSeq = -1;
        while (g_running) {
            std::vector<BYTE> frame;
            {
                std::unique_lock<std::mutex> lk(g_previewMutex);
                g_previewCV.wait_for(lk, std::chrono::seconds(2),
                                    [&]{ return g_previewSeq.load() != lastSeq || !g_running; });
                if (!g_running) break;
                if (g_previewSeq.load() == lastSeq) continue;
                frame = g_latestPreview;
                lastSeq = g_previewSeq.load();
            }
            char part[256];
            int plen = snprintf(part, sizeof(part),
                "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %zu\r\n\r\n",
                frame.size());
            if (send(sock, part, plen, 0) <= 0) break;
            if (send(sock, (const char*)frame.data(), (int)frame.size(), 0) <= 0) break;
            if (send(sock, "\r\n", 2, 0) <= 0) break;
        }
        closesocket(sock); return;
    }

    if (strcmp(path, "/still") == 0) {
        std::vector<BYTE> frame;
        {
            std::lock_guard<std::mutex> lk(g_stillMutex);
            frame = g_latestStill;
        }
        if (frame.empty()) {
            const char *r = "HTTP/1.0 404 Not Found\r\n"
                            "Content-Type: text/plain\r\n"
                            "Access-Control-Allow-Origin: *\r\n"
                            "Content-Length: 22\r\n"
                            "Connection: close\r\n\r\n"
                            "no still captured yet\n";
            send_all(sock, r, (int)strlen(r));
        } else {
            char hdr[256];
            int hlen = snprintf(hdr, sizeof(hdr),
                "HTTP/1.0 200 OK\r\n"
                "Content-Type: image/jpeg\r\n"
                "Cache-Control: no-store\r\n"
                "Access-Control-Allow-Origin: *\r\n"
                "Content-Length: %zu\r\n"
                "Connection: close\r\n\r\n",
                frame.size());
            send_all(sock, hdr, hlen);
            send_all(sock, (const char*)frame.data(), (int)frame.size());
        }
        closesocket(sock); return;
    }

    const char *resp = "HTTP/1.0 404 Not Found\r\n"
                       "Content-Length: 0\r\n"
                       "Access-Control-Allow-Origin: *\r\n"
                       "Connection: close\r\n\r\n";
    send_all(sock, resp, (int)strlen(resp));
    closesocket(sock);
}

static void http_server(int port) {
    WSADATA wsa;
    WSAStartup(MAKEWORD(2,2), &wsa);
    SOCKET srv = socket(AF_INET, SOCK_STREAM, 0);
    BOOL on = 1;
    setsockopt(srv, SOL_SOCKET, SO_REUSEADDR, (const char*)&on, sizeof(on));
    sockaddr_in addr = {0};
    addr.sin_family = AF_INET;
    addr.sin_port = htons((u_short)port);
    addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    if (bind(srv, (sockaddr*)&addr, sizeof(addr)) < 0) {
        log_ts("bind() failed: %d", WSAGetLastError());
        return;
    }
    listen(srv, 5);
    log_ts("HTTP server listening on http://localhost:%d/", port);
    while (g_running) {
        SOCKET cl = accept(srv, NULL, NULL);
        if (cl == INVALID_SOCKET) continue;
        std::thread(serve_client, cl).detach();
    }
}

int main(int argc, char **argv) {
    int port = (argc > 1) ? atoi(argv[1]) : 8080;
    if (argc > 2) g_debounce_ms = (DWORD)atoi(argv[2]);
    log_ts("Config: port=%d debounce_ms=%lu", port, g_debounce_ms);

    CoInitializeEx(NULL, COINIT_MULTITHREADED);

    log_ts("Looking for dermoscope (vid_ab02)...");
    IBaseFilter *pSrc = find_dermoscope("vid_ab02");
    if (!pSrc) { log_ts("Device not found."); return 1; }

    IGraphBuilder *pGraph = NULL;
    CoCreateInstance(CLSID_FilterGraph, NULL, CLSCTX_INPROC_SERVER,
                     IID_IGraphBuilder, (void**)&pGraph);
    ICaptureGraphBuilder2 *pBuilder = NULL;
    CoCreateInstance(CLSID_CaptureGraphBuilder2, NULL, CLSCTX_INPROC_SERVER,
                     IID_ICaptureGraphBuilder2, (void**)&pBuilder);
    pBuilder->SetFiltergraph(pGraph);
    pGraph->AddFilter(pSrc, L"Source");

    // Capture pin at 1600x1200 MJPG: live preview + source of /still snapshot.
    // Still pin at 320x240 MJPG: hardware-button trigger only; bytes discarded.
    configure_format(pSrc, pBuilder, &PIN_CATEGORY_CAPTURE, 9999, 9999);
    configure_format(pSrc, pBuilder, &PIN_CATEGORY_STILL,    320,  240);

    // Capture pin -> Preview SampleGrabber -> NullRenderer
    IBaseFilter *pPrevGrab = NULL;
    CoCreateInstance(CLSID_SampleGrabber_local, NULL, CLSCTX_INPROC_SERVER,
                     IID_IBaseFilter, (void**)&pPrevGrab);
    pGraph->AddFilter(pPrevGrab, L"PreviewGrabber");
    ISampleGrabber_local *pPrevSG = NULL;
    pPrevGrab->QueryInterface(IID_ISampleGrabber_local, (void**)&pPrevSG);
    AM_MEDIA_TYPE mtMjpg = {0};
    mtMjpg.majortype = MEDIATYPE_Video;
    mtMjpg.subtype   = MEDIASUBTYPE_MJPG;
    pPrevSG->SetMediaType(&mtMjpg);
    pPrevSG->SetBufferSamples(FALSE);
    pPrevSG->SetOneShot(FALSE);
    PreviewCB *pPrevCB = new PreviewCB();
    pPrevSG->SetCallback(pPrevCB, 1);

    IBaseFilter *pNullPrev = NULL;
    CoCreateInstance(CLSID_NullRenderer_local, NULL, CLSCTX_INPROC_SERVER,
                     IID_IBaseFilter, (void**)&pNullPrev);
    pGraph->AddFilter(pNullPrev, L"NullPreview");
    HRESULT hr = pBuilder->RenderStream(&PIN_CATEGORY_CAPTURE, &MEDIATYPE_Video,
                                        pSrc, pPrevGrab, pNullPrev);
    if (FAILED(hr)) { log_ts("RenderStream(CAPTURE) failed: 0x%08lX", (unsigned long)hr); return 2; }

    // Still pin -> Still SampleGrabber -> NullRenderer
    IBaseFilter *pStillGrab = NULL;
    CoCreateInstance(CLSID_SampleGrabber_local, NULL, CLSCTX_INPROC_SERVER,
                     IID_IBaseFilter, (void**)&pStillGrab);
    pGraph->AddFilter(pStillGrab, L"StillGrabber");
    ISampleGrabber_local *pStillSG = NULL;
    pStillGrab->QueryInterface(IID_ISampleGrabber_local, (void**)&pStillSG);
    AM_MEDIA_TYPE mtV = {0};
    mtV.majortype = MEDIATYPE_Video;
    pStillSG->SetMediaType(&mtV);
    pStillSG->SetBufferSamples(FALSE);
    pStillSG->SetOneShot(FALSE);
    StillCB *pStillCB = new StillCB();
    pStillSG->SetCallback(pStillCB, 1);

    IBaseFilter *pNullStill = NULL;
    CoCreateInstance(CLSID_NullRenderer_local, NULL, CLSCTX_INPROC_SERVER,
                     IID_IBaseFilter, (void**)&pNullStill);
    pGraph->AddFilter(pNullStill, L"NullStill");
    hr = pBuilder->RenderStream(&PIN_CATEGORY_STILL, &MEDIATYPE_Video,
                                pSrc, pStillGrab, pNullStill);
    if (FAILED(hr)) { log_ts("RenderStream(STILL) failed: 0x%08lX (button events disabled)", (unsigned long)hr); }

    IMediaControl *pMC = NULL;
    pGraph->QueryInterface(IID_IMediaControl, (void**)&pMC);
    hr = pMC->Run();
    log_ts("MediaControl::Run HR=0x%08lX", (unsigned long)hr);

    OAFilterState fs = State_Stopped;
    pMC->GetState(2000, &fs);
    log_ts("Graph state: %ld (2=Running)", fs);

    std::thread server(http_server, port);
    server.detach();

    log_ts("Helper ready. Open http://localhost:%d/ in your browser.", port);
    log_ts("Hardware button -> F9 -> web app fetches /still. Ctrl-C to quit.");

    while (g_running) Sleep(1000);

    pMC->Stop();
    return 0;
}
