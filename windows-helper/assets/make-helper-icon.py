"""
make-helper-icon.py -- generates the Dermoscope Helper tray icon (helper.ico) plus
loose PNGs and a judging contact sheet, using ONLY the Python standard library
(zlib for PNG deflate/CRC, struct for binary packing, math -- no Pillow, no numpy).

STYLE: "rounded-square badge" -- a deep-indigo rounded-rect plate with a near-white
microscope glyph drawn on top. The badge is the whole point of this direction: it
gives identical contrast on light AND dark taskbars for free, because the plate
fill colour never has to fight the desktop behind it (a bare monochrome glyph
would vanish on a same-toned taskbar; a badge sidesteps the problem entirely).

RENDERING STRATEGY
-------------------
Every shape (rounded rect, "capsule" = a stroked line segment with round caps,
plain circle) is defined as an analytic inside/outside test in float coordinates.
For each output pixel we sample an NxN sub-grid of points, classify each sub-point
(transparent / badge colour / glyph colour) against the shape list, and average
the samples with premultiplied alpha. That average IS the anti-aliased pixel --
no external AA library needed, and it is exact (not a blurred-then-thresholded
approximation) because the shape tests themselves are exact.

SMALL-SIZE STRATEGY (the part that actually matters for a tray icon)
----------------------------------------------------------------------
A microscope has a lot of thin structure (eyepiece tube, objective, stage bar,
diagonal arm). Naively downscaling the 256px artwork to 16px turns all of that
into grey mush -- individual strokes get thinner than a pixel and disappear.
Instead, sizes <= 24px use a SEPARATE, simplified geometry (SIMPLE_SHAPES):
fewer strokes, drawn fatter, with the specimen-dot cutout and the separate
objective tube dropped entirely (they are the first details to die at small
sizes, so we remove them deliberately rather than let them turn into noise).
Sizes >= 32px use the full geometry (DETAILED_SHAPES) with all the detail.
"""

import os
import struct
import sys
import zlib

# ---------------------------------------------------------------------------
# Palette. Deep indigo/navy plate, near-white glyph -- chosen so the PLATE
# (not the glyph) is what has to read against the taskbar, and a solid navy
# square reads fine on both a light (#F3F3F3) and dark (#202020) Windows 11
# taskbar. The glyph then just needs contrast against the plate, which is
# fixed and known, so it can go all the way to near-white.
# ---------------------------------------------------------------------------
BADGE = (0x23, 0x39, 0x5B, 255)   # #23395B deep indigo/navy
GLYPH = (0xF7, 0xFA, 0xFC, 255)   # #F7FAFC near-white

# Icon sizes required by the .ico container / Windows shell.
SIZES = [16, 24, 32, 48, 64, 128, 256]
SMALL_CUTOFF = 32  # sizes strictly below this use the simplified geometry

