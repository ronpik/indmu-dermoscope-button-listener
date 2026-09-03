// Dermoscope helper for Windows (approach C, single-click).
//
// Owns the HT-B30S dermoscope camera via DirectShow, serves an MJPEG live
// preview and a full-resolution still-capture endpoint over HTTP, and emits
// one F9 keystroke per hardware-button press via SendInput.
//
// Endpoints (default port 8080):
//   GET /          -> minimal HTML test page (preview + capture + F9 handler)
//   GET /preview   -> multipart/x-mixed-replace MJPEG stream (1600x1200 live)
//   GET /still     -> image/jpeg of the most recent button-triggered still,
//                     or 204 No Content if no still has been captured yet
//                     this session
//   GET /health    -> application/json helper status, for a web app to
//                     detect that the helper is running (see it below)
//   OPTIONS <path> -> 204 with CORS headers, for any of the paths above
//
// Web-app integration contract:
//   - poll GET /health to detect the helper; a refused connection means it
//     is stopped or not running -- that is the "not connected" signal, not
//     a 404/204 from /still
//   - <img src="http://localhost:8080/preview"> for live video
//   - listen for F9 keydown; on receipt, fetch('/still') for the full-res
//     JPEG (treat 204 as "no still yet", not an error)
//
// Run modes:
//   helper.exe [port] [debounce_ms]
//     Tray mode. Calls FreeConsole(), logs to helper.log next to the exe, adds
//     a notification-area icon and auto-starts capture. The icon's right-click
//     menu offers Start / Stop / Open test page / Exit; left double-click
//     toggles Start/Stop. Only one instance may run at a time (named mutex).
//   helper.exe --console [port] [debounce_ms]
//     Console mode. Keeps the console and logs to stderr exactly as the
//     original program did; the tray icon still appears. Ctrl-C exits cleanly
//     (releasing the camera) rather than hard-killing the process.
//
// Start/Stop own the whole pipeline: Stop tears the DirectShow graph down and
// releases every COM interface, so the camera really is handed back to Windows
// (the Camera app or a browser can open it while we are stopped), and it stops
// the HTTP server -- closing the listening socket and every live client socket,
// then joining the threads -- so a later Start can rebind the same port.
//
// DirectShow graph:
//   SourceFilter (HT-B30S)
//     +-- Capture pin (MJPG 1600x1200) -> SampleGrabber (PreviewCB) -> NullRenderer
//     |     -- feeds /preview (live) AND serves as the source of /still
//     +-- Still pin   (MJPG 320x240)   -> SampleGrabber (StillCB)   -> NullRenderer
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
#include <shellapi.h>
#include <dshow.h>
#include <stdio.h>
#include <stdarg.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#include <wctype.h>
#include <string>
#include <thread>
#include <mutex>
#include <condition_variable>
#include <vector>
#include <set>
#include <atomic>
#include <chrono>

#pragma comment(lib, "ws2_32.lib")

// Stamped by the Makefile via -DHELPER_VERSION=\"x.y.z\" (see CXXFLAGS in
// Makefile), mirroring the VERSIONINFO resource. Falls back to 0.0.0 for a
// bare `g++ helper.cpp` outside the normal build.
#ifndef HELPER_VERSION
#define HELPER_VERSION "0.0.0"
#endif

// Software debounce for the hardware button. The device's own firmware has
// a multi-second cooldown between stills at these settings, so this is
// defense-in-depth only against spurious rapid triggers. Override via argv[2].
static DWORD g_debounce_ms = 300;

// Configured TCP port (positional arg 1). Also used by the tray "Open test
// page" command, which must open the real port and not a hard-coded 8080.
static int g_port = 8080;

// ---- Tray / window constants ----
#define TRAY_WND_CLASS   L"DermoscopeHelperTrayWnd"
// Single-instance mutex name. windows-helper/installer/helper.iss mirrors
// this exact name in its AppMutex directive so the installer can detect (and
// prompt to close) a running helper before install/uninstall -- if you
// rename this, rename it there too or the two will silently drift apart.
#define SINGLE_INSTANCE_MUTEX_NAME L"Local\\DermoscopeHelperSingleInstance"
#define WM_TRAYICON      (WM_APP + 1)
// Posted by the accept thread when the listening socket dies for good, so the
// UI thread (the only thread allowed to touch the tray) can tell the truth.
#define WM_SERVERDOWN    (WM_APP + 2)
// DirectShow event notifications (IMediaEventEx::SetNotifyWindow) -- how we
// learn the dermoscope was unplugged while we were running.
#define WM_GRAPHEVENT    (WM_APP + 3)
#define TRAY_UID         1
#define IDM_START        1001
#define IDM_STOP         1002
#define IDM_OPENPAGE     1003
#define IDM_EXIT         1004
#define IDT_RETRY        1
#define RETRY_INTERVAL_MS 4000
// Icon health check. TaskbarCreated is broadcast to top-level windows only, so
// a message-only window never sees an Explorer restart; polling NIM_MODIFY is
// how we notice the icon is gone. Also retries an NIM_ADD that failed.
#define IDT_ICONCHECK    2
#define ICON_CHECK_INTERVAL_MS 5000

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

// DirectShow friendly name of the attached camera, for GET /health. Written
// once (UI thread, inside find_dermoscope()) per start_capture() call; read
// from HTTP threads, hence the mutex.
static std::mutex g_deviceNameMu;
static std::string g_deviceName;

// steady_clock ms timestamp of the current capture session's start, for
// GET /health's uptime_s. 0 means no session has started yet.
static std::atomic<long long> g_sessionStartMs{0};

// "The HTTP server is up." Deliberately distinct from "the app is exiting" so
// that Stop -> Start cycles work: stop_capture() clears it, start_capture()
// sets it again.
static std::atomic<bool> g_serverRunning{false};
// Incremented for every successful HTTP-server Start. A preview handler keeps
// the generation it joined so a thread that outlives Stop's bounded drain can
// never resume against a later Start when g_serverRunning becomes true again.
static std::atomic<unsigned long long> g_serverGeneration{0};

enum class HelperState { Stopped, Running, DeviceNotFound, CameraBusy, Error };
static std::atomic<HelperState> g_state{HelperState::Stopped};

// The message-only tray window. Declared up here because the accept thread and
// start_capture() both need to post to it; it is only ever *used* on the UI
// thread beyond PostMessage, which is thread-safe.
static HWND g_hwnd = NULL;

// ---- Logging ----
// Console mode: stderr, byte-for-byte the old format. Tray mode: helper.log
// next to the exe (stderr's handle is dead after FreeConsole, so log_ts must
// not touch it). Called from DirectShow streaming threads and HTTP threads,
// hence the mutex; flushed per line so a killed process still leaves a log.
static FILE *g_logFp = NULL;
static std::mutex g_logMu;

static void log_ts(const char *fmt, ...) {
    SYSTEMTIME st; GetLocalTime(&st);
    char ts[16];
    snprintf(ts, sizeof(ts), "%02d:%02d:%02d.%03d",
             st.wHour, st.wMinute, st.wSecond, st.wMilliseconds);
    std::lock_guard<std::mutex> lk(g_logMu);
    FILE *fp = g_logFp;
    if (!fp) return;
    fprintf(fp, "[%s] ", ts);
    va_list ap; va_start(ap, fmt);
    vfprintf(fp, fmt, ap);
    va_end(ap);
    fprintf(fp, "\n");
    fflush(fp);
}

// Opens helper.log next to the exe, rotating it to helper.log.1 once it passes
// ~1 MB. Silent no-op on any failure -- losing the log must never be fatal.
static void open_log_file() {
    wchar_t dir[MAX_PATH] = {0};
    DWORD n = GetModuleFileNameW(NULL, dir, MAX_PATH);
    if (n == 0 || n >= MAX_PATH) return;
    wchar_t *slash = wcsrchr(dir, L'\\');
    if (!slash) return;
    slash[1] = 0;
    size_t dirLen = wcslen(dir);
    if (dirLen + 20 >= MAX_PATH) return;

    wchar_t logPath[MAX_PATH];
    wcscpy(logPath, dir);
    wcscat(logPath, L"helper.log");
    wchar_t oldPath[MAX_PATH];
    wcscpy(oldPath, logPath);
    wcscat(oldPath, L".1");

    WIN32_FILE_ATTRIBUTE_DATA fad;
    if (GetFileAttributesExW(logPath, GetFileExInfoStandard, &fad)) {
        ULONGLONG sz = ((ULONGLONG)fad.nFileSizeHigh << 32) | (ULONGLONG)fad.nFileSizeLow;
        if (sz > 1024ULL * 1024ULL) {
            DeleteFileW(oldPath);
            MoveFileW(logPath, oldPath);
        }
    }
    FILE *fp = _wfopen(logPath, L"a");
    if (!fp) return;
    std::lock_guard<std::mutex> lk(g_logMu);
    g_logFp = fp;
}

