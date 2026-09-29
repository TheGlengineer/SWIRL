#!/usr/bin/env python3
"""Writes the crafted OPENMENU.INI and DAT cases for parsers_check into a folder.
Usage: mkcases.py <OPENMENU.INI> <META.DAT> <outdir>. Prints one line per case: kind, path, expected games (-1: any)."""
import os, struct, sys

ini_path, dat_path, out = sys.argv[1], sys.argv[2], sys.argv[3]
os.makedirs(out, exist_ok=True)
base = open(ini_path).read().replace('\r\n', '\n')
hdr, *items = base.split('\n\n')
count = int(hdr.split('num_items=')[1].split()[0])
games = count - 1
serial = [l for l in items[0].split('\n') if '.product=' in l][0].split('=')[1]

def ini(name, text, expected=-1):
    p = os.path.join(out, name + '.ini')
    open(p, 'w').write(text)
    print('ini', p, expected)

body = '\n\n'.join(items)
ini('ok', hdr + '\n\n' + body, games)
ini('slot0_first', hdr + '\n\n[ITEMS]\n0.name=Zero\n\n' + body, games)
ini('slot0_last', hdr + '\n\n' + body + '\n0.name=Zero\n', games)
ini('slot5000_last', hdr + '\n\n' + body + '\n5000.name=Far\n', games)
ini('slot5digit', hdr + '\n\n' + body + '\n12345.name=Big\n', games)
ini('before_header', '[ITEMS]\n05.name=Early\n\n' + hdr + '\n\n' + body, games)
ini('underreport', hdr.replace('num_items=%d' % count, 'num_items=%d' % (count - 3)) + '\n\n' + body, games)
ini('overreport', hdr.replace('num_items=%d' % count, 'num_items=%d' % (count + 3)) + '\n\n' + body, games)
ini('badline', hdr + '\n\n' + items[0] + '\nthis is not a key\n\n' + '\n\n'.join(items[1:]), games)
ini('longline', hdr + '\n\n' + body.replace('02.name=', '02.name=' + 'W' * 450 + ' ', 1), games)
ini('num_huge', hdr.replace('num_items=%d' % count, 'num_items=99999999') + '\n\n' + body, -1)
ini('num_negative', hdr.replace('num_items=%d' % count, 'num_items=-5') + '\n\n' + body, -1)
ini('num_zero', hdr.replace('num_items=%d' % count, 'num_items=0') + '\n\n' + body, -1)
ini('no_header', body, 0)
ini('empty', '', 0)
ini('junk_keys', hdr + '\n\n[ITEMS]\n.name=x\nabc.name=y\n1.=z\n=\n[unterminated\n' + '\n\n'.join(items[1:]), games)
ini('dup_header', hdr + '\n\n' + body + '\n[OPENMENU]\nnum_items=5\n', games)
extra = ''.join('%02d.name=SET %d\n%02d.disc=%d/12\n%02d.product=%s\n\n' % (i, i, i, i - count, i, serial)
                for i in range(count + 1, count + 13))
ini('multidisc12', hdr.replace('num_items=%d' % count, 'num_items=%d' % (count + 12)) + '\n\n' + body + '\n\n' + extra, games + 12)

d = open(dat_path, 'rb').read()
def dat(name, b):
    p = os.path.join(out, name + '.dat')
    open(p, 'wb').write(b)
    print('dat', p, -1)
cs, nc = struct.unpack_from('<II', d, 4)
def patched(off, fmt, v):
    h = bytearray(d); struct.pack_into(fmt, h, off, v); return bytes(h)
dat('ok', d)
dat('empty', patched(8, '<I', 0)[:16])
dat('hugecount', patched(8, '<I', 10**9))
dat('zerochunk', patched(4, '<I', 0))
dat('wrongchunk', patched(4, '<I', 100))
dat('badoffset', patched(16 + 12, '<I', 0xFFFFFF00))
h = bytearray(d); h[16:28] = b'ABCDEFGHIJKL'; dat('nonul', bytes(h))
dat('trunc100', d[:100])
dat('trunc8', d[:8])
dat('version2', patched(3, 'B', 2))
