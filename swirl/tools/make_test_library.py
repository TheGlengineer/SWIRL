#!/usr/bin/env python3
"""SWIRL: build a sample library for emulator testing.

Writes swirl/test/OPENMENU.INI plus BOX.DAT, ICON.DAT and META.DAT in openMenu's DAT format.
Cover art is generated placeholder art (abstract shapes + title), not real box art.
"""
import os, struct
from PIL import Image, ImageDraw, ImageFont

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
OUT = os.path.join(ROOT, 'swirl', 'test')
FONT = os.path.join(ROOT, 'swirl', 'assets', 'fonts', 'Sora.ttf')

G = dict(action=1, racing=2, sim=4, sports=8, lightgun=16, fighting=32, shooter=64, survival=128,
         adventure=256, platformer=512, rpg=1024, shmup=2048, strategy=4096, puzzle=8192, arcade=16384, music=32768)
A = dict(jump=1, keyboard=2, vga=4, mouse=8, maracas=16, wheel=32, mic=64, stick=128, gun=256, bba=512, modem=8192)

# name, product, year, region, discs, players, vmu, network, genres, accessories, colours, description
GAMES = [
    ('SONIC ADVENTURE', 'SWT0001', 1999, 'U', 1, 1, 4, 0, 'platformer', 'jump', ('#12306e', '#3a8ff0', '#f2c230'),
     'Six playable characters, one sprawling adventure. Blast through high speed action stages and explore the hub worlds between them.'),
    ('CRAZY TAXI', 'SWT0002', 2000, 'U', 1, 1, 5, 0, 'racing arcade', 'jump', ('#5a2a08', '#f5b21b', '#2fa3d8'),
     'Pick up fares and race them across a sunny coastal city before the clock runs out. Shortcuts, stunts and tips keep the meter alive.'),
    ('JET GRIND RADIO', 'SWT0003', 2000, 'U', 1, 1, 8, 0, 'action', 'jump', ('#3b0f4f', '#e8e23a', '#ea3c8f'),
     'Skate, grind and tag your way across Tokyo-to while rival gangs and the police close in on your crew.'),
    ('SOULCALIBUR', 'SWT0004', 1999, 'U', 1, 2, 6, 0, 'fighting', 'jump stick', ('#3d1010', '#d9a441', '#8c1c1c'),
     'Weapon based 3D fighting with eight way movement, a huge cast and a deep mission mode.'),
    ('SHENMUE', 'SWT0005', 2000, 'U', 4, 1, 12, 0, 'adventure', 'jump', ('#10281f', '#6fb58a', '#c9b27a'),
     'Search the streets of Yokosuka for the man who killed your father, in a living town with its own schedule and weather.'),
    ('CHUCHU ROCKET!', 'SWT0006', 2000, 'U', 1, 4, 5, 1, 'puzzle', 'modem', ('#0e3a4a', '#f2f2f2', '#f25c2a'),
     'Four player puzzle chaos: place arrows to guide mice into your rocket and cats into everyone else\'s.'),
    ('POWER STONE 2', 'SWT0007', 2000, 'U', 1, 4, 10, 0, 'fighting action', 'jump', ('#4a2a08', '#f07b22', '#2d2d2d'),
     'Four player arena brawling with shifting stages, grab anything weapons and power stone transformations.'),
    ('MARVEL VS. CAPCOM 2', 'SWT0008', 2000, 'U', 1, 2, 9, 0, 'fighting', 'stick', ('#16244f', '#e04b6a', '#f2e4c4'),
     'Three on three tag team fighting with an enormous roster and non stop assists.'),
    ('THE HOUSE OF THE DEAD 2', 'SWT0009', 1999, 'U', 1, 2, 4, 0, 'lightgun shooter', 'gun jump', ('#2b0b0b', '#c23a2a', '#e7e7e7'),
     'Grab a light gun and blast through a city overrun by the undead, with branching paths and a partner at your side.'),
    ('PHANTASY STAR ONLINE', 'SWT0010', 2001, 'U', 1, 4, 20, 3, 'rpg action', 'keyboard modem bba', ('#1b3a5e', '#7fd0f0', '#f2c230'),
     'Team up with players around the world to explore the planet Ragol in real time online action role playing.'),
    ('SKIES OF ARCADIA', 'SWT0011', 2000, 'U', 2, 1, 15, 0, 'rpg adventure', 'jump', ('#123d5a', '#f2d48a', '#c24a2a'),
     'Sail floating islands as a sky pirate in a sweeping adventure full of discovery, airship battles and crew recruiting.'),
    ('METROPOLIS STREET RACER', 'SWT0012', 2000, 'U', 1, 2, 18, 0, 'racing', 'wheel jump', ('#1e1e28', '#c46cf0', '#4ad0a2'),
     'Race through carefully recreated London, Tokyo and San Francisco, earning Kudos for driving with style.'),
    ('SAMBA DE AMIGO', 'SWT0013', 2000, 'U', 1, 2, 3, 0, 'music', 'maracas', ('#2a4a12', '#f2e03a', '#e04b2a'),
     'Shake the maracas to the beat of Latin favorites in a colorful rhythm party.'),
    ('SPACE CHANNEL 5', 'SWT0014', 2000, 'U', 1, 1, 3, 0, 'music action', '', ('#3a1250', '#f25cc8', '#f2f2f2'),
     'Report live from a groovy space age as Ulala, dancing invaders into submission with call and response rhythm.'),
    ('IKARUGA', 'SWT0015', 2002, 'J', 1, 2, 2, 0, 'shmup', 'stick', ('#101418', '#e7e7e7', '#2a6ad0'),
     'Switch your ship between light and dark polarity to absorb bullets in a legendary vertical shooter.'),
    ('REZ', 'SWT0016', 2002, 'E', 1, 1, 2, 0, 'shooter music', '', ('#050a1a', '#2ae0f0', '#f02a8a'),
     'Lock on and fire to the beat as a hacker diving into a synesthetic digital world.'),
    ('NFL 2K1', 'SWT0017', 2000, 'U', 1, 4, 30, 1, 'sports', 'modem', ('#123018', '#e7e7e7', '#c2862a'),
     'Pro football with smart play calling, franchise mode and online head to head games.'),
    ('VIRTUA TENNIS', 'SWT0018', 2000, 'U', 1, 4, 6, 0, 'sports arcade', 'jump', ('#1a3a6a', '#d0f02a', '#f2f2f2'),
     'Fast arcade tennis with a world circuit of training games and four player doubles.'),
    ('RESIDENT EVIL CODE: VERONICA', 'SWT0019', 2000, 'U', 2, 1, 7, 0, 'survival adventure', 'jump', ('#1a0a0a', '#8a1a1a', '#c9c9c9'),
     'Claire Redfield searches for her brother on a remote island prison in a tense survival horror.'),
    ('GRANDIA II', 'SWT0020', 2000, 'U', 1, 1, 10, 0, 'rpg', 'jump', ('#2a2a0a', '#e0b42a', '#2a8ad0'),
     'Ryudo, a cynical mercenary, is hired to escort a songstress in a story driven role playing game with dynamic battles.'),
    ('QUAKE III ARENA', 'SWT0021', 2000, 'U', 1, 4, 5, 3, 'shooter', 'keyboard mouse modem bba', ('#1a1206', '#c26a1a', '#8a8a8a'),
     'Fast arena combat with bots or friends online, playable with keyboard and mouse.'),
    ('STREET FIGHTER III 3RD STRIKE', 'SWT0022', 2000, 'U', 1, 2, 4, 0, 'fighting arcade', 'stick', ('#2a1a3a', '#f2a02a', '#2ac0f0'),
     'The arcade classic with parries, super arts and a razor sharp cast.'),
    ('SEAMAN', 'SWT0023', 2000, 'U', 1, 1, 12, 0, 'sim', 'mic', ('#0a2a2a', '#2ad0a0', '#f2f2f2'),
     'Raise a strange talking creature, speaking to it through the microphone as it grows and remembers you.'),
    ('BEATS OF RAGE', 'SWT0024', 2003, 'JUE', 1, 2, 0, 0, None, '', ('#2a2a2a', '#f24a2a', '#f2f2f2'), None),
    ('HOMEBREW DEMO DISC', 'SWT0025', 2024, 'JUE', 1, 1, 0, 0, None, '', ('#1a2a4a', '#8af0f0', '#f2f2f2'), None),
]