// ---- Sample Grabber callbacks ----
class PreviewCB : public ISampleGrabberCB_local {
public:
    LONG ref = 1;
    LONG frame_count = 0;
    static const LONG FRAME_SKIP = 2;
    virtual ~PreviewCB() {}   // we delete these by concrete type in stop_capture()
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
    virtual ~StillCB() {}     // we delete these by concrete type in stop_capture()

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

// ---- Small string helpers ----
static std::string wide_to_utf8(const wchar_t *w) {
    if (!w) return std::string();
    int len = WideCharToMultiByte(CP_UTF8, 0, w, -1, NULL, 0, NULL, NULL);
    if (len <= 0) return std::string();
    std::string out((size_t)len - 1, '\0');   // len counts the null terminator
    WideCharToMultiByte(CP_UTF8, 0, w, -1, &out[0], len, NULL, NULL);
    return out;
}

static std::string json_escape(const std::string &s) {
    std::string out;
    out.reserve(s.size());
    for (unsigned char c : s) {
        switch (c) {
            case '"':  out += "\\\""; break;
            case '\\': out += "\\\\"; break;
            case '\n': out += "\\n"; break;
            case '\r': out += "\\r"; break;
            case '\t': out += "\\t"; break;
            default:
                if (c < 0x20) {
                    char buf[8];
                    snprintf(buf, sizeof(buf), "\\u%04x", c);
                    out += buf;
                } else {
                    out += (char)c;
                }
        }
    }
    return out;
}

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
            // VariantInit only sets vt = VT_EMPTY; the union is NOT cleared, so
            // bstrVal is stack garbage unless Read() actually returned a BSTR.
            if (!pSrc && vDev.vt == VT_BSTR && vDev.bstrVal) {
                wchar_t lo[1024]={0};
                wcsncpy(lo, vDev.bstrVal, 1023);
                for (wchar_t *p=lo; *p; ++p) *p = towlower(*p);
                wchar_t target[64]={0};
                for (size_t i=0; substr[i] && i<63; ++i) target[i] = (wchar_t)substr[i];
                if (wcsstr(lo, target)) {
                    pMon->BindToObject(0,0,IID_IBaseFilter,(void**)&pSrc);
                    const wchar_t *nameW =
                        (vName.vt == VT_BSTR && vName.bstrVal) ? vName.bstrVal : L"(unnamed)";
                    log_ts("Selected device: %S", nameW);
                    {
                        std::lock_guard<std::mutex> lk(g_deviceNameMu);
                        g_deviceName = wide_to_utf8(nameW);
                    }
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

// What DeleteMediaType() does: release pUnk (the media type may carry a COM
// reference), free pbFormat, then free the struct itself.
static void free_mt(AM_MEDIA_TYPE *mt) {
    if (!mt) return;
    if (mt->pUnk) { mt->pUnk->Release(); mt->pUnk = NULL; }
    if (mt->cbFormat) { CoTaskMemFree(mt->pbFormat); mt->pbFormat = NULL; mt->cbFormat = 0; }
    CoTaskMemFree(mt);
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
    HRESULT chr = pCfg->GetNumberOfCapabilities(&count, &sz);
    if (FAILED(chr) || count <= 0 || sz <= 0) {
        log_ts("GetNumberOfCapabilities failed: 0x%08lX (count=%d size=%d)",
               (unsigned long)chr, count, sz);
        pCfg->Release();
        return;
    }
    BYTE *caps = (BYTE*)malloc((size_t)sz);
    if (!caps) { pCfg->Release(); return; }
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
                    free_mt(bestMT);
                    bestMT = mt; bestW = w; bestH = h;
                    continue;
                }
            }
            free_mt(mt);
        }
    }
    free(caps);
    if (bestMT) {
        log_ts("Setting format MJPG %dx%d on pin", bestW, bestH);
        pCfg->SetFormat(bestMT);
        free_mt(bestMT);
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
"  <div class=\"col\"><h2>Live preview</h2><img id=\"live\" src=\"/preview\" />\n"
"    <div id=\"liveStatus\" style=\"font-family: monospace; font-size: 0.85em; color: #888; margin-top: 4px;\"></div></div>\n"
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
"    if (resp.status === 204) { setStatus('No capture yet -- press the hardware button.'); return; }\n"
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
"const liveImg = document.getElementById('live');\n"
"const liveStatusEl = document.getElementById('liveStatus');\n"
"let previewUp = true;\n"
"async function checkPreviewHealth() {\n"
"  const up = await fetch('/', {cache:'no-store'}).then(r => r.ok).catch(() => false);\n"
"  if (!up) {\n"
"    liveStatusEl.textContent = 'Helper stopped -- preview will reconnect automatically once it is running again.';\n"
"  } else if (!previewUp) {\n"
"    liveImg.src = '/preview?t=' + Date.now();\n"
"    liveStatusEl.textContent = 'Reconnected.';\n"
"  } else {\n"
"    liveStatusEl.textContent = '';\n"
"  }\n"
"  previewUp = up;\n"
"}\n"
"checkPreviewHealth();\n"
"setInterval(checkPreviewHealth, 2000);\n"
"async function showHealth() {\n"
"  try {\n"
"    const resp = await fetch('/health', {cache:'no-store'});\n"
"    const data = await resp.json();\n"
"    setStatus(`/health: ${JSON.stringify(data)}`);\n"
"  } catch(e) { setStatus(`/health error: ${e.message}`); }\n"
"}\n"
"showHealth();\n"
"document.getElementById('clear').addEventListener('click', () => {\n"
"  const canvas = document.getElementById('captured');\n"
"  canvas.getContext('2d').clearRect(0, 0, canvas.width, canvas.height);\n"
"  setStatus('Cleared');\n"
"});\n"
"</script>\n"
"</body></html>\n";

static SOCKET g_listenSock = INVALID_SOCKET;
// Heap-owned on purpose: a file-static std::thread that is still joinable when
// static destructors run (i.e. the process was killed rather than exited
// through the message loop) would call std::terminate.
static std::thread *g_acceptThread = NULL;
static std::mutex g_clientMu;
static std::set<SOCKET> g_clientSocks;
static std::atomic<int> g_clientThreads{0};

// Sending every 1600x1200 JPEG to every viewer multiplies bandwidth by the
// number of open tabs. The test page plus the clinical app can otherwise push
// a 100 Mbps link into backpressure and trip the per-socket send timeout. Keep
// the aggregate preview rate close to one full-rate stream by sharing frame
// opportunities across all active preview clients (see /preview below).
static std::mutex g_previewClientMu;
static std::set<unsigned long long> g_previewClientIds;
static std::atomic<unsigned long long> g_nextPreviewClientId{1};

static int preview_client_count() {
    std::lock_guard<std::mutex> lk(g_previewClientMu);
    return (int)g_previewClientIds.size();
}

static void reset_preview_clients() {
    std::lock_guard<std::mutex> lk(g_previewClientMu);
    g_previewClientIds.clear();
}

static constexpr bool preview_sequence_is_assigned(
    unsigned long long sequence,
    unsigned long long clientCount,
    unsigned long long slot) {
    return clientCount <= 1 || sequence % clientCount == slot;
}

static_assert(preview_sequence_is_assigned(7, 1, 0),
              "A sole preview client must receive every frame");
static_assert(preview_sequence_is_assigned(8, 2, 0) &&
              !preview_sequence_is_assigned(8, 2, 1) &&
              !preview_sequence_is_assigned(9, 2, 0) &&
              preview_sequence_is_assigned(9, 2, 1),
              "Two preview clients must alternate frames");
static_assert(preview_sequence_is_assigned(8, 3, 2) &&
              preview_sequence_is_assigned(9, 3, 0) &&
              preview_sequence_is_assigned(10, 3, 1),
              "Preview scheduling must round-robin across three clients");

class PreviewClientRegistration {
public:
    PreviewClientRegistration()
        : id_(g_nextPreviewClientId.fetch_add(1)),
          generation_(g_serverGeneration.load()) {
        std::lock_guard<std::mutex> lk(g_previewClientMu);
        g_previewClientIds.insert(id_);
    }
    ~PreviewClientRegistration() {
        std::lock_guard<std::mutex> lk(g_previewClientMu);
        g_previewClientIds.erase(id_);
    }

    unsigned long long generation() const { return generation_; }

    bool should_send(long long sequence) const {
        std::lock_guard<std::mutex> lk(g_previewClientMu);
        auto mine = g_previewClientIds.find(id_);
        if (mine == g_previewClientIds.end()) return false;

        size_t slot = 0;
        for (auto it = g_previewClientIds.begin(); it != mine; ++it) ++slot;
        size_t count = g_previewClientIds.size();
        return preview_sequence_is_assigned(
            (unsigned long long)sequence,
            (unsigned long long)count,
            (unsigned long long)slot);
    }

    PreviewClientRegistration(const PreviewClientRegistration&) = delete;
    PreviewClientRegistration& operator=(
        const PreviewClientRegistration&) = delete;

private:
    unsigned long long id_;
    unsigned long long generation_;
};

// Returns false if the peer went away mid-write; a short send() is retried so
// a JPEG is never silently truncated.
static bool send_all(SOCKET s, const char *buf, int len) {
    while (len > 0) {
        int n = send(s, buf, len, 0);
        if (n <= 0) return false;
        buf += n; len -= n;
    }
    return true;
}

// Handles one request. Never closes the socket -- serve_client() owns that so
// the close is serialised with the shutdown path's closesocket().
static void handle_client(SOCKET sock) {
    char buf[4096] = {0};
    int n = recv(sock, buf, sizeof(buf)-1, 0);
    if (n <= 0) return;
    buf[n] = 0;

    char method[16] = {0}, path[256] = {0};
    sscanf(buf, "%15s %255s", method, path);

    // Strip a query string before routing: none of our endpoints take
    // parameters, and callers commonly append one anyway as a cache-buster
    // (e.g. "/preview?t=123" to force a browser to re-open the stream).
    // Without this every such request 404s.
    char *query = strchr(path, '?');
    if (query) *query = '\0';

    bool knownPath = strcmp(path, "/") == 0 || strcmp(path, "/index.html") == 0 ||
                     strcmp(path, "/preview") == 0 || strcmp(path, "/still") == 0 ||
                     strcmp(path, "/health") == 0;

    // CORS preflight. Handled before routing by path so a preflight never
    // falls into e.g. /preview's streaming loop or /still's image body.
    if (strcmp(method, "OPTIONS") == 0 && knownPath) {
        const char *r = "HTTP/1.0 204 No Content\r\n"
                        "Access-Control-Allow-Origin: *\r\n"
                        "Access-Control-Allow-Methods: GET, OPTIONS\r\n"
                        "Access-Control-Allow-Headers: *\r\n"
                        "Access-Control-Max-Age: 600\r\n"
                        "Connection: close\r\n\r\n";
        send_all(sock, r, (int)strlen(r));
        return;
    }

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
        return;
    }

    if (strcmp(path, "/preview") == 0) {
        PreviewClientRegistration previewClient;
        const char *hdr =
            "HTTP/1.0 200 OK\r\n"
            "Connection: close\r\n"
            "Cache-Control: no-cache, private\r\n"
            "Pragma: no-cache\r\n"
            "Access-Control-Allow-Origin: *\r\n"
            "Content-Type: multipart/x-mixed-replace; boundary=frame\r\n\r\n";
        if (!send_all(sock, hdr, (int)strlen(hdr))) {
            log_ts("Preview client disconnected while sending headers "
                   "(WSA error %d, %d preview client(s) active).",
                   WSAGetLastError(), preview_client_count());
            return;
        }

        // Start from the CURRENT sequence number, not -1: with -1 a client that
        // connects before the first frame arrives would immediately pass the
        // predicate and emit an empty Content-Length: 0 part.
        long long lastSeq = g_previewSeq.load();
        while (g_serverRunning &&
               g_serverGeneration.load() == previewClient.generation()) {
            std::vector<BYTE> frame;
            long long currentSeq = lastSeq;
            {
                std::unique_lock<std::mutex> lk(g_previewMutex);
                g_previewCV.wait_for(
                    lk,
                    std::chrono::seconds(2),
                    [&] {
                        return g_previewSeq.load() != lastSeq ||
                               !g_serverRunning ||
                               g_serverGeneration.load() !=
                                   previewClient.generation();
                    });
                if (!g_serverRunning ||
                    g_serverGeneration.load() != previewClient.generation()) {
                    break;
                }
                if (g_previewSeq.load() == lastSeq) continue;
                currentSeq = g_previewSeq.load();
                lastSeq = currentSeq;

                // One camera stream is already roughly 50 Mbps at the real
                // 1600x1200 frame sizes. Assign each captured frame to one of
                // the N viewers in round-robin order. Every viewer remains
                // live, sends are staggered instead of bursty, and aggregate
                // network traffic stays near the single-viewer rate. We copy
                // only frames assigned to this client.
                if (!previewClient.should_send(currentSeq)) continue;
                frame = g_latestPreview;
            }
            // Never emit a zero-byte part, and skip anything that isn't a JPEG:
            // the part must start with SOI (FF D8). Only SOI is checked -- many
            // UVC cameras (this Sonix chipset included) omit the trailing EOI
            // marker, so gating on it would drop every frame.
            if (frame.size() < 2 || frame[0] != 0xFF || frame[1] != 0xD8) continue;
            char part[256];
            int plen = snprintf(part, sizeof(part),
                "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %zu\r\n\r\n",
                frame.size());
            if (!send_all(sock, part, plen) ||
                !send_all(sock, (const char*)frame.data(), (int)frame.size()) ||
                !send_all(sock, "\r\n", 2)) {
                log_ts("Preview client disconnected while sending frame %lld "
                       "(WSA error %d, %d preview client(s) active).",
                       currentSeq, WSAGetLastError(), preview_client_count());
                break;
            }
        }
        return;
    }

    if (strcmp(path, "/still") == 0) {
        std::vector<BYTE> frame;
        {
            std::lock_guard<std::mutex> lk(g_stillMutex);
            frame = g_latestStill;
        }
        if (frame.empty()) {
            // 204, not 404: a web app polling /still needs "nothing captured
            // yet this session" to be distinguishable from "the helper isn't
            // running" (which now shows up as a refused connection, or via
            // GET /health). See the endpoint list at the top of this file.
            const char *r = "HTTP/1.0 204 No Content\r\n"
                            "Cache-Control: no-store\r\n"
                            "Access-Control-Allow-Origin: *\r\n"
                            "Connection: close\r\n\r\n";
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
        return;
    }

    if (strcmp(path, "/health") == 0) {
        std::string device;
        {
            std::lock_guard<std::mutex> lk(g_deviceNameMu);
            device = g_deviceName;
        }
        bool stillAvailable;
        {
            std::lock_guard<std::mutex> lk(g_stillMutex);
            stillAvailable = !g_latestStill.empty();
        }
        long long startMs = g_sessionStartMs.load();
        long long nowMs = std::chrono::duration_cast<std::chrono::milliseconds>(
            std::chrono::steady_clock::now().time_since_epoch()).count();
        long long uptimeS = (startMs > 0 && nowMs > startMs) ? (nowMs - startMs) / 1000 : 0;

        // status is always "running": the HTTP server -- and so /health --
        // only exists while capture is running (see the top of this file).
        std::string body;
        body += "{\"status\":\"running\",\"version\":\"" + json_escape(HELPER_VERSION) + "\"";
        body += ",\"device\":\"" + json_escape(device) + "\"";
        body += ",\"preview_frames\":" + std::to_string(g_previewSeq.load());
        body += ",\"preview_clients\":" + std::to_string(preview_client_count());
        body += ",\"still_seq\":" + std::to_string(g_stillSeq.load());
        body += std::string(",\"still_available\":") + (stillAvailable ? "true" : "false");
        body += ",\"port\":" + std::to_string(g_port);
        body += ",\"uptime_s\":" + std::to_string(uptimeS) + "}";

        char hdr[256];
        int hlen = snprintf(hdr, sizeof(hdr),
            "HTTP/1.0 200 OK\r\n"
            "Content-Type: application/json\r\n"
            "Cache-Control: no-store\r\n"
            "Access-Control-Allow-Origin: *\r\n"
            "Content-Length: %zu\r\n"
            "Connection: close\r\n\r\n",
            body.size());
        send_all(sock, hdr, hlen);
        send_all(sock, body.data(), (int)body.size());
        return;
    }

    const char *resp = "HTTP/1.0 404 Not Found\r\n"
                       "Content-Length: 0\r\n"
                       "Access-Control-Allow-Origin: *\r\n"
                       "Connection: close\r\n\r\n";
    send_all(sock, resp, (int)strlen(resp));
}

static void serve_client(SOCKET sock) {
    // handle_client can throw (std::bad_alloc copying a 1600x1200 frame). An
    // exception escaping a detached thread would call std::terminate and would
    // leak the g_clientThreads count, so every later stop_capture() would burn
    // its full 2 s drain budget and warn.
    try {
        handle_client(sock);
    } catch (...) {
        log_ts("HTTP client handler threw; connection dropped.");
    }
    // Erase-and-close under the same mutex the shutdown path uses, so a socket
    // is never closed twice (and a recycled handle is never closed by mistake).
    {
        std::lock_guard<std::mutex> lk(g_clientMu);
        if (g_clientSocks.erase(sock)) closesocket(sock);
    }
    g_clientThreads.fetch_sub(1);
}

static void accept_loop(SOCKET listenSock) {
    log_ts("HTTP server listening on http://localhost:%d/", g_port);
    bool fatal = false;
    while (g_serverRunning) {
        SOCKET cl = accept(listenSock, NULL, NULL);
        if (cl == INVALID_SOCKET) {
            int err = WSAGetLastError();
            if (!g_serverRunning) break;
            // A dead listening socket would otherwise spin this loop forever.
            if (err == WSAENOTSOCK || err == WSAEINVAL || err == WSAEINTR) {
                fatal = true;
                break;
            }
            // Known-transient: retry straight away. WSAECONNRESET here means a
            // QUEUED connection was aborted by the peer before we accepted it
            // (a browser closing a /preview tab mid-load does exactly this) --
            // the listening socket is perfectly healthy, so treating it as
            // fatal would kill the server for the life of the process.
            // Anything else (WSAEMFILE, WSAENOBUFS, ...) is persistent and
            // would peg a core, so back off.
            if (err == WSAECONNABORTED || err == WSAECONNRESET || err == WSAEWOULDBLOCK)
                continue;
            log_ts("accept() failed: %d; backing off 50ms.", err);
            Sleep(50);
            continue;
        }
        if (!g_serverRunning) { closesocket(cl); break; }
        // A client that connects and never sends would otherwise park a thread
        // and a handle forever, guaranteeing the 2 s give-up path on every stop.
        DWORD tmo = 5000;
        setsockopt(cl, SOL_SOCKET, SO_RCVTIMEO, (const char*)&tmo, sizeof(tmo));
        // /preview is write-only, so recv timeouts alone cannot rescue a thread
        // parked in send() behind a stalled reader: shutdown() does not unblock
        // an in-progress send. Keep this below the 2 s drain budget in
        // http_server_stop() -- send_all() treats n <= 0 as a dead peer.
        DWORD stmo = 1000;
        setsockopt(cl, SOL_SOCKET, SO_SNDTIMEO, (const char*)&stmo, sizeof(stmo));
        {
            std::lock_guard<std::mutex> lk(g_clientMu);
            g_clientSocks.insert(cl);
        }
        g_clientThreads.fetch_add(1);
        // Thread creation can throw std::system_error under resource pressure.
        // Unhandled here it would propagate out of the accept thread and abort
        // the process with the tray icon registered and the camera still held.
        try {
            std::thread(serve_client, cl).detach();
        } catch (...) {
            log_ts("Could not spawn HTTP client thread; dropping the connection.");
            {
                std::lock_guard<std::mutex> lk(g_clientMu);
                if (g_clientSocks.erase(cl)) closesocket(cl);
            }
            g_clientThreads.fetch_sub(1);
        }
    }
    if (fatal) {
        // Do not leave the state lying: g_serverRunning true with a dead accept
        // loop would make the tooltip say "running", grey out Start, and make
        // both http_server_start() and start_capture() early-return forever.
        g_serverRunning = false;
        log_ts("HTTP accept loop hit a fatal socket error; the server is DOWN.");
        if (g_hwnd) PostMessageW(g_hwnd, WM_SERVERDOWN, 0, 0);
    }
    log_ts("HTTP accept loop exited.");
}

// Creates, binds and listens on the calling thread so a bind failure is a
// start() failure rather than a silent death inside a detached thread.
static bool http_server_start(int port) {
    if (g_serverRunning) return true;

    SOCKET srv = socket(AF_INET, SOCK_STREAM, 0);
    if (srv == INVALID_SOCKET) {
        log_ts("socket() failed: %d", WSAGetLastError());
        return false;
    }
    BOOL on = 1;
    setsockopt(srv, SOL_SOCKET, SO_REUSEADDR, (const char*)&on, sizeof(on));
    sockaddr_in addr = {0};
    addr.sin_family = AF_INET;
    addr.sin_port = htons((u_short)port);
    addr.sin_addr.s_addr = htonl(INADDR_ANY);
    if (bind(srv, (sockaddr*)&addr, sizeof(addr)) < 0) {
        log_ts("bind() failed on port %d: %d", port, WSAGetLastError());
        closesocket(srv);
        return false;
    }
    if (listen(srv, 5) < 0) {
        log_ts("listen() failed on port %d: %d", port, WSAGetLastError());
        closesocket(srv);
        return false;
    }
    g_listenSock = srv;
    reset_preview_clients();
    g_serverGeneration.fetch_add(1);
    g_serverRunning = true;
    g_sessionStartMs.store(std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::steady_clock::now().time_since_epoch()).count());
    g_acceptThread = new std::thread(accept_loop, srv);
    return true;
}

