#!/usr/bin/env python3
"""SWIRL: draw the SWIRL logo and write every file that uses it.

The mark is three tapered strokes turning around a hub, in the orange of the menu's accent colour: a nod
to the swirl era of the Dreamcast, drawn from scratch for SWIRL.

Writes:
  docs/images/logo/swirl-mark.svg        the mark on its own (transparent)
  docs/images/logo/swirl-icon.svg        the mark on the dark app tile
  docs/images/logo/swirl-mark-512.png    PNG versions of both
  docs/images/logo/swirl-icon-512.png
  docs/images/logo/swirl-logo.png        mark and SWIRL wordmark for dark backgrounds (README)
  docs/images/logo/swirl-logo-light.png  the same for light backgrounds
  swirl/cardmanager/web/icon.svg         app tile, used by Card Manager's page
  swirl/cardmanager/web/favicon.png      32 x 32
  swirl/cardmanager/web/icon-256.png     256 x 256
  swirl/cardmanager/assets/app.ico       Windows icon, 16 to 256 pixels (run winres/make.sh afterwards)
  swirl/cardmanager/assets/default_logo.bin and the default_logo array in ui/swirl/sw_vmu.c: the 48 x 32
                                         VMU logo (rebuild the menu afterwards)
Card Manager's header draws the same paths (the i-swirl symbol in web/index.html); paste them from
mark_paths() if the shape changes.

Needs: pip install cairosvg pillow
"""
import io
import math
import os
import re

import cairosvg
from PIL import Image, ImageDraw, ImageFont

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
CM = os.path.join(ROOT, 'swirl', 'cardmanager')
LOGO = os.path.join(ROOT, 'docs', 'images', 'logo')
FONT = os.path.join(ROOT, 'swirl', 'assets', 'fonts', 'Sora.ttf')

ORANGE_LIGHT, ORANGE_DARK = '#ffb24a', '#ee6a12'
TILE_LIGHT, TILE_DARK = '#1b2540', '#070a12'


def arm(theta0, sweep=3.2, r0=6.0, r1=44.0, width=17.0, n=90):
    """One stroke: follows a spiral outwards, thin at the hub, widest past the middle, sharp at the tip."""
    def at(t):
        r = r0 + (r1 - r0) * t
        a = theta0 + sweep * t
        return r * math.cos(a), r * math.sin(a)

    left, right = [], []
    for i in range(n + 1):
        t = i / n
        x, y = at(t)
        xa, ya = at(max(0.0, t - 1e-3))
        xb, yb = at(min(1.0, t + 1e-3))
        tx, ty = xb - xa, yb - ya
        length = math.hypot(tx, ty) or 1.0
        nx, ny = -ty / length, tx / length
        w = width * (math.sin(math.pi * t) ** 0.8) * (0.35 + 0.65 * t)
        left.append((x + nx * w / 2, y + ny * w / 2))
        right.append((x - nx * w / 2, y - ny * w / 2))
    pts = left + right[::-1]
    return 'M' + ' L'.join(f'{x:.2f},{y:.2f}' for x, y in pts) + ' Z'


def mark_paths():
    return [arm(math.radians(-90) + k * 2 * math.pi / 3) for k in range(3)]


def svg(tile, size=None):
    grad = (f'<linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{ORANGE_LIGHT}"/>'
            f'<stop offset="1" stop-color="{ORANGE_DARK}"/></linearGradient>')
    bg = ''
    if tile:
        grad += (f'<linearGradient id="bg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{TILE_LIGHT}"/>'
                 f'<stop offset="1" stop-color="{TILE_DARK}"/></linearGradient>')
        bg = ('<rect x="-58" y="-58" width="116" height="116" rx="26" fill="url(#bg)"/>'
              '<rect x="-57.25" y="-57.25" width="114.5" height="114.5" rx="25.3" fill="none" '
              'stroke="#f28c28" stroke-opacity=".35" stroke-width="1.5"/>')
    dims = f' width="{size}" height="{size}"' if size else ''
    body = ''.join(f'<path d="{d}"/>' for d in mark_paths())
    view = '-60 -60 120 120' if tile else '-52 -52 104 104'
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{view}"{dims}><defs>{grad}</defs>{bg}'
            f'<g fill="url(#g)">{body}<circle r="6.5"/></g></svg>\n')


def png(svg_text, size):
    return Image.open(io.BytesIO(cairosvg.svg2png(bytestring=svg_text.encode(), output_width=size,
                                                  output_height=size))).convert('RGBA')