# ---------------------------------------------------------------------------
# Geometry constants, all expressed in a local 0..100 square ("glyph space").
# This square is later mapped into an inset box on the actual canvas (see
# layout_for_size), so these numbers never need to know the final pixel size.
#
# DETAILED_SHAPES draws a recognisable microscope silhouette: a foot-shaped
# base, a backward-leaning column, a body "hub", an objective tube pointing
# down at the stage, a diagonal arm up to the eyepiece tube, a stage bar, and
# (size permitting) a knocked-out specimen dot sitting on the stage.
#
# Shapes are ("capsule", x0,y0,x1,y1,r) for a stroked line with round caps,
# or ("circle", cx,cy,r) for a filled disc. Round caps are why capsules that
# share an endpoint and radius join seamlessly (each already paints a full
# round cap at the shared point) -- no separate miter/joint logic needed.
# ---------------------------------------------------------------------------
# NOTE on shape: the eyepiece is a STRAIGHT vertical tube with a horizontal
# cap (not a curved neck down to the arm) and the arm is a SINGLE straight
# diagonal from that tube to the hub. An early draft bent the eyepiece->hub
# path through a soft curve and it read as a swan/flamingo, not an optical
# instrument -- straight segments meeting at sharp-ish vertices is what
# makes this mechanical instead of organic. From the hub the silhouette
# forks like a real microscope's arm: one tube drops straight down to the
# stage (the objective), another drops diagonally to the base (the stand).
DETAILED_SHAPES = [
    ("capsule", 14, 90, 86, 90, 9.0),   # base foot
    ("capsule", 64, 45, 42, 88, 6.5),   # column: hub down to the base
    ("capsule", 64, 45, 68, 63, 6.0),   # objective tube: hub down to the stage
    ("capsule", 38, 26, 64, 45, 6.5),   # diagonal arm: eyepiece bend to hub
    ("capsule", 38, 6, 38, 26, 6.0),    # eyepiece tube (straight, vertical)
    ("capsule", 30, 6, 46, 6, 5.5),     # eyepiece rim cap (horizontal)
    ("capsule", 18, 66, 78, 66, 6.5),   # stage bar
    ("circle", 64, 45, 7.0),            # hub joint, same scale as the arms
]
# Specimen dot: punched OUT of the badge colour on top of the stage bar, i.e.
# rendered in BADGE after the glyph shapes so it reads as a hole, not a blob
# the same colour as everything else. Only used at 48px and above -- below
# that its diameter rounds to noise, so it is left out rather than mushed.
DETAILED_CUTOUT = ("circle", 68, 66, 4.5)
DETAILED_CUTOUT_MIN_SIZE = 48

# Simplified geometry for 16/24px: four fat strokes plus one head circle, no
# objective tube, no eyepiece rim, no cutout dot. Still a single straight
# diagonal (not a curve) from the head to the hub-ish bend point, then a
# second straight segment down to the base -- a plain "V", never an S, so it
# keeps reading as a rigid instrument even once it is only a few pixels wide.
SIMPLE_SHAPES = [
    ("capsule", 16, 89, 84, 89, 12.5),  # base foot
    ("capsule", 58, 50, 50, 86, 12.5),  # column: bend point down to base
    ("capsule", 40, 20, 58, 50, 13.0),  # diagonal arm: head to bend point
    ("capsule", 16, 58, 72, 58, 10.5),  # stage bar
    ("circle", 40, 20, 15.0),           # fused eyepiece/head knob
]

# Badge (plate) geometry, as fractions of the canvas side.
BADGE_MARGIN_FRAC = 0.035   # plate inset from the canvas edge
BADGE_RADIUS_FRAC = 0.18    # corner radius, as a fraction of the PLATE side
GLYPH_MARGIN_FRAC = 0.14    # glyph-box inset from the canvas edge (per spec)


def layout_for_size(size):
    """Returns (badge_rect, badge_radius, glyph_origin, glyph_scale) in pixel
    coordinates for a canvas of size x size."""
    bm = BADGE_MARGIN_FRAC * size
    badge_rect = (bm, bm, size - bm, size - bm)
    badge_radius = BADGE_RADIUS_FRAC * (size - 2 * bm)
    gm = GLYPH_MARGIN_FRAC * size
    glyph_box_side = size - 2 * gm
    glyph_origin = (gm, gm)
    glyph_scale = glyph_box_side / 100.0
    return badge_rect, badge_radius, glyph_origin, glyph_scale


