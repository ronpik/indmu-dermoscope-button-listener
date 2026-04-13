// Dermoscope helper for Windows (approach C).
//
// Owns the dermoscope camera via DirectShow. Serves an MJPEG preview and a
// full-resolution still-capture endpoint over HTTP, and emits a sequence of
// F9 keystrokes via SendInput matching the count of physical button presses
// (1=single, 2=double, 3=triple; 4+ is clamped to 3).
//
// F9 was chosen as the only keystroke (vs F10/F11) because F10 activates the
// window menu bar in many Windows apps and F11 toggles browser fullscreen.
// The web app is expected to count F9 keydowns arriving within a short window
// (~300-500 ms) to decide the action.
//
// Endpoints (default port 8080):
//   GET /          -> minimal HTML test page (preview + capture + F9 sequence handler)
//   GET /preview   -> multipart/x-mixed-replace MJPEG stream (low-res, always on)
//   GET /still     -> image/jpeg of the most recent button-triggered still (high-res)
//
// Web-app integration contract:
//   - <img src="http://localhost:8080/preview"> for live video
//   - listen for F9 keydown; buffer arrivals until a short quiet window elapses;
//     dispatch single/double/triple action based on the count; fetch /still for
//     the high-res JPEG when the action is "capture"
//
// DirectShow graph:
//   SourceFilter (HT-B30S)
//     ├── Capture pin (MJPG 1600x1200) -> SampleGrabber (PreviewCB) -> NullRenderer
//     │     -- feeds /preview (live stream) AND /still (snapshot at click moment)
//     └── Still pin   (MJPG 320x240)   -> SampleGrabber (StillCB)   -> NullRenderer
//           -- used ONLY as the hardware-button trigger; its bytes are discarded.
//              Kept at 320x240 because higher-res stills make the device's
//              firmware cooldown too long to detect rapid multi-clicks.

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
#include <queue>
#include <chrono>

#pragma comment(lib, "ws2_32.lib")