static void http_server_stop() {
    if (!g_serverRunning && g_listenSock == INVALID_SOCKET && !g_acceptThread)
        return;

    // Store the flag UNDER g_previewMutex: otherwise a /preview thread that has
    // already evaluated the predicate but not yet parked on the CV misses the
    // wakeup and sleeps out its full 2 s -- exactly the drain budget below.
    {
        std::lock_guard<std::mutex> lk(g_previewMutex);
        g_serverRunning = false;
    }
    g_previewCV.notify_all();

    SOCKET srv = g_listenSock;
    g_listenSock = INVALID_SOCKET;
    if (srv != INVALID_SOCKET) closesocket(srv);   // unblocks accept()

    if (g_acceptThread) {
        if (g_acceptThread->joinable()) g_acceptThread->join();
        delete g_acceptThread;
        g_acceptThread = NULL;
    }

    // Unblock any client parked in recv(), and make every subsequent send() on
    // these sockets fail so the handlers unwind. shutdown() does NOT abort a
    // send() that is already in progress -- SO_SNDTIMEO (set in accept_loop) is
    // what bounds that case.
    //
    // shutdown() -- NOT closesocket() -- because the handle must stay allocated:
    // serve_client still holds the numeric value, and closing here would free it
    // for reuse, so a later connection could be handed the same number and then
    // be closed (or written to) by the stale thread. serve_client remains the
    // single owner of the close, hence g_clientSocks is deliberately not
    // cleared here.
    {
        std::lock_guard<std::mutex> lk(g_clientMu);
        for (SOCKET s : g_clientSocks) shutdown(s, SD_BOTH);
    }
    for (int i = 0; i < 200 && g_clientThreads.load() > 0; ++i) Sleep(10);
    if (g_clientThreads.load() > 0)
        log_ts("Warning: %d HTTP client thread(s) still running after 2s.",
               g_clientThreads.load());
    log_ts("HTTP server stopped.");
}