def shapes_for_size(size):
    """Picks geometry by size, and maps glyph-space coordinates into canvas
    pixel space. Returns (glyph_shapes, cutout_shapes)."""
    _, _, (ox, oy), scale = layout_for_size(size)

    def to_canvas(shape):
        kind = shape[0]
        if kind == "capsule":
            _, x0, y0, x1, y1, r = shape
            return ("capsule", ox + x0 * scale, oy + y0 * scale,
                     ox + x1 * scale, oy + y1 * scale, r * scale)
        else:
            _, cx, cy, r = shape
            return ("circle", ox + cx * scale, oy + cy * scale, r * scale)

    if size < SMALL_CUTOFF:
        glyph = [to_canvas(s) for s in SIMPLE_SHAPES]
        cutouts = []
    else:
        glyph = [to_canvas(s) for s in DETAILED_SHAPES]
        cutouts = []
        if size >= DETAILED_CUTOUT_MIN_SIZE:
            cutouts = [to_canvas(DETAILED_CUTOUT)]
    return glyph, cutouts


# ---------------------------------------------------------------------------
# Analytic shape tests (all in canvas pixel coordinates).
# ---------------------------------------------------------------------------
def in_rounded_rect(x, y, x0, y0, x1, y1, r):
    if x < x0 or x > x1 or y < y0 or y > y1:
        return False
    if x < x0 + r and y < y0 + r:
        dx, dy = x - (x0 + r), y - (y0 + r)
        return dx * dx + dy * dy <= r * r
    if x > x1 - r and y < y0 + r:
        dx, dy = x - (x1 - r), y - (y0 + r)
        return dx * dx + dy * dy <= r * r
    if x < x0 + r and y > y1 - r:
        dx, dy = x - (x0 + r), y - (y1 - r)
        return dx * dx + dy * dy <= r * r
    if x > x1 - r and y > y1 - r:
        dx, dy = x - (x1 - r), y - (y1 - r)
        return dx * dx + dy * dy <= r * r
    return True


def in_capsule(x, y, ax, ay, bx, by, r):
    dx, dy = bx - ax, by - ay
    l2 = dx * dx + dy * dy
    if l2 == 0:
        t = 0.0
    else:
        t = ((x - ax) * dx + (y - ay) * dy) / l2
        if t < 0.0:
            t = 0.0
        elif t > 1.0:
            t = 1.0
    px, py = ax + t * dx, ay + t * dy
    ex, ey = x - px, y - py
    return ex * ex + ey * ey <= r * r


def in_circle(x, y, cx, cy, r):
    dx, dy = x - cx, y - cy
    return dx * dx + dy * dy <= r * r


def point_in_shape(x, y, shape):
    if shape[0] == "capsule":
        _, ax, ay, bx, by, r = shape
        return in_capsule(x, y, ax, ay, bx, by, r)
    else:
        _, cx, cy, r = shape
        return in_circle(x, y, cx, cy, r)


# ---------------------------------------------------------------------------
# Rasterizer: supersample each output pixel, classify sub-points, average
# with premultiplied alpha (every sub-point is either fully transparent or
# fully opaque, so the premultiplied sum is simply "sum of colour over the
# opaque sub-points", divided by the opaque count -- this is exact box-filter
# downsampling, not a blur).
# ---------------------------------------------------------------------------
def supersample_factor(size):
    if size <= 32:
        return 12
    if size <= 64:
        return 8
    return 4


def render_rgba(size):
    badge_rect, badge_radius, _, _ = layout_for_size(size)
    glyph_shapes, cutout_shapes = shapes_for_size(size)
    bx0, by0, bx1, by1 = badge_rect
    ss = supersample_factor(size)
    inv_ss = 1.0 / ss
    total = ss * ss

    rows = []
    for py in range(size):
        row = bytearray(size * 4)
        base_y = py + 0.5 * inv_ss
        for px in range(size):
            sum_r = sum_g = sum_b = 0
            opaque = 0
            base_x = px + 0.5 * inv_ss
            for j in range(ss):
                sy = base_y + j * inv_ss
                for i in range(ss):
                    sx = base_x + i * inv_ss
                    if not in_rounded_rect(sx, sy, bx0, by0, bx1, by1, badge_radius):
                        continue
                    opaque += 1
                    colour = BADGE
                    for shp in glyph_shapes:
                        if point_in_shape(sx, sy, shp):
                            colour = GLYPH
                            break
                    for shp in cutout_shapes:
                        if point_in_shape(sx, sy, shp):
                            colour = BADGE
                            break
                    sum_r += colour[0]
                    sum_g += colour[1]
                    sum_b += colour[2]
            if opaque:
                r = sum_r // opaque
                g = sum_g // opaque
                b = sum_b // opaque
                a = (opaque * 255) // total
            else:
                r = g = b = a = 0
            o = px * 4
            row[o] = r
            row[o + 1] = g
            row[o + 2] = b
            row[o + 3] = a
        rows.append(bytes(row))
    return rows  # list of `size` byte-strings, each size*4 bytes (RGBA rows)