def twiddle_index(x, y):
    r = 0
    for i in range(10):
        r |= ((y >> i) & 1) << (2 * i)
        r |= ((x >> i) & 1) << (2 * i + 1)
    return r

def hexrgb(h):
    h = h.lstrip('#')
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))

def art(name, cols, size):
    c1, c2, c3 = (hexrgb(c) for c in cols)
    im = Image.new('RGB', (size, size), c1)
    d = ImageDraw.Draw(im)
    s = size / 256
    d.ellipse([60 * s, 24 * s, 196 * s, 160 * s], fill=c2)
    d.polygon([(-10 * s, 170 * s), (266 * s, 120 * s), (266 * s, 150 * s), (-10 * s, 200 * s)], fill=c3)
    d.rectangle([0, 196 * s, size, size], fill=tuple(int(v * 0.35) for v in c1))
    font = ImageFont.truetype(FONT, int(18 * s) if size >= 256 else 13)
    font.set_variation_by_name(b'Bold')
    words, lines, cur = name.split(), [], ''
    for w in words:
        t = (cur + ' ' + w).strip()
        if font.getlength(t) > size - 20 * s and cur:
            lines.append(cur)
            cur = w
        else:
            cur = t
    lines.append(cur)
    y = 204 * s
    for ln in lines[:2]:
        d.text((10 * s, y), ln, font=font, fill=(245, 245, 245))
        y += 22 * s
    d.text((size - 10 * s, 8 * s), 'TEST ART', font=ImageFont.truetype(FONT, max(8, int(9 * s))), fill=(255, 255, 255), anchor='ra')
    return im

