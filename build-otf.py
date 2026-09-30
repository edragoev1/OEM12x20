#!/usr/bin/env python3
"""Converts OEM12x20.BDF to OpenType, each pixel a square of 50 by 50 units.

    python3 build-otf.py            writes OEM12x20.otf, with CFF outlines

and then, from the pdfjet repository, the OEM12x20.otf.stream that PDFjet
embeds faster and smaller:

    util/generate-stream-fonts-files.sh ../OEM12x20

The pixels of a glyph are merged into outlines, so that no two contours
overlap or touch along an edge, and the glyphs are mapped from code page 437
to Unicode, so that the text of a PDF can be extracted and read aloud.
Needs fontTools (pip install fonttools).
"""

from fontTools.fontBuilder import FontBuilder
from fontTools.pens.t2CharStringPen import T2CharStringPen

UNIT = 50          # font units per pixel; 20 pixels = 1000 units, the em
# How far an outline that reaches the edge of its cell goes past it, a tenth
# of a pixel: the blocks and lines of two cells side by side, or one above
# the other, then overlap instead of touching, and no seam shows where a
# viewer's anti-aliasing would leave one
OVERSHOOT = 5
# The characters drawn to join their neighbours: the shades, the lines of
# box drawing and the blocks (0xB0 to 0xDF), and the two halves of the
# integral sign; letters are left as they are
JOINING = set(range(0xB0, 0xE0)) | {0xF4, 0xF5}
FAMILY = "OEM12x20"
VERSION = "1.000"
COPYRIGHT = "Copyright (c) 2026 PDFjet Software"
LICENSE = "MIT License"

# The graphic characters that code page 437 shows for the bytes 0x01 to 0x1F
# and 0x7F; Python's cp437 codec maps these to control characters instead
CP437_LOW = (
    "\u0000☺☻♥♦♣♠•◘○◙♂♀♪♫☼►◄↕‼¶§▬↨↑↓→←∟↔▲▼"
)


def unicode_of(code):
    if code < 0x20:
        return ord(CP437_LOW[code])
    if code == 0x7F:
        return 0x2302  # ⌂
    return ord(bytes([code]).decode("cp437"))


