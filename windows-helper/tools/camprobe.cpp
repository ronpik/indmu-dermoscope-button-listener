// camprobe -- can a second process claim the dermoscope right now?
//
// Answers one question the helper's own logs cannot: is the camera actually
// free, or is something still holding it? Builds the same shape of DirectShow
// graph the helper does (source -> null renderer on the capture pin), runs it,
// and reports every HRESULT along the way.
//
// See tools/README.md for what the numbers mean and what has been established
// with this tool.
//
// Exit codes:  0 = camera available   1 = camera blocked   2 = probe error

#include <windows.h>
#include <dshow.h>
#include <stdio.h>
#include <string.h>

static const GUID CLSID_NullRenderer_qedit =
    {0xC1F400A4, 0x3F08, 0x11D3, {0x9F, 0x0B, 0x00, 0x60, 0x08, 0x03, 0x9E, 0x37}};

// Only the codes this probe has actually produced, plus the ones helper.cpp
// reacts to. HRESULT_FROM_WIN32(ERROR_NO_SYSTEM_RESOURCES) is the important
// one: it is the sole code helper.cpp's is_camera_busy_hr() treats as "another
// application has the camera", so the two agree by construction.
static const char *hresult_name(HRESULT hr) {
    switch ((unsigned long)hr) {
        case 0x00000000UL: return "S_OK";
        case 0x00000001UL: return "S_FALSE (still transitioning -- not an error)";
        case 0x800705AAUL: return "ERROR_NO_SYSTEM_RESOURCES -- CAMERA IN USE by another process";
        case 0x800700AAUL: return "ERROR_BUSY";
        case 0x80070005UL: return "E_ACCESSDENIED (privacy setting? another session?)";
        case 0x8007001FUL: return "ERROR_GEN_FAILURE (device wedged -- try a replug)";
        case 0x80040275UL: return "VFW_E_NO_CAPTURE_HARDWARE";
        case 0x80040217UL: return "VFW_E_CANNOT_CONNECT (pin/format negotiation failed)";
        case 0x80040154UL: return "REGDB_E_CLASSNOTREG (qedit.dll not registered?)";
        default:           return "";
    }
}

static void report(const char *step, HRESULT hr) {
    const char *n = hresult_name(hr);
    printf("  %-14s -> 0x%08lX%s%s\n", step, (unsigned long)hr, *n ? "  " : "", n);
}

static void rel(IUnknown *p) { if (p) p->Release(); }

