// Unit tests for mjpeg_first_frame_size(). Build and run with `make test`.
#include "mjpeg_frame.h"

#include <stdio.h>
#include <string.h>
#include <vector>

static int g_failures = 0;

static void expect(const char *name, const std::vector<unsigned char> &buf, size_t want) {
    size_t got = mjpeg_first_frame_size(buf.empty() ? NULL : buf.data(), buf.size());
    if (got != want) {
        printf("FAIL %-42s want %zu, got %zu\n", name, want, got);
        ++g_failures;
    } else {
        printf("ok   %-42s %zu\n", name, got);
    }
}

// SOI + a DQT-shaped segment + SOS + `scan` bytes + EOI, so the tests exercise
// the marker walk rather than a two-byte degenerate buffer.
static std::vector<unsigned char> frame(const std::vector<unsigned char> &scan,
                                        bool eoi = true) {
    std::vector<unsigned char> f = {0xFF, 0xD8, 0xFF, 0xDB, 0x00, 0x04, 0x11, 0x22,
                                    0xFF, 0xDA, 0x00, 0x03, 0x01};
    f.insert(f.end(), scan.begin(), scan.end());
    if (eoi) { f.push_back(0xFF); f.push_back(0xD9); }
    return f;
}

static std::vector<unsigned char> cat(std::vector<unsigned char> a,
                                      const std::vector<unsigned char> &b) {
    a.insert(a.end(), b.begin(), b.end());
    return a;
}

int main(void) {
    const std::vector<unsigned char> scan = {0x01, 0x02, 0x03, 0x04};

    // --- well-formed input ---
    std::vector<unsigned char> ok = frame(scan);
    expect("single complete frame", ok, ok.size());

    // Minimum accepted buffer: SOI immediately followed by EOI.
    expect("SOI+EOI only", {0xFF, 0xD8, 0xFF, 0xD9}, 4);

    // --- the bug this function exists for ---
    // Truncated preview frame (no EOI) spliced onto a full still. Serving this
    // raw is what produced /snapshot bodies with two SOI markers.
    expect("splice: unterminated frame + second SOI",
           cat(frame(scan, /*eoi=*/false), frame(scan)), 0);

    // A terminated frame followed by another one: keep only the first.
    std::vector<unsigned char> first = frame(scan);
    expect("splice: complete frame + second frame",
           cat(first, frame(scan)), first.size());

    // --- trailing bytes after EOI are dropped ---
    // The device appends a stray 0xD9 to every frame it emits (FF D9 D9).
    expect("stray 0xD9 after EOI", cat(ok, {0xD9}), ok.size());
    expect("zero padding after EOI", cat(ok, {0x00, 0x00, 0x00}), ok.size());

    // --- bytes that must NOT be mistaken for EOI ---
    // A literal 0xFF in entropy-coded data is stuffed as FF 00; the 0x00 must
    // not be read as a marker, and an FF D9 spanning the stuffed pair must not
    // terminate the frame early.
    std::vector<unsigned char> stuffed = frame({0xFF, 0x00, 0xD9, 0x11});
    expect("byte-stuffed FF 00 in scan", stuffed, stuffed.size());

    std::vector<unsigned char> restarts = frame({0xFF, 0xD0, 0x11, 0xFF, 0xD7, 0x22});
    expect("restart markers FF D0..FF D7", restarts, restarts.size());

    std::vector<unsigned char> fill = frame({0xFF, 0xFF, 0x11});
    expect("fill bytes FF FF", fill, fill.size());

    // FF FF D9 is a fill byte followed by a real EOI, not two fill bytes.
    expect("fill byte immediately before EOI",
           {0xFF, 0xD8, 0x11, 0xFF, 0xFF, 0xD9}, 6);

    // --- rejected input ---
    expect("no SOI", {0x00, 0x11, 0x22, 0xFF, 0xD9}, 0);
    expect("SOI but no EOI", frame(scan, /*eoi=*/false), 0);
    expect("truncated: trailing lone FF", cat(frame(scan, false), {0xFF}), 0);
    expect("too short", {0xFF, 0xD8, 0xFF}, 0);
    expect("empty", {}, 0);

    if (mjpeg_first_frame_size(NULL, 1024) != 0) {
        printf("FAIL %-42s want 0\n", "null buffer");
        ++g_failures;
    } else {
        printf("ok   %-42s 0\n", "null buffer");
    }

    // --- shape of a real capture ---
    // A 1024x768 preview frame is ~250 KB; make sure nothing in the walk trips
    // on a buffer of realistic size with FF bytes scattered through the scan.
    std::vector<unsigned char> big;
    for (size_t i = 0; i < 250000; ++i) {
        big.push_back((i % 997) == 0 ? 0xFF : (unsigned char)(i & 0xFF));
        if (big.back() == 0xFF) big.push_back(0x00);
    }
    std::vector<unsigned char> real = frame(big);
    expect("250 KB frame with stuffed FF bytes", real, real.size());
    expect("250 KB frame spliced onto a second",
           cat(frame(big, /*eoi=*/false), real), 0);

    printf("\n%s\n", g_failures ? "FAILED" : "all tests passed");
    return g_failures ? 1 : 0;
}