# ---------------------------------------------------------------------------
# Minimal PNG encoder: 8-bit RGBA, colour type 6, single IDAT, filter 0 (None)
# on every scanline. Good enough quality here (no dithering needed) and keeps
# the encoder tiny.
# ---------------------------------------------------------------------------
def _chunk(tag, data):
    return (struct.pack(">I", len(data)) + tag + data +
            struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF))


def encode_png_rows(rows, width, height):
    sig = b"\x89PNG\r\n\x1a\n"
    ihdr = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    raw = bytearray()
    for row in rows:
        raw.append(0)  # filter type 0 = None
        raw += row
    idat = zlib.compress(bytes(raw), 9)
    return sig + _chunk(b"IHDR", ihdr) + _chunk(b"IDAT", idat) + _chunk(b"IEND", b"")


def encode_png(rows, size):
    return encode_png_rows(rows, size, size)


# ---------------------------------------------------------------------------
# Minimal .ico container: ICONDIR + one ICONDIRENTRY per image, each entry
# pointing at a PNG-compressed frame (supported since Vista, correct choice
# for every size shipped here -- no need for legacy uncompressed DIB frames).
# ---------------------------------------------------------------------------
def encode_ico(png_by_size):
    sizes = sorted(png_by_size.keys())
    count = len(sizes)
    header = struct.pack("<HHH", 0, 1, count)
    entries = b""
    offset = 6 + 16 * count
    images = b""
    for size in sizes:
        data = png_by_size[size]
        wh = size if size < 256 else 0  # 0 means 256 in the ICO format
        entries += struct.pack("<BBBBHHII", wh, wh, 0, 0, 1, 32, len(data), offset)
        images += data
        offset += len(data)
    return header + entries + images


# ---------------------------------------------------------------------------
# Tiny 5x7 bitmap digit font, used only to label the judging contact sheet.
# ---------------------------------------------------------------------------
_DIGIT_ROWS = {
    "0": ["01110", "10001", "10011", "10101", "11001", "10001", "01110"],
    "1": ["00100", "01100", "00100", "00100", "00100", "00100", "01110"],
    "2": ["01110", "10001", "00001", "00010", "00100", "01000", "11111"],
    "3": ["11110", "00001", "00001", "01110", "00001", "00001", "11110"],
    "4": ["00010", "00110", "01010", "10010", "11111", "00010", "00010"],
    "5": ["11111", "10000", "11110", "00001", "00001", "10001", "01110"],
    "6": ["00110", "01000", "10000", "11110", "10001", "10001", "01110"],
    "7": ["11111", "00001", "00010", "00100", "01000", "01000", "01000"],
    "8": ["01110", "10001", "10001", "01110", "10001", "10001", "01110"],
    "9": ["01110", "10001", "10001", "01111", "00001", "00010", "01100"],
}


def draw_text(canvas, x, y, text, scale, colour):
    cx = x
    for ch in text:
        rows = _DIGIT_ROWS.get(ch)
        if rows is None:
            cx += 4 * scale
            continue
        for ry, rowbits in enumerate(rows):
            for rx, bit in enumerate(rowbits):
                if bit == "1":
                    fill_rect(canvas, cx + rx * scale, y + ry * scale, scale, scale, colour)
        cx += 6 * scale