def read_bdf(path):
    """Returns the font's properties and its glyphs, as {code: (bbx, rows)}."""
    props, glyphs = {}, {}
    with open(path, encoding="latin-1") as f:
        lines = iter(f.read().splitlines())
    for line in lines:
        key, _, value = line.partition(" ")
        if key in ("FONT_ASCENT", "FONT_DESCENT"):
            props[key] = int(value)
        elif key == "STARTCHAR":
            code, bbx, rows = None, None, []
            for line in lines:
                key, _, value = line.partition(" ")
                if key == "ENCODING":
                    code = int(value)
                elif key == "BBX":
                    bbx = tuple(int(v) for v in value.split())
                elif key == "BITMAP":
                    for line in lines:
                        if line == "ENDCHAR":
                            break
                        rows.append(int(line, 16))
                    break
            width, height = bbx[0], bbx[1]
            bits = (len(f"{rows[0]:x}") if rows else 0) * 4
            bits = max(bits, (width + 7) // 8 * 8)
            glyphs[code] = (bbx, [[(row >> (bits - 1 - x)) & 1
                                   for x in range(width)] for row in rows])
    return props, glyphs


def contours(bbx, pixels):
    """Traces the outlines of the lit pixels, in font units, clockwise for
    the outer contours, with the collinear points removed."""
    width, height, x0, y0 = bbx
    # The directed edges of every lit pixel, clockwise with y up; an edge
    # shared by two lit pixels appears in both directions and cancels out
    edges = set()
    for row, bits in enumerate(pixels):
        y = y0 + height - 1 - row
        for col, lit in enumerate(bits):
            if not lit:
                continue
            x = x0 + col
            for edge in (((x, y), (x, y + 1)), ((x, y + 1), (x + 1, y + 1)),
                         ((x + 1, y + 1), (x + 1, y)), ((x + 1, y), (x, y))):
                reverse = (edge[1], edge[0])
                if reverse in edges:
                    edges.remove(reverse)
                else:
                    edges.add(edge)
    outgoing = {}
    for a, b in edges:
        outgoing.setdefault(a, []).append(b)
    result = []
    while outgoing:
        start = min(outgoing)
        points, point, came = [start], start, None
        while True:
            targets = outgoing[point]
            if len(targets) == 1:
                target = targets.pop()
            else:
                # Two pixels touching at a corner: turn right, keeping them
                # in separate contours
                dx, dy = point[0] - came[0], point[1] - came[1]
                right = (point[0] + dy, point[1] - dx)
                target = right if right in targets else targets[0]
                targets.remove(target)
            if not targets:
                del outgoing[point]
            came, point = point, target
            if point == start:
                break
            points.append(point)
        # Keep only the corners
        corners = []
        for i, p in enumerate(points):
            a, b = points[i - 1], points[(i + 1) % len(points)]
            if (p[0] - a[0]) * (b[1] - p[1]) != (p[1] - a[1]) * (b[0] - p[0]):
                corners.append((p[0] * UNIT, p[1] * UNIT))
        result.append(corners)
    return result


def overshoot(outlines, left, right, bottom, top):
    """Moves the points on the edges of the cell OVERSHOOT units out."""
    def out(v, low, high):
        return v - OVERSHOOT if v == low else v + OVERSHOOT if v == high else v
    return [[(out(x, left, right), out(y, bottom, top)) for x, y in corners]
            for corners in outlines]


def draw(pen, outlines):
    # CFF outer contours run counterclockwise
    for corners in outlines:
        corners = corners[:1] + corners[:0:-1]
        pen.moveTo(corners[0])
        for p in corners[1:]:
            pen.lineTo(p)
        pen.closePath()


def build():
    props, glyphs = read_bdf("OEM12x20.BDF")
    ascent = props["FONT_ASCENT"] * UNIT
    descent = props["FONT_DESCENT"] * UNIT
    advance = 12 * UNIT

    names, cmap, outlines = [".notdef"], {}, {".notdef": []}
    for code in sorted(glyphs):
        if code == 0:
            continue    # NUL, blank, has no character to map to
        u = unicode_of(code)
        name = "space" if u == 0x20 else f"uni{u:04X}"
        names.append(name)
        cmap[u] = name
        outlines[name] = contours(*glyphs[code])
        if code in JOINING:
            outlines[name] = overshoot(outlines[name],
                                       0, advance, -descent, ascent)

    fb = FontBuilder(1000, isTTF=False)
    fb.setupGlyphOrder(names)
    fb.setupCharacterMap(cmap)
    metrics = {}
    charstrings = {}
    for name in names:
        pen = T2CharStringPen(advance, None)
        draw(pen, outlines[name])
        charstrings[name] = pen.getCharString()
    fb.setupCFF(FAMILY, {"FullName": FAMILY, "Copyright": COPYRIGHT},
                charstrings, {})
    for name in names:
        xs = [p[0] for c in outlines[name] for p in c]
        metrics[name] = (advance, min(xs) if xs else 0)
    fb.setupHorizontalMetrics(metrics)
    fb.setupHorizontalHeader(ascent=ascent, descent=-descent)
    fb.setupNameTable({
        "copyright": COPYRIGHT,
        "familyName": FAMILY,
        "styleName": "Regular",
        "uniqueFontIdentifier": f"PDFjet Software: {FAMILY}: {VERSION}",
        "fullName": FAMILY,
        "version": f"Version {VERSION}",
        "psName": FAMILY,
        "licenseDescription": LICENSE,
    })
    fb.setupOS2(sTypoAscender=ascent, sTypoDescender=-descent,
                sTypoLineGap=0, usWinAscent=ascent, usWinDescent=descent,
                sxHeight=9 * UNIT, sCapHeight=14 * UNIT,
                achVendID="PDFJ", fsType=0)
    # Latin text, monospaced
    fb.font["OS/2"].panose.bFamilyType = 2
    fb.font["OS/2"].panose.bProportion = 9
    fb.setupPost(isFixedPitch=1, underlinePosition=-2 * UNIT,
                 underlineThickness=UNIT)
    fb.save("OEM12x20.otf")


if __name__ == "__main__":
    build()
    print("OEM12x20.otf")
