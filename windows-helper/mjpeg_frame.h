// Pure JPEG byte-structure parsing, split out of helper.cpp so it can be unit
// tested without linking DirectShow. Header-only and free of windows.h.
#pragma once

#include <stddef.h>

// Length of the first complete MJPEG frame in [buf, buf+len). Requires the
// buffer to start with SOI (FF D8) and to contain an EOI (FF D9) before any
// second SOI. Returns 0 if the buffer is not a well-formed single frame.
//
// The concatenated case is real: at least one UVC driver in the wild delivers
// two frames spliced end-to-end in a single BufferCB (observed: a truncated
// preview frame with no EOI followed by a full 1600x1200 still, arriving on
// the preview pin's SampleGrabber). Copying that raw into g_latestPreview
// yields /snapshot bodies with two SOI markers -- browsers stop at the first
// scan and display, but strict decoders (Pillow) reject it. Trimming here also
// drops the stray fill byte some devices append after EOI, so JPEGs served
// downstream end cleanly at FF D9.
//
// Known limitation: segment payloads are scanned rather than skipped by their
// length field, so a literal FF D9 inside e.g. a DQT or APPn payload would end
// the frame early. Entropy-coded scan data cannot trigger this (a literal FF is
// always stuffed to FF 00), which is where the overwhelming majority of bytes
// live, and skipping segments would mean trusting length fields in a buffer we
// already know can be malformed.
static inline size_t mjpeg_first_frame_size(const unsigned char *buf, size_t len) {
    if (!buf || len < 4 || buf[0] != 0xFF || buf[1] != 0xD8) return 0;
    for (size_t i = 2; i + 1 < len; ++i) {
        if (buf[i] != 0xFF) continue;
        unsigned char m = buf[i + 1];
        if (m == 0x00) continue;                    // FF-stuffed literal 0xFF in scan
        if (m == 0xFF) continue;                    // fill byte
        if (m >= 0xD0 && m <= 0xD7) continue;       // restart marker within scan
        if (m == 0xD9) return i + 2;                // EOI: end of the first frame
        if (m == 0xD8) return 0;                    // spliced second SOI before EOI
        // any other marker is legal JPEG structure; keep scanning
    }
    return 0;
}