# ---------------------------------------------------------------------------
# Small mutable RGBA canvas helpers used only for composing the contact sheet
# (the .ico/.png outputs go through render_rgba/encode_png above).
# ---------------------------------------------------------------------------
def new_canvas(w, h, colour=(0, 0, 0, 0)):
    row = bytes(colour) * w
    return {"w": w, "h": h, "rows": [bytearray(row) for _ in range(h)]}


def fill_rect(canvas, x, y, w, h, colour):
    x0, y0 = max(0, int(x)), max(0, int(y))
    x1, y1 = min(canvas["w"], int(x + w)), min(canvas["h"], int(y + h))
    for yy in range(y0, y1):
        row = canvas["rows"][yy]
        for xx in range(x0, x1):
            o = xx * 4
            row[o:o + 4] = bytes(colour)


def blit(canvas, dst_x, dst_y, rgba_rows, src_size, scale=1):
    """Nearest-neighbour blit of an rgba_rows image (src_size x src_size) at
    integer `scale`, composited with simple alpha-over onto `canvas`."""
    for sy in range(src_size):
        src_row = rgba_rows[sy]
        for sx in range(src_size):
            o = sx * 4
            r, g, b, a = src_row[o], src_row[o + 1], src_row[o + 2], src_row[o + 3]
            if a == 0:
                continue
            for ry in range(scale):
                dy = dst_y + sy * scale + ry
                if dy < 0 or dy >= canvas["h"]:
                    continue
                row = canvas["rows"][dy]
                for rx in range(scale):
                    dx = dst_x + sx * scale + rx
                    if dx < 0 or dx >= canvas["w"]:
                        continue
                    do = dx * 4
                    if a == 255:
                        row[do:do + 4] = bytes((r, g, b, 255))
                    else:
                        dr, dg, db, da = row[do], row[do + 1], row[do + 2], row[do + 3]
                        na = a + da * (255 - a) // 255
                        if na == 0:
                            row[do:do + 4] = bytes((0, 0, 0, 0))
                        else:
                            nr = (r * a + dr * da * (255 - a) // 255) // na
                            ng = (g * a + dg * da * (255 - a) // 255) // na
                            nb = (b * a + db * da * (255 - a) // 255) // na
                            row[do:do + 4] = bytes((nr, ng, nb, na))


def canvas_to_rows(canvas):
    return [bytes(r) for r in canvas["rows"]]


LIGHT_BG = (0xF3, 0xF3, 0xF3, 255)
DARK_BG = (0x20, 0x20, 0x20, 255)
INK_ON_LIGHT = (0x20, 0x20, 0x20, 255)
INK_ON_DARK = (0xF3, 0xF3, 0xF3, 255)
LABEL_INK = (0x40, 0x40, 0x40, 255)


def build_sheet(rgba_by_size):
    """Composes sheet.png: the 256px icon at 1:1, sizes 48/32/24/16 upscaled
    (nearest-neighbour) to 128px on both a light and a dark strip so a judge
    can see exactly what the tray will show, plus the true 1:1 tiny rasters."""
    pad = 40
    w = 1400

    hero_y = pad
    hero_swatch = 320
    hero_icon = 256

    row_y = hero_y + hero_swatch + 70
    swatch = 168
    icon_up = 128
    cell_w = 320
    upscale_sizes = [48, 32, 24, 16]

    native_y = row_y + swatch * 2 + 20 + 60
    native_swatch_h = 110

    h = native_y + native_swatch_h + 40 + pad

    canvas = new_canvas(w, h, (0, 0, 0, 0))

    # 1) Hero: 256px icon at 1:1 on a light swatch and a dark swatch.
    fill_rect(canvas, pad, hero_y, hero_swatch, hero_swatch, LIGHT_BG)
    blit(canvas, pad + (hero_swatch - hero_icon) // 2,
         hero_y + (hero_swatch - hero_icon) // 2, rgba_by_size[256], 256, 1)
    draw_text(canvas, pad + 10, hero_y + hero_swatch - 26, "256", 3, INK_ON_LIGHT)

    dark_x = pad + hero_swatch + 40
    fill_rect(canvas, dark_x, hero_y, hero_swatch, hero_swatch, DARK_BG)
    blit(canvas, dark_x + (hero_swatch - hero_icon) // 2,
         hero_y + (hero_swatch - hero_icon) // 2, rgba_by_size[256], 256, 1)
    draw_text(canvas, dark_x + 10, hero_y + hero_swatch - 26, "256", 3, INK_ON_DARK)

    # 2) Tray-realistic row: each small size upscaled nearest-neighbour to
    #    128px, shown on both a light and a dark strip, size labelled below.
    for idx, size in enumerate(upscale_sizes):
        cx = pad + idx * cell_w
        lx = cx + (swatch - icon_up) // 2
        fill_rect(canvas, cx, row_y, swatch, swatch, LIGHT_BG)
        blit(canvas, lx, row_y + (swatch - icon_up) // 2,
             rgba_by_size[size], size, icon_up // size)

        dy = row_y + swatch + 20
        fill_rect(canvas, cx, dy, swatch, swatch, DARK_BG)
        blit(canvas, lx, dy + (swatch - icon_up) // 2,
             rgba_by_size[size], size, icon_up // size)

        draw_text(canvas, cx + 10, dy + swatch + 14, str(size), 3, LABEL_INK)

    # 3) True 1:1 tiny rasters (16/24/32) on small light+dark chips, actual
    #    pixel size, so a judge can see them completely undistorted too.
    ny = native_y
    nx = pad
    for size in [16, 24, 32]:
        chip = size + 20
        half = native_swatch_h // 2 - 4
        fill_rect(canvas, nx, ny, chip, half, LIGHT_BG)
        blit(canvas, nx + 10, ny + (half - size) // 2, rgba_by_size[size], size, 1)
        dyy = ny + native_swatch_h // 2 + 4
        fill_rect(canvas, nx, dyy, chip, half, DARK_BG)
        blit(canvas, nx + 10, dyy + (half - size) // 2, rgba_by_size[size], size, 1)
        draw_text(canvas, nx, ny + native_swatch_h + 6, str(size), 2, LABEL_INK)
        nx += chip + 40

    return canvas_to_rows(canvas), w, h


def main():
    if len(sys.argv) < 2:
        print("usage: python make-helper-icon.py <output-dir>")
        sys.exit(1)
    out_dir = sys.argv[1]
    png_dir = os.path.join(out_dir, "png")
    os.makedirs(png_dir, exist_ok=True)

    rgba_by_size = {}
    png_by_size = {}
    for size in SIZES:
        rows = render_rgba(size)
        rgba_by_size[size] = rows
        png_bytes = encode_png(rows, size)
        png_by_size[size] = png_bytes
        with open(os.path.join(png_dir, "icon-%d.png" % size), "wb") as f:
            f.write(png_bytes)
        print("rendered %dpx (%d bytes)" % (size, len(png_bytes)))

    ico_bytes = encode_ico(png_by_size)
    with open(os.path.join(out_dir, "helper.ico"), "wb") as f:
        f.write(ico_bytes)
    print("wrote helper.ico (%d bytes)" % len(ico_bytes))

    sheet_rows, sw, sh = build_sheet(rgba_by_size)
    sheet_bytes = encode_png_rows(sheet_rows, sw, sh)
    with open(os.path.join(out_dir, "sheet.png"), "wb") as f:
        f.write(sheet_bytes)
    print("wrote sheet.png (%d bytes, %dx%d)" % (len(sheet_bytes), sw, sh))


if __name__ == "__main__":
    main()
