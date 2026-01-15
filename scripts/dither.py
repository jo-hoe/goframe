#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Generic palette-based dithering filter.

Reads a PNG (or any Pillow-supported) image from stdin, applies palette
quantization with Floyd-Steinberg (or NONE) dithering using the provided
palette, and writes a PNG to stdout.

Args:
  --palette: Palette as a semicolon-separated list of RGB triplets.
             Example: "0,0,0;255,255,255" or "25,30,33;232,232,232;239,222,68"
  --dither:  Dither algorithm, one of: fs, none
             fs   -> Pillow's Image.FLOYDSTEINBERG
             none -> Pillow's Image.NONE

Notes:
  - The gist this is inspired by uses Pillow's quantize with a fixed SPECTRA6
    palette. This script generalizes that approach so any palette can be used.
  - Pillow requires a palette length of exactly 256 colors (768 values). If the
    provided palette has fewer than 256 colors, we pad using the last color.
    If it has more than 256 colors, we truncate to 256.
  - All logging and errors go to stderr; image bytes go to stdout.
"""

import sys
import io
import argparse
from PIL import Image


def parse_palette(palette_str: str):
    # Expect "r,g,b;r,g,b;..."
    if not palette_str:
        raise ValueError("empty --palette")
    colors = []
    entries = [e.strip() for e in palette_str.split(";") if e.strip()]
    for idx, entry in enumerate(entries):
        parts = [p.strip() for p in entry.split(",")]
        if len(parts) != 3:
            raise ValueError(f"palette entry {idx} must have exactly 3 components, got: {entry}")
        try:
            r, g, b = (int(parts[0]), int(parts[1]), int(parts[2]))
        except ValueError:
            raise ValueError(f"palette entry {idx} must be integers 0-255, got: {entry}")
        for c in (r, g, b):
            if c < 0 or c > 255:
                raise ValueError(f"palette entry {idx} contains out-of-range value (0-255): {entry}")
        colors.append((r, g, b))
    if len(colors) == 0:
        raise ValueError("palette must contain at least one color")
    return colors


def build_palette_table(colors):
    # Pillow expects exactly 256 colors (256 * 3 = 768 values)
    # Truncate or pad with the last color as necessary.
    max_colors = 256
    if len(colors) > max_colors:
        colors = colors[:max_colors]
    if len(colors) < max_colors:
        # Match the gist behavior: pad with the FIRST color
        first = colors[0]
        colors = colors + [first] * (max_colors - len(colors))
    flat = []
    for (r, g, b) in colors:
        flat.extend([r, g, b])
    # Ensure correct length
    if len(flat) != 768:
        raise RuntimeError(f"internal palette error, length={len(flat)} != 768")
    return flat


def main():
    parser = argparse.ArgumentParser(description="Apply palette-based dithering via Pillow quantize().")
    parser.add_argument("--palette", required=True, help='Palette like "0,0,0;255,255,255"')
    parser.add_argument("--dither", choices=["fs", "none"], default="fs", help="Dithering algorithm")
    args = parser.parse_args()

    try:
        raw = sys.stdin.buffer.read()
        if not raw:
            sys.stderr.write("No input received on stdin\n")
            sys.exit(1)
    except Exception as e:
        sys.stderr.write(f"Failed to read stdin: {e}\n")
        sys.exit(1)

    try:
        img = Image.open(io.BytesIO(raw)).convert("RGB")
    except Exception as e:
        sys.stderr.write(f"Failed to decode input image: {e}\n")
        sys.exit(2)

    try:
        colors = parse_palette(args.palette)
        pal_table = build_palette_table(colors)
        pal_img = Image.new("P", (1, 1))
        pal_img.putpalette(pal_table)
    except Exception as e:
        sys.stderr.write(f"Invalid palette: {e}\n")
        sys.exit(3)

    dither_mode = Image.FLOYDSTEINBERG if args.dither == "fs" else Image.NONE

    try:
        # Quantize with provided palette and dither, convert back to RGB for output
        q = img.quantize(dither=dither_mode, palette=pal_img).convert("RGB")
        out = io.BytesIO()
        q.save(out, format="PNG")
        sys.stdout.buffer.write(out.getvalue())
        sys.stdout.flush()
    except Exception as e:
        sys.stderr.write(f"Dithering/encoding failed: {e}\n")
        sys.exit(4)


if __name__ == "__main__":
    main()