// ---- Click-pattern tuning (overridable from argv) ----
// Device throttles stills to ~515 ms min gap on rapid clicks, so g_group_ms
// must comfortably exceed that. See docs/NEXT-SESSION.md for background.
static DWORD g_debounce_ms = 150;
static DWORD g_group_ms    = 800;

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
    std::queue<DWORD> press_queue;
    std::mutex queue_mu;
    std::condition_variable queue_cv;
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
        (void)pBuf;  // Still pin bytes are discarded; we snapshot the preview frame instead.
        DWORD now = GetTickCount();
        bool first_in_burst = false;
        {
            std::lock_guard<std::mutex> lk(queue_mu);
            DWORD since = now - last_press_tick;
            if (last_press_tick != 0 && since < g_debounce_ms) {
                log_ts("  still trigger (%ld bytes, +%lums) DEBOUNCED", len, since);
                return S_OK;
            }
            last_press_tick = now;
            first_in_burst = press_queue.empty();
            press_queue.push(now);
            log_ts("  still trigger (%ld bytes, +%lums) accepted (q=%zu%s)",
                   len, since, press_queue.size(),
                   first_in_burst ? ", first-in-burst -> snapshot preview" : "");
            queue_cv.notify_one();
        }
        // On the first click of a burst, snapshot the latest high-res preview
        // frame into g_latestStill (the buffer served by GET /still). We do
        // this here -- not in the grouper -- so the snapshot reflects the
        // click moment, not 800ms later after the grouper decides.
        if (first_in_burst) {
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
                log_ts("  captured preview frame into /still buffer: %zu bytes", sz);
            } else {
                log_ts("  preview frame not yet ready; /still unchanged");
            }
        }
        return S_OK;
    }

    void run_grouper() {
        while (g_running) {
            std::unique_lock<std::mutex> lk(queue_mu);
            queue_cv.wait(lk, [&]{ return !press_queue.empty() || !g_running; });
            if (!g_running) break;
            for (;;) {
                DWORD last = press_queue.back();
                DWORD now = GetTickCount();
                DWORD elapsed = now - last;
                if (elapsed >= g_group_ms) break;
                queue_cv.wait_for(lk, std::chrono::milliseconds(g_group_ms - elapsed));
                if (!g_running) return;
            }
            int n_raw = (int)press_queue.size();
            std::queue<DWORD> empty;
            std::swap(press_queue, empty);
            lk.unlock();

            // Clamp to max 3 (4+ clicks -> triple).
            int n = n_raw > 3 ? 3 : n_raw;
            if (n <= 0) continue;

            log_ts("CLICK PATTERN: raw=%d clamped=%d -> sending %d F9 keystroke(s)",
                   n_raw, n, n);

            // Send N F9 keystrokes spaced ~40ms apart. Web app counts F9s
            // arriving within its own grouping window and decides the action.
            for (int i = 0; i < n; ++i) {
                if (i > 0) Sleep(40);
                INPUT inp[2] = {0};
                inp[0].type = INPUT_KEYBOARD;
                inp[0].ki.wVk = VK_F9;
                inp[1] = inp[0];
                inp[1].ki.dwFlags = KEYEVENTF_KEYUP;
                SendInput(2, inp, sizeof(INPUT));
            }
        }
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
"</style></head><body>\n"
"<h1>Dermoscope helper — test page</h1>\n"
"<p>Live preview streams from <code>/preview</code>. Button press -&gt; F9 -&gt; page fetches <code>/still</code> for the full-res capture.</p>\n"
"<div class=\"row\">\n"
"  <div class=\"col\"><h2>Live preview (low-res)</h2><img id=\"live\" src=\"/preview\" /></div>\n"
"  <div class=\"col\"><h2>Last capture (full-res)</h2><canvas id=\"captured\"></canvas></div>\n"
"</div>\n"
"<div id=\"status\">Waiting... (single F9=capture, double F9=no-op, triple F9=clear)</div>\n"
"<script>\n"
"// F9 sequence detection: buffer F9 keydowns, wait for a short quiet window,\n"
"// then dispatch by count. 1=capture, 2=no-op, 3(+)=clear.\n"
"const F9_WINDOW_MS = 400;\n"
"let f9Count = 0;\n"
"let f9Timer = null;\n"
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
"function noop() { setStatus('Double F9 (no-op, last capture unchanged)'); }\n"
"function clearCap() {\n"
"  const canvas = document.getElementById('captured');\n"
"  canvas.getContext('2d').clearRect(0, 0, canvas.width, canvas.height);\n"
"  setStatus('Triple F9 (cleared)');\n"
"}\n"
"function dispatchF9Sequence() {\n"
"  const n = Math.min(f9Count, 3);\n"
"  f9Count = 0;\n"
"  if (n === 1) capture();\n"
"  else if (n === 2) noop();\n"
"  else if (n >= 3) clearCap();\n"
"}\n"
"document.addEventListener('keydown', e => {\n"
"  if (e.key === 'F9' || e.code === 'F9') {\n"
"    e.preventDefault();\n"
"    f9Count++;\n"
"    if (f9Timer) clearTimeout(f9Timer);\n"
"    f9Timer = setTimeout(dispatchF9Sequence, F9_WINDOW_MS);\n"
"  }\n"
"});\n"
"// click on live image to simulate a single F9 (handy for manual testing).\n"
"document.getElementById('live').addEventListener('click', () => {\n"
"  f9Count++;\n"
"  if (f9Timer) clearTimeout(f9Timer);\n"
"  f9Timer = setTimeout(dispatchF9Sequence, F9_WINDOW_MS);\n"
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
                            "Content-Length: 28\r\n"
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
    if (argc > 3) g_group_ms    = (DWORD)atoi(argv[3]);
    log_ts("Config: port=%d debounce_ms=%lu group_ms=%lu", port, g_debounce_ms, g_group_ms);

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

    // Preview: pick the device's highest MJPG (1600x1200). That gives us both
    // a sharp live stream at /preview AND a sharp snapshot on button press
    // (we copy the latest preview frame into /still when the Still pin fires).
    //
    // Still: pick the smallest MJPG (320x240). Still bytes are discarded; the
    // only thing that matters is firing fast enough to detect rapid clicks.
    // At high resolution the device's firmware stretches its post-still
    // cooldown long enough that the 2nd/3rd clicks get dropped -- observed
    // empirically and fixed by keeping the still pin small.
    // Capture pin at 1280x960: high-res enough for quality dermoscopy captures
    // (served via /still as a snapshot of the latest preview frame) while
    // keeping continuous USB bandwidth (~2.5 MB/s) well below the cliff where
    // multi-click detection breaks. 1600x1200 was confirmed broken (~9 MB/s
    // saturated the bus and caused Still-pin deliveries for clicks 2/3 of a
    // burst to be dropped upstream); 320x240 was confirmed working. Binary-
    // searching down from here if 1280x960 turns out to still break clicks.
    configure_format(pSrc, pBuilder, &PIN_CATEGORY_CAPTURE,  320,  240);
    configure_format(pSrc, pBuilder, &PIN_CATEGORY_STILL,   320,  240);

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
    std::thread grouper(&StillCB::run_grouper, pStillCB);
    grouper.detach();

    log_ts("Helper ready. Open http://localhost:%d/ in your browser.", port);
    log_ts("Click pattern: 1=F9 (capture), 2=F10 (no-op), 3=F11 (clear).");
    log_ts("Ctrl-C to quit.");

    while (g_running) Sleep(1000);

    pMC->Stop();
    return 0;
}
