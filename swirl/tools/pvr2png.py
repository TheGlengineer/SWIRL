#!/usr/bin/env python3
"""Decode simple Dreamcast .PVR (RGB565/ARGB1555/ARGB4444, twiddled square) to PNG."""
import struct, sys
from PIL import Image
def untwiddle(x, y):
    r = 0
    for i in range(12):
        r |= ((y >> i) & 1) << (2 * i)
        r |= ((x >> i) & 1) << (2 * i + 1)
    return r
def conv(v, fmt):
    if fmt == 1:  # 565
        return ((v >> 11 & 31) * 255 // 31, (v >> 5 & 63) * 255 // 63, (v & 31) * 255 // 31, 255)
    if fmt == 0:  # 1555
        return ((v >> 10 & 31) * 255 // 31, (v >> 5 & 31) * 255 // 31, (v & 31) * 255 // 31, 255 if v >> 15 else 0)
    return ((v >> 8 & 15) * 17, (v >> 4 & 15) * 17, (v & 15) * 17, (v >> 12 & 15) * 17)
d = open(sys.argv[1], 'rb').read()
p = d.find(b'PVRT')
size, pf, dt, w, h = struct.unpack('<IBBxxHH', d[p + 4:p + 16])
px = d[p + 16:]
m = min(w, h)
img = Image.new('RGBA', (w, h))
for y in range(h):
    for x in range(w):
        if dt in (1, 2):
            blk = (x // m) + (y // m)
            i = untwiddle(x % m, y % m) + blk * m * m
        else:
            i = y * w + x
        img.putpixel((x, y), conv(struct.unpack_from('<H', px, i * 2)[0], pf))
img.save(sys.argv[2])
print(w, h, pf, dt)
