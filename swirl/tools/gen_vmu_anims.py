"""SWIRL VMU animations (run from the repository root: python3 swirl/tools/gen_vmu_anims.py).

SWIRL VMU animations: every frame is a real 48x32 1-bit bitmap (1 = dark pixel).
The same frames feed the preview page and, once approved, the C tables in the menu."""
import math, random, re, json
import numpy as np

W, H = 48, 32
FPS = 12  # pushed every 5 video frames (60 Hz)

# ---------- font (Silkscreen, as in ui/swirl/vmu_font.h) ----------
src = open('ui/swirl/vmu_font.h').read()
adv = [int(x) for x in re.search(r'vmu_font_adv\[95\] = \{([^}]*)\}', src).group(1).split(',')]
rows = [[int(v, 16) for v in m.split(',')] for m in re.findall(r'\{(0x[^}]*)\}', src.split('vmu_font_rows')[1])]


def text(img, x, y, s, on=1):
    for ch in s:
        i = ord(ch) - 32
        for r in range(5):
            for c in range(8):
                if rows[i][r] & (0x80 >> c) and 0 <= x + c < W and 0 <= y + r < H:
                    img[y + r, x + c] = on
        x += adv[i]
    return x


def text_w(s):
    return sum(adv[ord(c) - 32] for c in s) - 1


def blank():
    return np.zeros((H, W), np.uint8)


def dot(img, x, y, on=1):
    x, y = int(round(x)), int(round(y))
    if 0 <= x < W and 0 <= y < H:
        img[y, x] = on


def disc(img, cx, cy, r, on=1):
    for y in range(H):
        for x in range(W):
            if (x - cx) ** 2 + (y - cy) ** 2 <= r * r:
                img[y, x] = on


def ring(img, cx, cy, r, t0=0.0, t1=1.0, thick=1.0, on=1):
    """ring from fraction t0 to t1 of a turn, clockwise from 12 o'clock"""
    for y in range(H):
        for x in range(W):
            d = math.hypot(x - cx, y - cy)
            if r - thick / 2 <= d < r + thick / 2:
                a = (math.atan2(x - cx, -(y - cy)) / (2 * math.pi)) % 1.0
                if t0 <= a <= t1:
                    img[y, x] = on


def line(img, x0, y0, x1, y1, on=1):
    n = int(max(abs(x1 - x0), abs(y1 - y0))) + 1
    for i in range(n + 1):
        t = i / max(n, 1)
        dot(img, x0 + (x1 - x0) * t, y0 + (y1 - y0) * t, on)


def sparkle(img, x, y, size, on=1):
    dot(img, x, y, on)
    for k in range(1, size + 1):
        for dx, dy in ((k, 0), (-k, 0), (0, k), (0, -k)):
            dot(img, x + dx, y + dy, on)


# ---------- the SWIRL logo ----------
logo_bytes = [int(x, 16) for x in re.search(r'default_logo\[192\] = \{([^}]*)\}',
              open('ui/swirl/sw_vmu.c').read()).group(1).split(',')]
