#!/usr/bin/env python3
"""Writes sealane-stacked.svg for the CNCF Landscape: the favicon mark (no background) above a
"Sealane" wordmark with the text converted to paths. Run: python3 hack/landscape/build-logo.py"""
import os

from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont

FONT = "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "sealane-stacked.svg")
WORD, W, WORD_W, WORD_TOP = "Sealane", 320, 280, 150

font = TTFont(FONT)
glyphs, cmap, hmtx = font.getGlyphSet(), font.getBestCmap(), font["hmtx"]

# Lay the glyphs out in font units (y up), then place them with one transform.
paths, x, top, bottom = [], 0, 0, 0
for ch in WORD:
    name = cmap[ord(ch)]
    pen, box = SVGPathPen(glyphs), BoundsPen(glyphs)
    glyphs[name].draw(pen)
    glyphs[name].draw(box)
    paths.append(f'<path transform="translate({x})" d="{pen.getCommands()}"/>')
    top, bottom = max(top, box.bounds[3]), min(bottom, box.bounds[1])
    x += hmtx[name][0]
s = WORD_W / x
baseline = round(WORD_TOP + top * s)
height = round(baseline - bottom * s) + 10

# Mark: the favicon's 64x64 drawing, nodes span 13..51, scaled 3x and centred above the wordmark.
mark = (
    f'<g transform="translate({W / 2 - 32 * 3:g},{10 - 13 * 3:g}) scale(3)">'
    '<path d="M18 19v26h28V19M18 19l28 26M18 19h28" fill="none" stroke="#2563eb" stroke-width="4" '
    'stroke-linecap="round" stroke-linejoin="round"/>'
    '<circle cx="18" cy="19" r="5" fill="#2563eb"/><circle cx="46" cy="19" r="5" fill="#93c5fd"/>'
    '<circle cx="18" cy="45" r="5" fill="#93c5fd"/><circle cx="46" cy="45" r="5" fill="#2563eb"/></g>'
)
word = (
    f'<g fill="#111827" transform="translate({(W - WORD_W) / 2:g},{baseline}) scale({s:.6f},{-s:.6f})">'
    + "".join(paths) + "</g>"
)
svg = (
    f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {height}" width="{W}" height="{height}">'
    f"<title>Sealane</title>{mark}{word}</svg>\n"
)
with open(OUT, "w") as f:
    f.write(svg)
print(f"wrote {OUT} ({len(svg)} bytes, viewBox 0 0 {W} {height})")