// ---- Capture session ----
// Everything the running graph owns, in one place, so stop_capture() can reach
// it. Creation order is top to bottom; teardown releases bottom to top.
struct CaptureSession {
    IBaseFilter           *pSrc       = NULL;
    IGraphBuilder         *pGraph     = NULL;
    ICaptureGraphBuilder2 *pBuilder   = NULL;
    IBaseFilter           *pPrevGrab  = NULL;
    ISampleGrabber_local  *pPrevSG    = NULL;
    IBaseFilter           *pNullPrev  = NULL;
    IBaseFilter           *pStillGrab = NULL;
    ISampleGrabber_local  *pStillSG   = NULL;
    IBaseFilter           *pNullStill = NULL;
    IMediaControl         *pMC        = NULL;
    IMediaEventEx         *pME        = NULL;   // device-loss notifications
    PreviewCB             *pPrevCB    = NULL;
    StillCB               *pStillCB   = NULL;
};
static CaptureSession g_cap;

static bool is_camera_busy_hr(HRESULT hr) {
    return hr == HRESULT_FROM_WIN32(ERROR_NO_SYSTEM_RESOURCES);   // 0x800705AA
}

template <class T> static void safe_release(T *&p) {
    if (p) { p->Release(); p = NULL; }
}

// Full teardown. Safe to call when already stopped, and safe to call on a
// half-built session (start_capture uses it as its failure path).
static void stop_capture() {
    bool hadSomething = g_serverRunning || g_listenSock != INVALID_SOCKET ||
                        g_acceptThread || g_cap.pGraph || g_cap.pSrc;

    // 1. HTTP server first: no new clients, no threads left running.
    http_server_stop();

    // 2. Detach the callbacks before anything is freed, so no in-flight
    //    streaming callback can touch a destroyed object. Same reasoning for
    //    the graph's event notifications: unhook the window BEFORE the release
    //    below, so no EC_* message can be posted to a window we are tearing
    //    down (or, worse, to a recycled HWND).
    if (g_cap.pME)      g_cap.pME->SetNotifyWindow((OAHWND)0, 0, 0);
    if (g_cap.pStillSG) g_cap.pStillSG->SetCallback(NULL, 1);
    if (g_cap.pPrevSG)  g_cap.pPrevSG->SetCallback(NULL, 1);

    // 3. Stop the graph and wait for the transition to complete. A wedged USB
    //    driver can leave GetState returning VFW_S_STATE_INTERMEDIATE (S_FALSE)
    //    after the 2 s timeout; that must be reported, not papered over with a
    //    "camera released" line that is not true.
    bool stoppedClean = true;
    if (g_cap.pMC) {
        g_cap.pMC->Stop();
        OAFilterState fs = State_Stopped;
        HRESULT shr = g_cap.pMC->GetState(2000, &fs);
        if (shr != S_OK || fs != State_Stopped) {
            log_ts("WARNING: graph did not reach Stopped (GetState HR=0x%08lX state=%ld); retrying Stop().",
                   (unsigned long)shr, (long)fs);
            g_cap.pMC->Stop();
            fs = State_Stopped;
            shr = g_cap.pMC->GetState(2000, &fs);
            if (shr != S_OK || fs != State_Stopped) {
                log_ts("WARNING: graph STILL not Stopped (GetState HR=0x%08lX state=%ld); "
                       "the camera may not have been released.",
                       (unsigned long)shr, (long)fs);
                stoppedClean = false;
            }
        }
    }

    // 4. Unhook the filters from the graph.
    if (g_cap.pGraph) {
        if (g_cap.pNullStill) g_cap.pGraph->RemoveFilter(g_cap.pNullStill);
        if (g_cap.pStillGrab) g_cap.pGraph->RemoveFilter(g_cap.pStillGrab);
        if (g_cap.pNullPrev)  g_cap.pGraph->RemoveFilter(g_cap.pNullPrev);
        if (g_cap.pPrevGrab)  g_cap.pGraph->RemoveFilter(g_cap.pPrevGrab);
        if (g_cap.pSrc)       g_cap.pGraph->RemoveFilter(g_cap.pSrc);
    }

    // 5. Release every interface exactly once, reverse creation order. This is
    //    what actually hands the camera back to Windows.
    safe_release(g_cap.pME);
    safe_release(g_cap.pMC);
    safe_release(g_cap.pStillSG);
    safe_release(g_cap.pPrevSG);
    safe_release(g_cap.pNullStill);
    safe_release(g_cap.pStillGrab);
    safe_release(g_cap.pNullPrev);
    safe_release(g_cap.pPrevGrab);
    safe_release(g_cap.pBuilder);
    safe_release(g_cap.pGraph);
    safe_release(g_cap.pSrc);

    // 6. The qedit SampleGrabber does NOT AddRef the callback object, so its
    //    lifetime is ours. Delete only after SetCallback(NULL) + full release.
    delete g_cap.pPrevCB;  g_cap.pPrevCB  = NULL;
    delete g_cap.pStillCB; g_cap.pStillCB = NULL;

    // 7. Start from a clean slate next time.
    {
        std::lock_guard<std::mutex> lk(g_previewMutex);
        g_latestPreview.clear();
        g_latestPreview.shrink_to_fit();
    }
    {
        std::lock_guard<std::mutex> lk(g_stillMutex);
        g_latestStill.clear();
        g_latestStill.shrink_to_fit();
    }
    {
        std::lock_guard<std::mutex> lk(g_deviceNameMu);
        g_deviceName.clear();
    }
    g_previewSeq.store(0);
    g_stillSeq.store(0);
    g_sessionStartMs.store(0);

    g_state.store(HelperState::Stopped);
    if (hadSomething && stoppedClean) log_ts("Capture stopped; camera released.");
}