LOGO = np.array([[1 if logo_bytes[y * 6 + x // 8] & (0x80 >> (x % 8)) else 0 for x in range(W)] for y in range(H)], np.uint8)
SWIRL_PART = LOGO.copy(); SWIRL_PART[24:, :] = 0   # the swirl shape
WORD_PART = LOGO.copy(); WORD_PART[:24, :] = 0     # the word SWIRL under it
CX, CY = 26.5, 12.5                                 # centre of the swirl


def stroke_order(shape, start):
    """distance along the shape from its tail, so it can be drawn like a brush stroke"""
    from collections import deque
    dist = -np.ones(shape.shape, int)
    q = deque([start]); dist[start[1], start[0]] = 0
    while q:
        x, y = q.popleft()
        for dx in (-1, 0, 1):
            for dy in (-1, 0, 1):
                nx, ny = x + dx, y + dy
                if 0 <= nx < W and 0 <= ny < H and shape[ny, nx] and dist[ny, nx] < 0:
                    dist[ny, nx] = dist[y, x] + 1
                    q.append((nx, ny))
    # pixels not connected to the tail (the small hook) come last, by angle
    rest = (shape == 1) & (dist < 0)
    mx = dist.max()
    for y, x in zip(*np.nonzero(rest)):
        dist[y, x] = mx + 1 + int((math.atan2(y - CY, x - CX) + math.pi) * 3)
    return dist


# the tail at the bottom left of the swirl is where the brush starts
tail = min(((x, y) for y in range(24) for x in range(W) if SWIRL_PART[y, x]), key=lambda p: (-(p[1]) + p[0] * 0.2))
ORDER = stroke_order(SWIRL_PART, tail)
ORDER_MAX = ORDER.max()


def logo_intro():
    frames = []
    rnd = random.Random(7)
    parts = [(arm, k) for arm in range(3) for k in range(11)]
    # 1. a three armed galaxy turns and collapses into the centre (10 frames)
    for f in range(10):
        img = blank()
        t = f / 9
        scale = 1.15 * (1 - t) ** 1.4
        for arm, k in parts:
            r0 = 2 + k * 2.4
            a = arm * 2 * math.pi / 3 + k * 0.42 + t * 4.5
            r = r0 * scale
            if r > 0.6:
                dot(img, CX + r * math.cos(a) * 1.35, CY + r * math.sin(a))
        if f >= 8:
            disc(img, CX, CY, 1.0 + (f - 8))
        frames.append(img)
    # 2. a burst at the centre, then the swirl is drawn from its tail like a brush stroke (12 frames)
    for f in range(12):
        img = blank()
        if f < 3:
            sparkle(img, CX, CY, 1 + f * 2)
        lim = ORDER_MAX * (f + 1) / 12
        img |= ((ORDER >= 0) & (ORDER <= lim) & (SWIRL_PART == 1)).astype(np.uint8)
        # the brush tip: a bright gap just ahead of the stroke
        frames.append(img)
    # 3. the word types in behind a scanning bar (8 frames)
    xs = [x for x in range(W) if WORD_PART[:, x].any()]
    x0, x1 = min(xs), max(xs) + 1
    for f in range(8):
        img = SWIRL_PART.copy()
        edge = x0 + (x1 - x0 + 2) * (f + 1) / 8
        word = WORD_PART.copy(); word[:, int(edge):] = 0
        img |= word
        if f < 7:
            for y in range(25, 32):
                dot(img, edge, y)
        frames.append(img)
    # 4. a flash: the screen inverts once, like a camera flash
    inv = 1 - LOGO
    frames += [inv, LOGO.copy(), inv, LOGO.copy(), LOGO.copy()]
    return frames


def logo_shatter():
    """power on: the still logo (already on the VMU) breaks apart and its pixels spin outwards into the galaxy"""
    frames = []
    rnd = random.Random(11)
    ys, xs = np.nonzero(LOGO)
    pts = [(x, y, rnd.uniform(0.7, 1.4)) for y, x in zip(ys, xs)]
    for f in range(8):
        img = blank()
        t = (f + 1) / 8
        for x, y, sp in pts:
            dx, dy = x - CX, y - CY
            r = math.hypot(dx, dy) + t * t * 30 * sp
            a = math.atan2(dy, dx) + t * 2.2 * sp
            if rnd.random() < 1 - t * 0.7:
                dot(img, CX + r * math.cos(a), CY + r * math.sin(a))
        frames.append(img)
    return frames


def logo_idle():
    """a glint runs along the swirl, then two sparkles twinkle (4 s loop)"""
    frames = []
    n = 48
    for f in range(n):
        img = LOGO.copy()
        if f < 20:  # the glint: a short gap travelling along the stroke
            pos = ORDER_MAX * f / 19
            glint = (SWIRL_PART == 1) & (np.abs(ORDER - pos) <= 1.5)
            img[glint] = 0
            # light escaping at the tip
            if 3 < f < 18:
                ys, xs = np.nonzero(glint)
                if len(xs):
                    dot(img, xs.mean() + 2 * math.cos(f), ys.mean() - 3, 1)
        for (sx, sy, start) in ((41, 4, 24), (9, 18, 30), (42, 19, 37)):
            k = f - start
            if 0 <= k < 5:
                sparkle(img, sx, sy, [0, 1, 2, 1, 0][k])
        frames.append(img)
    return frames


def custom_logo_intro(bits):
    """for an owner's own LOGO.VMU: pixels appear in a spiral sweep from the centre"""
    frames = []
    ys, xs = np.nonzero(bits)
    cx, cy = (xs.mean(), ys.mean()) if len(xs) else (24, 16)
    key = np.full(bits.shape, 99.0)
    for y, x in zip(ys, xs):
        a = (math.atan2(y - cy, x - cx) + math.pi) / (2 * math.pi)
        key[y, x] = math.hypot(x - cx, y - cy) / 30 + a * 0.35
    for f in range(14):
        img = ((key <= (f + 1) / 14 * 1.35) & (bits == 1)).astype(np.uint8)
        frames.append(img)
    frames += [1 - bits, bits.copy(), bits.copy()]
    return frames


def custom_logo_idle(bits):
    """a diagonal shine sweeps across any logo (4 s loop)"""
    frames = []
    for f in range(48):
        img = bits.copy()
        if f < 14:
            p = -10 + f * 5
            for y in range(H):
                for x in range(W):
                    if p <= x + y * 0.6 < p + 3 and bits[y, x]:
                        img[y, x] = 0
        frames.append(img)
    return frames


# ---------- save countdown: 3, 2, 1 ----------
BIG = {  # 12 x 17 digits, 3 px strokes, so they read from across the room
    '3': ["############", "############", "############", "         ###", "         ###", "         ###", "  ##########", "  ##########", "  ##########", "         ###", "         ###", "         ###", "         ###", "############", "############", "############", ""],
    '2': ["############", "############", "############", "         ###", "         ###", "         ###", "############", "############", "############", "###         ", "###         ", "###         ", "###         ", "############", "############", "############", ""],
    '1': ["   #####    ", "  ######    ", " #######    ", "    ####    ", "    ####    ", "    ####    ", "    ####    ", "    ####    ", "    ####    ", "    ####    ", "    ####    ", "    ####    ", "    ####    ", " ########## ", " ########## ", " ########## ", ""],
}


def big_digit(img, d, x, y):
    for r, row in enumerate(BIG[d]):
        for c, ch in enumerate(row):
            if ch == '#':
                dot(img, x + c, y + r)


def countdown():
    frames = []
    cx, cy, r = 23.5, 15.5, 14.0
    for n in '321':
        for f in range(FPS):
            img = blank()
            t = f / FPS
            # the ring drains clockwise over the second, with a bright head
            ring(img, cx, cy, r, t, 1.0, thick=2.2)
            a = 2 * math.pi * t
            hx, hy = cx + r * math.sin(a), cy - r * math.cos(a)
            sparkle(img, hx, hy, 1)
            # the digit drops in with a small bounce
            drop = [-12, -5, 2, -1, 0][f] if f < 5 else 0
            big_digit(img, n, 18, 7 + drop)
            # ticks around the ring, one per second left
            frames.append(img)
    return frames


# ---------- saving ----------
def card(img, x, y, fill=0.0, slot_open=True):
    """a little VMU: body, screen and buttons"""
    for yy in range(y, y + 20):
        for xx in range(x, x + 14):
            edge = yy in (y, y + 19) or xx in (x, x + 13)
            corner = (yy in (y, y + 19)) and (xx in (x, x + 13))
            if edge and not corner:
                img[yy, xx] = 1
    # screen
    for yy in range(y + 2, y + 9):
        for xx in range(x + 2, x + 12):
            if yy in (y + 2, y + 8) or xx in (x + 2, x + 11):
                img[yy, xx] = 1
    # fill bars inside the screen
    bars = int(round(fill * 8))
    for b in range(bars):
        for yy in range(y + 4, y + 7):
            img[yy, x + 3 + b] = 1
    # buttons
    dot(img, x + 3, y + 13); dot(img, x + 4, y + 12); dot(img, x + 5, y + 13); dot(img, x + 4, y + 14)
    dot(img, x + 9, y + 14); dot(img, x + 11, y + 12)


def mini_swirl(img, cx, cy, turn):
    """three curved arms turning around a centre, like the SWIRL mark"""
    disc(img, cx, cy, 1.2)
    for arm in range(3):
        for k in range(10):
            r = 1.8 + k * 0.52
            a = 2 * math.pi * (turn + arm / 3) + k * 0.3
            dot(img, cx + r * math.cos(a), cy + r * math.sin(a))
            if k > 3:
                dot(img, cx + (r - 0.7) * math.cos(a - 0.15), cy + (r - 0.7) * math.sin(a - 0.15))


def saving_loop():
    frames = []
    n = 24  # 2 s
    rnd = random.Random(3)
    bits = [(rnd.uniform(0, 1), rnd.choice((0, 1))) for _ in range(9)]
    for f in range(n):
        img = blank()
        t = f / n
        card(img, 31, 7, fill=(f % 12 + 1) / 12)
        # data streams from the Dreamcast swirl on the left, arcing into the card
        mini_swirl(img, 8, 12, t * 2)
        for ph, b in bits:
            p = (t * 2 + ph) % 1.0
            x = 13 + p * 18
            y = 12 - math.sin(p * math.pi) * 7
            if b:
                dot(img, x, y); dot(img, x + 1, y); dot(img, x, y + 1); dot(img, x + 1, y + 1)
            else:
                dot(img, x, y)
        dots = '.' * (f // 6 % 4)
        text(img, 4, 26, 'SAVING' + dots)
        frames.append(img)
    return frames


def saving_still():
    img = blank()
    card(img, 31, 7, fill=0.5)
    mini_swirl(img, 8, 12, 0)
    # a solid arrow into the card
    line(img, 14, 12, 27, 12); line(img, 14, 13, 27, 13)
    for k in range(4):
        line(img, 27 - k, 12 - k, 27 - k, 13 + k)
    text(img, 4, 26, 'SAVING...')
    return [img]


# ---------- saved ----------
CHECK = [(15, 14), (21, 20), (33, 7)]


def saved():
    frames = []
    cx, cy = 24, 13
    # circle pops open with an overshoot
    for r in (3, 8, 13, 15, 13):
        img = blank(); ring(img, cx, cy, r, thick=2.0); frames.append(img)
    # the tick draws itself
    pts = []
    (ax, ay), (bx, by), (qx, qy) = CHECK
    for i in range(7):
        pts.append((ax + (bx - ax) * i / 6, ay + (by - ay) * i / 6))
    for i in range(1, 13):
        pts.append((bx + (qx - bx) * i / 12, by + (qy - by) * i / 12))
    for k in range(4, len(pts) + 1, 3):
        img = blank(); ring(img, cx, cy, 13, thick=2.0)
        for i in range(1, k):
            (x0, y0), (x1, y1) = pts[i - 1], pts[i]
            line(img, x0, y0, x1, y1); line(img, x0, y0 + 1, x1, y1 + 1)
        frames.append(img)
    done = frames[-1].copy()
    # sparkles burst out and the word appears
    for k in range(8):
        img = done.copy()
        for ang in (0.8, 2.3, 3.9, 5.5):
            d = 15 + k * 1.6
            s = [1, 2, 2, 1, 1, 0, 0, 0][k]
            if s:
                sparkle(img, cx + d * math.cos(ang) * 1.2, cy + d * math.sin(ang), s)
        w = 'SAVED'
        tw = text_w(w)
        if k >= 1:
            text(img, 24 - tw // 2, 27, w[:min(len(w), k)])
        frames.append(img)
    hold = frames[-1].copy()
    frames += [hold] * 10
    return frames


# ---------- not saved ----------
def not_saved(reason='NO SPACE'):
    frames = []
    cx, cy = 24, 12
    for k in range(18):
        img = blank()
        shake = [0, -3, 3, -2, 2, -1, 1, 0][k] if k < 8 else 0
        ring(img, cx + shake, cy, 11, thick=2.0)
        for o in (0, 1):
            line(img, cx + shake - 5 + o, cy - 5, cx + shake + 5 + o, cy + 5)
            line(img, cx + shake + 5 + o, cy - 5, cx + shake - 5 + o, cy + 5)
        tw = text_w(reason)
        if k >= 4:
            text(img, 24 - tw // 2, 26, reason)
        frames.append(img)
    return frames


def pack(frames):
    """rows of 6 bytes, MSB = left, 1 = dark (the VMU.DAT / LOGO.VMU layout)"""
    out = []
    for img in frames:
        b = []
        for y in range(H):
            for bx in range(6):
                v = 0
                for i in range(8):
                    if img[y, bx * 8 + i]:
                        v |= 0x80 >> i
                b.append(v)
        out.append(b)
    return out


if __name__ == '__main__':
    # an example owner logo to show the generic effects
    demo = blank(); text(demo, 5, 9, 'MY'); text(demo, 5, 17, 'LOGO'); ring(demo, 36, 15, 8, thick=2); disc(demo, 36, 15, 3)
    anims = {
        'logo_boot': logo_shatter() + logo_intro(),
        'logo_intro': logo_intro(),
        'logo_idle': logo_idle(),
        'custom_intro': custom_logo_intro(demo),
        'custom_idle': custom_logo_idle(demo),
        'countdown': countdown(),
        'saving_still': saving_still(),
        'saving': saving_loop(),
        'saved': saved(),
        'not_saved': not_saved(),
        'not_saved_check': not_saved('CHECK VMU'),
        'not_saved_busy': not_saved('VMU BUSY'),
    }
    data = {k: [''.join(''.join(str(int(v)) for v in row) for row in f) for f in fr] for k, fr in anims.items()}
    import os
    os.makedirs('swirl/build', exist_ok=True)
    json.dump({'fps': FPS, 'anims': data}, open('swirl/build/vmu_anims.json', 'w'))  # for the preview page
    for k, fr in anims.items():
        print(k, len(fr), 'frames', f'{len(fr) / FPS:.1f}s', len(fr) * 192, 'bytes')
    # the menu's table (custom_* are made on the Dreamcast from the owner's own LOGO.VMU)
    keep = ['logo_boot', 'logo_intro', 'logo_idle', 'countdown', 'saving_still', 'saving', 'saved',
            'not_saved', 'not_saved_check', 'not_saved_busy']
    out = ['/* Generated by swirl/tools/gen_vmu_anims.py. Do not edit: change the script and run it again.',
           '   48x32 VMU pictures, 6 bytes a row, MSB = left pixel, 1 = dark; played at 12 frames a second. */',
           '#pragma once', '#include <stdint.h>', '']
    for k in keep:
        data = pack(anims[k])
        out.append(f'static const uint8_t vmu_anim_{k}[{len(data)}][192] = {{')
        for fr in data:
            out.append('  {' + ','.join(f'0x{v:02x}' for v in fr) + '},')
        out.append('};')
    out.append('')
    out.append('#define VMU_ANIM_FPS 12')
    open('ui/swirl/sw_vmu_anim.h', 'w').write('\n'.join(out) + '\n')