# "SWIRL" in 5 pixel high letters for the bottom of the VMU logo
VMU_TEXT = [
    '..............###.#...#.#.##..#.................',
    '..............#...#...#.#.#.#.#.................',
    '..............###.#.#.#.#.##..#.................',
    '................#.#.#.#.#.#.#.#.................',
    '..............###..#.#..#.#.#.###...............',
]


def vmu_logo():
    """48 x 32 one bit picture: the mark, centred in the top 24 rows, and SWIRL underneath."""
    size, ink = 25, 180
    art = svg(False).replace('url(#g)', '#000')
    im = Image.open(io.BytesIO(cairosvg.svg2png(bytestring=art.encode(), output_width=size * 4,
                                                output_height=size * 4, background_color='white')))
    im = im.convert('L').resize((size, size), Image.BOX)
    pts = [(x, y) for y in range(size) for x in range(size) if im.getpixel((x, y)) < ink]
    x0, x1 = min(p[0] for p in pts), max(p[0] for p in pts)
    y0, y1 = min(p[1] for p in pts), max(p[1] for p in pts)
    ox, oy = (48 - (x1 - x0 + 1)) // 2 - x0, (24 - (y1 - y0 + 1)) // 2 - y0
    px = [[0] * 48 for _ in range(32)]
    for x, y in pts:
        px[y + oy][x + ox] = 1
    for i, row in enumerate(VMU_TEXT):
        px[26 + i] = [1 if c == '#' else 0 for c in row]
    out = bytearray()
    for y in range(32):
        for xb in range(6):
            v = 0
            for i in range(8):
                v |= px[y][xb * 8 + i] << (7 - i)
            out.append(v)
    return bytes(out)


def main():
    os.makedirs(LOGO, exist_ok=True)
    mark, tile = svg(False), svg(True)
    open(os.path.join(LOGO, 'swirl-mark.svg'), 'w').write(mark)
    open(os.path.join(LOGO, 'swirl-icon.svg'), 'w').write(tile)
    open(os.path.join(CM, 'web', 'icon.svg'), 'w').write(tile)
    png(mark, 512).save(os.path.join(LOGO, 'swirl-mark-512.png'), optimize=True)
    png(tile, 512).save(os.path.join(LOGO, 'swirl-icon-512.png'), optimize=True)
    png(tile, 32).save(os.path.join(CM, 'web', 'favicon.png'), optimize=True)
    png(tile, 256).save(os.path.join(CM, 'web', 'icon-256.png'), optimize=True)
    sizes = [16, 24, 32, 48, 64, 128, 256]
    big = png(tile, 256)
    big.save(os.path.join(CM, 'assets', 'app.ico'), sizes=[(s, s) for s in sizes],
             append_images=[png(tile, s) for s in sizes[:-1]])

    logo = vmu_logo()
    open(os.path.join(CM, 'assets', 'default_logo.bin'), 'wb').write(logo)
    vmu_c = os.path.join(ROOT, 'ui', 'swirl', 'sw_vmu.c')
    src = open(vmu_c).read()
    arr = ', '.join(f'0x{b:02x}' for b in logo)
    src = re.sub(r'(static const uint8_t default_logo\[192\] = \{).*?(\};)', lambda m: m.group(1) + arr + m.group(2),
                 src, flags=re.S)
    open(vmu_c, 'w').write(src)

    # README lockups: mark and wordmark on a transparent canvas, white text for dark pages and near black
    # text for light pages
    h = 220
    m = png(mark, h)
    font = ImageFont.truetype(FONT, 150)
    font.set_variation_by_name('Bold')
    text = 'SWIRL'
    tw = int(ImageDraw.Draw(Image.new('RGBA', (1, 1))).textlength(text, font=font))
    top = font.getbbox(text)
    for name, colour in (('swirl-logo.png', (255, 255, 255, 255)), ('swirl-logo-light.png', (13, 17, 28, 255))):
        out = Image.new('RGBA', (h + 40 + tw + 10, h), (0, 0, 0, 0))
        out.paste(m, (0, 0), m)
        ImageDraw.Draw(out).text((h + 40, (h - (top[3] - top[1])) // 2 - top[1]), text, font=font, fill=colour)
        out.save(os.path.join(LOGO, name), optimize=True)

    print('Wrote the logo files. Next: winres/make.sh <version> and swirl/build.sh')


if __name__ == '__main__':
    main()