static HelperState start_fail(HelperState st) {
    stop_capture();               // tear down whatever was built so far
    g_state.store(st);
    return st;
}

static HelperState start_capture() {
    if (g_state.load() == HelperState::Running) return HelperState::Running;
    stop_capture();               // guarantee a clean slate

    log_ts("Looking for dermoscope (vid_ab02)...");
    g_cap.pSrc = find_dermoscope("vid_ab02");
    if (!g_cap.pSrc) {
        log_ts("Device not found.");
        return start_fail(HelperState::DeviceNotFound);
    }

    HRESULT hr = CoCreateInstance(CLSID_FilterGraph, NULL, CLSCTX_INPROC_SERVER,
                                  IID_IGraphBuilder, (void**)&g_cap.pGraph);
    if (FAILED(hr) || !g_cap.pGraph) {
        log_ts("CoCreateInstance(FilterGraph) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    hr = CoCreateInstance(CLSID_CaptureGraphBuilder2, NULL, CLSCTX_INPROC_SERVER,
                          IID_ICaptureGraphBuilder2, (void**)&g_cap.pBuilder);
    if (FAILED(hr) || !g_cap.pBuilder) {
        log_ts("CoCreateInstance(CaptureGraphBuilder2) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    g_cap.pBuilder->SetFiltergraph(g_cap.pGraph);
    g_cap.pGraph->AddFilter(g_cap.pSrc, L"Source");

    // Capture pin at 1600x1200 MJPG: live preview + source of /still snapshot.
    // Still pin at 320x240 MJPG: hardware-button trigger only; bytes discarded.
    configure_format(g_cap.pSrc, g_cap.pBuilder, &PIN_CATEGORY_CAPTURE, 9999, 9999);
    configure_format(g_cap.pSrc, g_cap.pBuilder, &PIN_CATEGORY_STILL,    320,  240);

    // Capture pin -> Preview SampleGrabber -> NullRenderer
    hr = CoCreateInstance(CLSID_SampleGrabber_local, NULL, CLSCTX_INPROC_SERVER,
                          IID_IBaseFilter, (void**)&g_cap.pPrevGrab);
    if (FAILED(hr) || !g_cap.pPrevGrab) {
        log_ts("CoCreateInstance(SampleGrabber/preview) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    g_cap.pGraph->AddFilter(g_cap.pPrevGrab, L"PreviewGrabber");
    hr = g_cap.pPrevGrab->QueryInterface(IID_ISampleGrabber_local, (void**)&g_cap.pPrevSG);
    if (FAILED(hr) || !g_cap.pPrevSG) {
        log_ts("QueryInterface(ISampleGrabber/preview) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    AM_MEDIA_TYPE mtMjpg = {0};
    mtMjpg.majortype = MEDIATYPE_Video;
    mtMjpg.subtype   = MEDIASUBTYPE_MJPG;
    g_cap.pPrevSG->SetMediaType(&mtMjpg);
    g_cap.pPrevSG->SetBufferSamples(FALSE);
    g_cap.pPrevSG->SetOneShot(FALSE);
    g_cap.pPrevCB = new PreviewCB();
    g_cap.pPrevSG->SetCallback(g_cap.pPrevCB, 1);

    hr = CoCreateInstance(CLSID_NullRenderer_local, NULL, CLSCTX_INPROC_SERVER,
                          IID_IBaseFilter, (void**)&g_cap.pNullPrev);
    if (FAILED(hr) || !g_cap.pNullPrev) {
        log_ts("CoCreateInstance(NullRenderer/preview) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    g_cap.pGraph->AddFilter(g_cap.pNullPrev, L"NullPreview");
    hr = g_cap.pBuilder->RenderStream(&PIN_CATEGORY_CAPTURE, &MEDIATYPE_Video,
                                      g_cap.pSrc, g_cap.pPrevGrab, g_cap.pNullPrev);
    if (FAILED(hr)) {
        log_ts("RenderStream(CAPTURE) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(is_camera_busy_hr(hr) ? HelperState::CameraBusy
                                                : HelperState::Error);
    }

    // Still pin -> Still SampleGrabber -> NullRenderer
    hr = CoCreateInstance(CLSID_SampleGrabber_local, NULL, CLSCTX_INPROC_SERVER,
                          IID_IBaseFilter, (void**)&g_cap.pStillGrab);
    if (FAILED(hr) || !g_cap.pStillGrab) {
        log_ts("CoCreateInstance(SampleGrabber/still) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    g_cap.pGraph->AddFilter(g_cap.pStillGrab, L"StillGrabber");
    hr = g_cap.pStillGrab->QueryInterface(IID_ISampleGrabber_local, (void**)&g_cap.pStillSG);
    if (FAILED(hr) || !g_cap.pStillSG) {
        log_ts("QueryInterface(ISampleGrabber/still) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    AM_MEDIA_TYPE mtV = {0};
    mtV.majortype = MEDIATYPE_Video;
    g_cap.pStillSG->SetMediaType(&mtV);
    g_cap.pStillSG->SetBufferSamples(FALSE);
    g_cap.pStillSG->SetOneShot(FALSE);
    g_cap.pStillCB = new StillCB();
    g_cap.pStillSG->SetCallback(g_cap.pStillCB, 1);

    hr = CoCreateInstance(CLSID_NullRenderer_local, NULL, CLSCTX_INPROC_SERVER,
                          IID_IBaseFilter, (void**)&g_cap.pNullStill);
    if (FAILED(hr) || !g_cap.pNullStill) {
        log_ts("CoCreateInstance(NullRenderer/still) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    g_cap.pGraph->AddFilter(g_cap.pNullStill, L"NullStill");
    hr = g_cap.pBuilder->RenderStream(&PIN_CATEGORY_STILL, &MEDIATYPE_Video,
                                      g_cap.pSrc, g_cap.pStillGrab, g_cap.pNullStill);
    if (FAILED(hr)) {
        // Non-fatal, exactly as before: preview still works, the hardware
        // button does not.
        log_ts("RenderStream(STILL) failed: 0x%08lX (button events disabled)", (unsigned long)hr);
    }

    hr = g_cap.pGraph->QueryInterface(IID_IMediaControl, (void**)&g_cap.pMC);
    if (FAILED(hr) || !g_cap.pMC) {
        log_ts("QueryInterface(IMediaControl) failed: 0x%08lX", (unsigned long)hr);
        return start_fail(HelperState::Error);
    }
    hr = g_cap.pMC->Run();
    log_ts("MediaControl::Run HR=0x%08lX", (unsigned long)hr);
    if (FAILED(hr)) {
        if (is_camera_busy_hr(hr))
            log_ts("Camera is in use by another application (ERROR_NO_SYSTEM_RESOURCES).");
        return start_fail(is_camera_busy_hr(hr) ? HelperState::CameraBusy
                                                : HelperState::Error);
    }

    // Run() legitimately returns S_FALSE (0x00000001) on this device while the
    // graph is still transitioning -- that is NOT a failure, so GetState is the
    // arbiter. Without this check a graph that never reaches Running would
    // still light up the tooltip, grey out Start and kill the retry timer.
    OAFilterState fs = State_Stopped;
    HRESULT ghr = g_cap.pMC->GetState(2000, &fs);
    log_ts("Graph state: %ld (2=Running)", (long)fs);
    if (ghr != S_OK || fs != State_Running) {
        log_ts("Graph did not reach Running (GetState HR=0x%08lX state=%ld).",
               (unsigned long)ghr, (long)fs);
        return start_fail(is_camera_busy_hr(ghr) ? HelperState::CameraBusy
                                                 : HelperState::Error);
    }

    // Device-loss detection. Purely additive: if any of this fails we simply
    // lose the unplug notification, exactly as before.
    HRESULT ehr = g_cap.pGraph->QueryInterface(IID_IMediaEventEx, (void**)&g_cap.pME);
    if (SUCCEEDED(ehr) && g_cap.pME && g_hwnd) {
        g_cap.pME->SetNotifyWindow((OAHWND)g_hwnd, WM_GRAPHEVENT, 0);
    } else {
        log_ts("IMediaEventEx unavailable (0x%08lX); device-loss detection is off.",
               (unsigned long)ehr);
        safe_release(g_cap.pME);
    }

    if (!http_server_start(g_port)) {
        return start_fail(HelperState::Error);
    }

    g_state.store(HelperState::Running);
    log_ts("Helper ready. Open http://localhost:%d/ in your browser.", g_port);
    log_ts("Hardware button -> F9 -> web app fetches /still.");
    return HelperState::Running;
}

// ---- Tray icon ----
static HICON g_iconColor   = NULL;   // resource 101 if present, else IDI_APPLICATION
static HICON g_iconGrey    = NULL;   // desaturated variant; may be NULL
static bool  g_iconGreyOwned = false;
static UINT  g_wmTaskbarCreated  = 0;
static UINT  g_wmAlreadyRunning  = 0;
static NOTIFYICONDATAW g_nid = {};
static bool  g_iconAdded   = false;
static bool  g_shutdownDone = false;
static std::atomic<bool> g_cleanExitDone{false};

static const wchar_t* state_tip(HelperState s) {
    switch (s) {
        case HelperState::Running:        return L"Dermoscope Helper: running";
        case HelperState::DeviceNotFound: return L"Dermoscope Helper: device not found";
        case HelperState::CameraBusy:     return L"Dermoscope Helper: camera busy";
        case HelperState::Error:          return L"Dermoscope Helper: error";
        default:                          return L"Dermoscope Helper: stopped";
    }
}

// Renders the icon into a 32bpp top-down DIB, desaturates RGB (alpha kept) and
// rebuilds it. Every step is checked; on any failure we return NULL and the
// caller simply uses the colour icon for both states.
//
// The four GDI entry points are bound at runtime rather than at link time:
// gdi32 is not in mingw's default library list and the Makefile is off limits.
// gdi32.dll is present on every Windows install (user32 depends on it), so
// this only ever costs a GetProcAddress; if anything is missing we bail out
// and the tray simply uses one icon for every state.
typedef int     (WINAPI *PFN_GetObjectW)(HANDLE, int, LPVOID);
typedef int     (WINAPI *PFN_GetDIBits)(HDC, HBITMAP, UINT, UINT, LPVOID, LPBITMAPINFO, UINT);
typedef HBITMAP (WINAPI *PFN_CreateDIBSection)(HDC, const BITMAPINFO*, UINT, void**, HANDLE, DWORD);
typedef BOOL    (WINAPI *PFN_DeleteObject)(HGDIOBJ);

static HICON make_grey_icon(HICON src) {
    if (!src) return NULL;

    HMODULE gdi = LoadLibraryW(L"gdi32.dll");
    if (!gdi) return NULL;
    PFN_GetObjectW       pGetObject   = (PFN_GetObjectW)      (void*)GetProcAddress(gdi, "GetObjectW");
    PFN_GetDIBits        pGetDIBits   = (PFN_GetDIBits)       (void*)GetProcAddress(gdi, "GetDIBits");
    PFN_CreateDIBSection pCreateDIB   = (PFN_CreateDIBSection)(void*)GetProcAddress(gdi, "CreateDIBSection");
    PFN_DeleteObject     pDeleteObject= (PFN_DeleteObject)    (void*)GetProcAddress(gdi, "DeleteObject");
    if (!pGetObject || !pGetDIBits || !pCreateDIB || !pDeleteObject) {
        FreeLibrary(gdi);
        return NULL;
    }

    ICONINFO ii = {};
    if (!GetIconInfo(src, &ii)) { FreeLibrary(gdi); return NULL; }

    HICON out = NULL;
    HDC hdc = GetDC(NULL);
    BITMAP bm = {};
    if (hdc && ii.hbmColor && ii.hbmMask &&
        pGetObject(ii.hbmColor, sizeof(bm), &bm) &&
        bm.bmWidth > 0 && bm.bmHeight > 0 && bm.bmWidth <= 512 && bm.bmHeight <= 512) {
        BITMAPINFO bi = {};
        bi.bmiHeader.biSize        = sizeof(BITMAPINFOHEADER);
        bi.bmiHeader.biWidth       = bm.bmWidth;
        bi.bmiHeader.biHeight      = -bm.bmHeight;   // top-down
        bi.bmiHeader.biPlanes      = 1;
        bi.bmiHeader.biBitCount    = 32;
        bi.bmiHeader.biCompression = BI_RGB;
        size_t px = (size_t)bm.bmWidth * (size_t)bm.bmHeight;
        std::vector<BYTE> bits(px * 4, 0);
        if (pGetDIBits(hdc, ii.hbmColor, 0, (UINT)bm.bmHeight,
                       bits.data(), &bi, DIB_RGB_COLORS)) {
            for (size_t i = 0; i < px; ++i) {
                BYTE b = bits[i*4 + 0], g = bits[i*4 + 1], r = bits[i*4 + 2];
                BYTE y = (BYTE)(((unsigned)r * 54 + (unsigned)g * 183 + (unsigned)b * 19) >> 8);
                bits[i*4 + 0] = bits[i*4 + 1] = bits[i*4 + 2] = y;   // alpha untouched
            }
            void *dst = NULL;
            HBITMAP hNew = pCreateDIB(hdc, &bi, DIB_RGB_COLORS, &dst, NULL, 0);
            if (hNew && dst) {
                memcpy(dst, bits.data(), px * 4);
                ICONINFO ni = {};
                ni.fIcon    = TRUE;
                ni.hbmColor = hNew;
                ni.hbmMask  = ii.hbmMask;
                out = CreateIconIndirect(&ni);
            }
            if (hNew) pDeleteObject(hNew);
        }
    }
    if (hdc) ReleaseDC(NULL, hdc);
    if (ii.hbmColor) pDeleteObject(ii.hbmColor);
    if (ii.hbmMask)  pDeleteObject(ii.hbmMask);
    FreeLibrary(gdi);
    return out;
}

static void load_icons() {
    HICON h = LoadIcon(GetModuleHandle(NULL), MAKEINTRESOURCE(101));
    if (!h) h = LoadIcon(NULL, IDI_APPLICATION);
    g_iconColor = h;
    g_iconGrey  = make_grey_icon(h);
    g_iconGreyOwned = (g_iconGrey != NULL);
    if (!g_iconGrey) g_iconGrey = g_iconColor;
}

static void tray_update() {
    if (!g_iconAdded) return;
    HelperState s = g_state.load();
    g_nid.uFlags  = NIF_ICON | NIF_TIP | NIF_MESSAGE;
    g_nid.hIcon   = (s == HelperState::Running) ? g_iconColor : g_iconGrey;
    wcsncpy(g_nid.szTip, state_tip(s), 127);
    g_nid.szTip[127] = 0;
    Shell_NotifyIconW(NIM_MODIFY, &g_nid);
}

static void tray_add() {
    ZeroMemory(&g_nid, sizeof(g_nid));
    g_nid.cbSize           = sizeof(g_nid);
    g_nid.hWnd             = g_hwnd;
    g_nid.uID              = TRAY_UID;
    g_nid.uFlags           = NIF_ICON | NIF_TIP | NIF_MESSAGE;
    g_nid.uCallbackMessage = WM_TRAYICON;
    g_nid.hIcon            = (g_state.load() == HelperState::Running) ? g_iconColor : g_iconGrey;
    wcsncpy(g_nid.szTip, state_tip(g_state.load()), 127);
    g_nid.szTip[127] = 0;
    g_iconAdded = (Shell_NotifyIconW(NIM_ADD, &g_nid) != FALSE);
    if (!g_iconAdded) log_ts("Shell_NotifyIcon(NIM_ADD) failed: %lu", GetLastError());
}

// TaskbarCreated is broadcast to TOP-LEVEL windows only, so our message-only
// window never learns that Explorer restarted and took the icon with it. Poll
// instead: NIM_MODIFY fails once the icon is gone. Also re-drives NIM_ADD if it
// failed earlier, so a transient shell hiccup does not leave a camera-holding
// process with no UI at all.
static void tray_health_check() {
    if (g_shutdownDone) return;
    if (!g_iconAdded) {
        tray_add();
        if (g_iconAdded) log_ts("Tray icon (re-)added.");
        return;
    }
    g_nid.uFlags = NIF_ICON | NIF_TIP | NIF_MESSAGE;
    if (!Shell_NotifyIconW(NIM_MODIFY, &g_nid)) {
        log_ts("Tray icon has disappeared (NIM_MODIFY failed); re-adding.");
        g_iconAdded = false;
        tray_add();
    }
}

static void tray_balloon(const wchar_t *title, const wchar_t *text) {
    if (!g_iconAdded) return;
    NOTIFYICONDATAW nid = g_nid;
    nid.uFlags      = NIF_INFO;
    nid.dwInfoFlags = NIIF_INFO;
    wcsncpy(nid.szInfoTitle, title, 63); nid.szInfoTitle[63] = 0;
    wcsncpy(nid.szInfo,      text, 255); nid.szInfo[255]     = 0;
    Shell_NotifyIconW(NIM_MODIFY, &nid);
}

static void balloon_for_state(HelperState s) {
    wchar_t msg[256];
    switch (s) {
        case HelperState::Running:
            _snwprintf(msg, 255, L"Capture running. Test page: http://localhost:%d/", g_port);
            msg[255] = 0;
            tray_balloon(L"Dermoscope Helper", msg);
            break;
        case HelperState::DeviceNotFound:
            tray_balloon(L"Dermoscope Helper",
                         L"Dermoscope not found. Plug it in - retrying automatically.");
            break;
        case HelperState::CameraBusy:
            tray_balloon(L"Dermoscope Helper",
                         L"Camera is in use by another application. Close it and press Start.");
            break;
        case HelperState::Error:
            tray_balloon(L"Dermoscope Helper",
                         L"Failed to start. See helper.log next to helper.exe.");
            break;
        default:
            tray_balloon(L"Dermoscope Helper", L"Capture stopped. The camera is free.");
            break;
    }
}

// Auto-retry exists only for DeviceNotFound (waiting for the user to plug the
// dermoscope in). We never auto-retry CameraBusy -- that would fight whatever
// app currently owns the camera -- nor after an explicit Stop.
static void retry_timer_set(bool on) {
    if (!g_hwnd) return;
    if (on) SetTimer(g_hwnd, IDT_RETRY, RETRY_INTERVAL_MS, NULL);
    else    KillTimer(g_hwnd, IDT_RETRY);
}

// start/stop block the message loop for seconds. Clicks the user made during
// that window are dispatched afterwards, against the NEW state -- so clicks
// queued during a Stop would call do_start() and snatch the camera straight
// back from whatever app the user was releasing it for (and vice versa).
// Discard tray input that arrived while we were busy; the user's intent was
// expressed by the click we already acted on.
static void drain_tray_input() {
    if (!g_hwnd) return;
    MSG m;
    while (PeekMessageW(&m, g_hwnd, WM_TRAYICON, WM_TRAYICON, PM_REMOVE)) {}
}

static void do_start(bool balloon) {
    // Shutdown is final: never rebuild the graph once teardown has begun.
    if (g_shutdownDone) return;
    HelperState s = start_capture();
    tray_update();
    if (balloon) balloon_for_state(s);
    retry_timer_set(s == HelperState::DeviceNotFound);
    drain_tray_input();
}

static void do_stop(bool balloon) {
    retry_timer_set(false);
    stop_capture();
    tray_update();
    if (balloon) balloon_for_state(HelperState::Stopped);
    drain_tray_input();
}

static void tray_shutdown() {
    if (g_shutdownDone) return;
    g_shutdownDone = true;
    retry_timer_set(false);
    if (g_hwnd) KillTimer(g_hwnd, IDT_ICONCHECK);
    // Remove the icon FIRST: stop_capture() takes seconds, and an icon that is
    // still clickable during teardown just queues messages we have to discard.
    if (g_iconAdded) {
        Shell_NotifyIconW(NIM_DELETE, &g_nid);
        g_iconAdded = false;
    }
    stop_capture();
}

static void show_menu(HWND hwnd) {
    HMENU menu = CreatePopupMenu();
    if (!menu) return;
    bool running = (g_state.load() == HelperState::Running);
    AppendMenuW(menu, MF_STRING, IDM_START,    L"Start");
    AppendMenuW(menu, MF_STRING, IDM_STOP,     L"Stop");
    AppendMenuW(menu, MF_SEPARATOR, 0, NULL);
    AppendMenuW(menu, MF_STRING, IDM_OPENPAGE, L"Open test page");
    AppendMenuW(menu, MF_SEPARATOR, 0, NULL);
    AppendMenuW(menu, MF_STRING, IDM_EXIT,     L"Exit");
    EnableMenuItem(menu, IDM_START, MF_BYCOMMAND | (running ? MF_GRAYED : MF_ENABLED));
    EnableMenuItem(menu, IDM_STOP,  MF_BYCOMMAND | (running ? MF_ENABLED : MF_GRAYED));

    POINT pt; GetCursorPos(&pt);
    SetForegroundWindow(hwnd);                    // standard tray-menu workaround
    int cmd = TrackPopupMenu(menu, TPM_RIGHTBUTTON | TPM_RETURNCMD | TPM_NONOTIFY,
                             pt.x, pt.y, 0, hwnd, NULL);
    PostMessage(hwnd, WM_NULL, 0, 0);             // ...and its other half
    DestroyMenu(menu);

    switch (cmd) {
        case IDM_START: do_start(true); break;
        case IDM_STOP:  do_stop(true);  break;
        case IDM_OPENPAGE: {
            wchar_t url[64];
            _snwprintf(url, 63, L"http://localhost:%d/", g_port);
            url[63] = 0;
            ShellExecuteW(NULL, L"open", url, NULL, NULL, SW_SHOWNORMAL);
            break;
        }
        case IDM_EXIT:
            // DestroyWindow (not a bare tray_shutdown + PostQuitMessage): it
            // sends WM_DESTROY, which does the shutdown, and it takes the window
            // out of the message queue so nothing queued during the multi-second
            // teardown can be dispatched afterwards and restart capture.
            DestroyWindow(hwnd);
            break;
        default: break;
    }
}

static LRESULT CALLBACK wnd_proc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
    if (msg == g_wmTaskbarCreated && g_wmTaskbarCreated != 0) {
        // Explorer restarted -- our icon went with it. A message-only window is
        // NOT in the broadcast set, so in practice IDT_ICONCHECK is what
        // notices; this stays for correctness if the window type ever changes.
        if (g_shutdownDone) return 0;
        g_iconAdded = false;
        tray_add();
        return 0;
    }
    if (msg == g_wmAlreadyRunning && g_wmAlreadyRunning != 0) {
        tray_balloon(L"Dermoscope Helper", L"Dermoscope Helper is already running.");
        return 0;
    }
    if (msg == WM_SERVERDOWN) {
        if (g_shutdownDone) return 0;
        // The accept loop died and already cleared g_serverRunning. Say so,
        // so Start is offered again instead of a tooltip that claims "running".
        g_state.store(HelperState::Error);
        tray_update();
        tray_balloon(L"Dermoscope Helper",
                     L"The local HTTP server stopped unexpectedly. "
                     L"Press Start to restart it; see helper.log.");
        return 0;
    }
    if (msg == WM_GRAPHEVENT) {
        if (g_shutdownDone) return 0;
        IMediaEventEx *pME = g_cap.pME;
        if (!pME) return 0;
        bool lost = false;
        long evCode = 0;
        LONG_PTR p1 = 0, p2 = 0;
        // Must drain fully: the graph only posts another WM_GRAPHEVENT once the
        // queue has been emptied.
        while (SUCCEEDED(pME->GetEvent(&evCode, &p1, &p2, 0))) {
            // EC_DEVICE_LOST also carries lParam2 == 1 for "the device is back",
            // but only for filters registered via IAMDeviceRemoval, which we are
            // not. Treating every EC_DEVICE_LOST as a loss therefore fails safe:
            // a spurious stop re-arms the retry timer and recovers by itself,
            // whereas a missed removal leaves the tooltip lying -- the exact bug
            // this handler exists to fix.
            if (evCode == EC_DEVICE_LOST || evCode == EC_ERRORABORT) {
                log_ts("DirectShow event 0x%02lX (device lost / abort).", (unsigned long)evCode);
                lost = true;
            }
            pME->FreeEventParams(evCode, p1, p2);
        }
        if (lost) {
            log_ts("Dermoscope disappeared while running; stopping and waiting for it to return.");
            stop_capture();                          // releases pME too
            g_state.store(HelperState::DeviceNotFound);
            tray_update();
            balloon_for_state(HelperState::DeviceNotFound);
            retry_timer_set(true);                   // so replugging recovers
        }
        return 0;
    }

    switch (msg) {
        case WM_TRAYICON:
            if (g_shutdownDone) return 0;   // shutdown is final
            if (LOWORD(lp) == WM_RBUTTONUP) {
                show_menu(hwnd);
            } else if (LOWORD(lp) == WM_LBUTTONDBLCLK) {
                if (g_state.load() == HelperState::Running) do_stop(true);
                else                                        do_start(true);
            }
            return 0;

        case WM_TIMER:
            if (g_shutdownDone) return 0;   // shutdown is final
            if (wp == IDT_ICONCHECK) {
                tray_health_check();
                return 0;
            }
            if (wp == IDT_RETRY) {
                if (g_state.load() != HelperState::DeviceNotFound) {
                    retry_timer_set(false);
                    return 0;
                }
                log_ts("Auto-retry: looking for the dermoscope again...");
                HelperState s = start_capture();
                tray_update();
                if (s != HelperState::DeviceNotFound) {
                    retry_timer_set(false);
                    balloon_for_state(s);   // only the outcome, never each attempt
                }
                return 0;
            }
            break;

        case WM_CLOSE:
            tray_shutdown();
            DestroyWindow(hwnd);
            return 0;

        case WM_ENDSESSION:
            if (wp) {                       // session really is ending
                tray_shutdown();
                g_cleanExitDone.store(true);
                PostQuitMessage(0);         // unwind rather than wait to be killed
            }
            return 0;

        case WM_DESTROY:
            tray_shutdown();
            PostQuitMessage(0);
            return 0;

        default: break;
    }
    return DefWindowProcW(hwnd, msg, wp, lp);
}

// Console mode only: Ctrl-C / console close must release the camera instead of
// hard-killing us, so we post WM_CLOSE and let the message loop unwind.
static BOOL WINAPI console_ctrl_handler(DWORD type) {
    switch (type) {
        case CTRL_C_EVENT:
        case CTRL_BREAK_EVENT:
        case CTRL_CLOSE_EVENT:
        case CTRL_LOGOFF_EVENT:
        case CTRL_SHUTDOWN_EVENT:
            if (g_hwnd) PostMessageW(g_hwnd, WM_CLOSE, 0, 0);
            // The system may terminate us as soon as we return for the
            // close/logoff/shutdown events; give the clean exit a moment.
            for (int i = 0; i < 400 && !g_cleanExitDone.load(); ++i) Sleep(10);
            return TRUE;
        default:
            return FALSE;
    }
}

int main(int argc, char **argv) {
    // Positional args as before (port, debounce_ms); flags may appear anywhere
    // and never consume a positional slot. Unknown --flags are ignored.
    bool consoleMode = false;
    bool badDebounce = false;     // logged once the log destination is known
    int positional = 0;
    for (int i = 1; i < argc; ++i) {
        if (argv[i][0] == '-' && argv[i][1] == '-') {
            if (strcmp(argv[i], "--console") == 0) consoleMode = true;
            continue;
        }
        if (positional == 0) {
            g_port = atoi(argv[i]);
        } else if (positional == 1) {
            // Validate like the port next to it. A negative value cast straight
            // to DWORD wraps to a ~49-day debounce, which silently disables the
            // hardware button for good while the tooltip still says "running".
            char *end = NULL;
            long d = strtol(argv[i], &end, 10);
            if (end != argv[i] && d >= 0 && d <= 60000) g_debounce_ms = (DWORD)d;
            else badDebounce = true;
        }
        ++positional;
    }
    if (g_port <= 0 || g_port > 65535) g_port = 8080;

    g_wmTaskbarCreated = RegisterWindowMessageW(L"TaskbarCreated");
    g_wmAlreadyRunning = RegisterWindowMessageW(L"DermoscopeHelperAlreadyRunning");

    // Single instance. Local\ is right: SendInput is per-session anyway.
    HANDLE hMutex = CreateMutexW(NULL, TRUE, SINGLE_INSTANCE_MUTEX_NAME);
    if (hMutex && GetLastError() == ERROR_ALREADY_EXISTS) {
        HWND other = FindWindowExW(HWND_MESSAGE, NULL, TRAY_WND_CLASS, NULL);
        if (other) PostMessageW(other, g_wmAlreadyRunning, 0, 0);
        CloseHandle(hMutex);
        return 0;
    }

    if (consoleMode) {
        g_logFp = stderr;
    } else {
        FreeConsole();            // stderr's handle is dead from here on
        open_log_file();
    }
    log_ts("Config: port=%d debounce_ms=%lu mode=%s",
           g_port, g_debounce_ms, consoleMode ? "console" : "tray");
    if (badDebounce)
        log_ts("Ignored an out-of-range debounce_ms argument (allowed 0..60000); "
               "using %lu.", g_debounce_ms);

    WSADATA wsa;
    int wsaErr = WSAStartup(MAKEWORD(2,2), &wsa);
    if (wsaErr != 0) {
        // Without Winsock there is no HTTP server, so there is nothing useful to
        // run -- and WSACleanup() must not be called for a failed WSAStartup().
        log_ts("WSAStartup failed: %d -- cannot serve HTTP; exiting.", wsaErr);
        if (hMutex) { ReleaseMutex(hMutex); CloseHandle(hMutex); }
        return 2;
    }

    CoInitializeEx(NULL, COINIT_MULTITHREADED);

    WNDCLASSEXW wc = {};
    wc.cbSize        = sizeof(wc);
    wc.lpfnWndProc   = wnd_proc;
    wc.hInstance     = GetModuleHandleW(NULL);
    wc.lpszClassName = TRAY_WND_CLASS;
    RegisterClassExW(&wc);
    g_hwnd = CreateWindowExW(0, TRAY_WND_CLASS, L"Dermoscope Helper", 0,
                             0, 0, 0, 0, HWND_MESSAGE, NULL, wc.hInstance, NULL);
    if (!g_hwnd) {
        log_ts("CreateWindowEx failed: %lu", GetLastError());
        WSACleanup();
        CoUninitialize();
        if (hMutex) { ReleaseMutex(hMutex); CloseHandle(hMutex); }
        return 3;
    }

    load_icons();
    tray_add();
    // Without an icon there is no menu, no Stop and no Exit -- the helper would
    // hold the camera and the port invisibly, unkillable except via Task
    // Manager and unrelaunchable because of the single-instance mutex. Retry a
    // few times (the shell can be briefly unready at logon), then refuse to run
    // rather than auto-start blind.
    for (int i = 0; i < 5 && !g_iconAdded; ++i) {
        Sleep(1000);
        tray_add();
    }
    if (!g_iconAdded) {
        log_ts("Could not add the tray icon after 6 attempts; refusing to run "
               "invisibly while holding the camera. Exiting.");
        DestroyWindow(g_hwnd);
        g_hwnd = NULL;
        if (g_iconGreyOwned && g_iconGrey) DestroyIcon(g_iconGrey);
        g_iconGrey = NULL;
        WSACleanup();
        CoUninitialize();
        if (hMutex) { ReleaseMutex(hMutex); CloseHandle(hMutex); }
        return 4;
    }
    SetTimer(g_hwnd, IDT_ICONCHECK, ICON_CHECK_INTERVAL_MS, NULL);

    if (consoleMode) SetConsoleCtrlHandler(console_ctrl_handler, TRUE);

    do_start(true);               // auto-start; balloon reports the outcome

    MSG msg;
    while (GetMessageW(&msg, NULL, 0, 0) > 0) {
        TranslateMessage(&msg);
        DispatchMessageW(&msg);
    }

    tray_shutdown();              // no-op if the window already did it

    // If the 2 s drain gave up, a detached client thread is still alive. Letting
    // main return would run static destructors on g_clientMu, g_clientSocks and
    // the log FILE* while that thread is still using all three -- undefined
    // behaviour, i.e. a crash or hang on exit. Everything that matters (camera
    // released, icon removed) has already happened in tray_shutdown(), and
    // log_ts flushes every line, so ending the process outright is safe and
    // strictly better than the race.
    if (g_clientThreads.load() > 0) {
        log_ts("Exiting with %d HTTP client thread(s) still live; "
               "ending the process without static teardown.", g_clientThreads.load());
        if (hMutex) CloseHandle(hMutex);   // the kernel drops the mutex with us
        _exit(0);
    }

    if (g_iconGreyOwned && g_iconGrey) DestroyIcon(g_iconGrey);
    g_iconGrey = NULL;
    WSACleanup();
    CoUninitialize();
    if (hMutex) { ReleaseMutex(hMutex); CloseHandle(hMutex); }
    g_cleanExitDone.store(true);
    log_ts("Exited cleanly.");
    return 0;
}