def pvr565(im):
    w, h = im.size
    data = bytearray(w * h * 2)
    px = im.load()
    for y in range(h):
        for x in range(w):
            r, g, b = px[x, y]
            v = ((r >> 3) << 11) | ((g >> 2) << 5) | (b >> 3)
            struct.pack_into('<H', data, twiddle_index(x, y) * 2, v)
    hdr = b'GBIX' + struct.pack('<I', 8) + b'\0' * 8 + b'PVRT' + struct.pack('<IBBxxHH', len(data) + 8, 1, 1, w, h)
    return hdr + bytes(data)

def write_dat(path, entries, chunk):
    n = len(entries)
    first = -(-(16 + 16 * n) // chunk)
    out = bytearray(b'DAT\x01' + struct.pack('<III', chunk, n, 0))
    for i, (pid, _) in enumerate(entries):
        out += pid.encode()[:11].ljust(12, b'\0') + struct.pack('<I', first + i)
    out += b'\0' * (first * chunk - len(out))
    for _, blob in entries:
        out += blob.ljust(chunk, b'\0')
    open(path, 'wb').write(out)

def main():
    os.makedirs(OUT, exist_ok=True)
    box, icon, meta = [], [], []
    ini = ['[OPENMENU]', 'num_items=0', '', '[ITEMS]',
           '01.name=openMenu', '01.disc=1/1', '01.vga=1', '01.region=JUE', '01.version=V0.1.0', '01.date=20210609', '01.product=NEODC_1', '']
    slot = 2
    for (name, pid, year, region, discs, players, vmu, net, genres, acc, cols, desc) in GAMES:
        for d in range(1, discs + 1):
            ini += [f'{slot:02d}.name={name}', f'{slot:02d}.disc={d}/{discs}', f'{slot:02d}.vga=1', f'{slot:02d}.region={region}',
                    f'{slot:02d}.version=V1.000', f'{slot:02d}.date={year}0901', f'{slot:02d}.product={pid}', '']
            slot += 1
        if pid in ('SWT0025',):
            continue  # no art at all: tests the placeholder path
        box.append((pid, pvr565(art(name, cols, 256))))
        icon.append((pid, pvr565(art(name, cols, 128))))
        if genres is None:
            continue  # no metadata: lands in "Homebrew & Other"
        gbits = sum(G[g] for g in genres.split())
        abits = sum(A[a] for a in acc.split()) | A['vga']
        rec = struct.pack('<BBBBHH', players, vmu, 0, net, gbits, abits) + desc.encode('ascii', 'replace')[:375].ljust(376, b'\0')
        meta.append((pid, rec))
    ini[1] = f'num_items={slot - 1}'
    open(os.path.join(OUT, 'OPENMENU.INI'), 'w', newline='\r\n').write('\n'.join(ini) + '\n')
    write_dat(os.path.join(OUT, 'BOX.DAT'), box, 131104)
    write_dat(os.path.join(OUT, 'ICON.DAT'), icon, 32800)
    write_dat(os.path.join(OUT, 'META.DAT'), meta, 384)
    art('SHENMUE', ('#10281f', '#6fb58a', '#c9b27a'), 256).save(os.path.join(ROOT, 'swirl', 'build', 'sample_art.png'))
    print(f'{slot - 2} slots, {len(box)} covers, {len(meta)} metadata records')

main()