int main(int argc, char **argv) {
    const char *want = "USB Camera";
    bool listOnly = false;

    for (int i = 1; i < argc; ++i) {
        if (strcmp(argv[i], "--list") == 0) {
            listOnly = true;
        } else if (strcmp(argv[i], "--help") == 0 || strcmp(argv[i], "-h") == 0) {
            printf("Usage: camprobe [--list] [device-name-substring]\n"
                   "  default device-name-substring is \"USB Camera\"\n"
                   "  --list   only enumerate capture devices, do not claim one\n"
                   "Exit: 0 = available, 1 = blocked, 2 = probe error\n");
            return 2;
        } else {
            want = argv[i];
        }
    }

    CoInitializeEx(NULL, COINIT_APARTMENTTHREADED);

    ICreateDevEnum *pDevEnum = NULL;
    IEnumMoniker   *pEnum    = NULL;
    IBaseFilter    *pSrc     = NULL;
    HRESULT hr;

    hr = CoCreateInstance(CLSID_SystemDeviceEnum, NULL, CLSCTX_INPROC_SERVER,
                          IID_ICreateDevEnum, (void **)&pDevEnum);
    if (FAILED(hr)) { report("DeviceEnum", hr); return 2; }

    hr = pDevEnum->CreateClassEnumerator(CLSID_VideoInputDeviceCategory, &pEnum, 0);
    if (hr != S_OK) {
        printf("No video capture devices present at all (hr=0x%08lX).\n", (unsigned long)hr);
        return 2;
    }

    printf("Video capture devices:\n");
    IMoniker *pMon = NULL;
    while (pEnum->Next(1, &pMon, NULL) == S_OK) {
        IPropertyBag *bag = NULL;
        char name[256] = "(unnamed)";
        if (SUCCEEDED(pMon->BindToStorage(0, 0, IID_IPropertyBag, (void **)&bag))) {
            VARIANT v; VariantInit(&v);
            if (SUCCEEDED(bag->Read(L"FriendlyName", &v, 0))) {
                WideCharToMultiByte(CP_UTF8, 0, v.bstrVal, -1, name, sizeof(name), NULL, NULL);
                VariantClear(&v);
            }
            bag->Release();
        }
        bool match = !listOnly && !pSrc && strstr(name, want) != NULL;
        printf("  %s%s\n", name, match ? "   <-- probing this one" : "");
        if (match) {
            // Binding alone rarely fails on a busy camera -- the USB stream is
            // not opened until the graph runs -- so a success here proves very
            // little. It is reported only to localise a failure if one happens.
            hr = pMon->BindToObject(0, 0, IID_IBaseFilter, (void **)&pSrc);
            report("BindToObject", hr);
        }
        rel(pMon); pMon = NULL;
    }
    rel(pEnum); rel(pDevEnum);

    if (listOnly) { CoUninitialize(); return 0; }
    if (!pSrc) {
        printf("RESULT: no device matching \"%s\".\n", want);
        CoUninitialize();
        return 2;
    }

    IGraphBuilder         *pGraph = NULL;
    ICaptureGraphBuilder2 *pBuild = NULL;
    IBaseFilter           *pNull  = NULL;
    IMediaControl         *pMC    = NULL;
    int rc = 1;

    CoCreateInstance(CLSID_FilterGraph, NULL, CLSCTX_INPROC_SERVER,
                     IID_IGraphBuilder, (void **)&pGraph);
    CoCreateInstance(CLSID_CaptureGraphBuilder2, NULL, CLSCTX_INPROC_SERVER,
                     IID_ICaptureGraphBuilder2, (void **)&pBuild);
    hr = CoCreateInstance(CLSID_NullRenderer_qedit, NULL, CLSCTX_INPROC_SERVER,
                          IID_IBaseFilter, (void **)&pNull);
    report("NullRenderer", hr);
    if (FAILED(hr)) { rc = 2; goto done; }

    pBuild->SetFiltergraph(pGraph);
    hr = pGraph->AddFilter(pSrc, L"Source");
    report("AddFilter", hr);
    pGraph->AddFilter(pNull, L"Null");

    hr = pBuild->RenderStream(&PIN_CATEGORY_CAPTURE, &MEDIATYPE_Video, pSrc, NULL, pNull);
    report("RenderStream", hr);
    if (FAILED(hr)) goto done;

    // Run() is where the USB stream is actually opened, so this is the step a
    // busy camera fails on. It legitimately returns S_FALSE while the graph is
    // still transitioning, exactly as it does inside the helper -- so GetState
    // is the arbiter, not Run()'s return value.
    pGraph->QueryInterface(IID_IMediaControl, (void **)&pMC);
    hr = pMC->Run();
    report("Run", hr);
    {
        OAFilterState fs = State_Stopped;
        HRESULT ghr = pMC->GetState(3000, &fs);
        printf("  %-14s -> 0x%08lX  state=%ld (0=Stopped 1=Paused 2=Running)\n",
               "GetState", (unsigned long)ghr, (long)fs);
        if (SUCCEEDED(hr) && ghr == S_OK && fs == State_Running) rc = 0;
    }
    pMC->Stop();

done:
    printf("RESULT: camera %s\n",
           rc == 0 ? "AVAILABLE -- claimed it successfully" :
           rc == 1 ? "BLOCKED -- could not claim it"
                   : "probe error");
    rel(pMC); rel(pNull); rel(pBuild); rel(pGraph); rel(pSrc);
    CoUninitialize();
    return rc;
}
