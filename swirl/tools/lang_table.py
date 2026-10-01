#!/usr/bin/env python3
"""SWIRL language table tool.

  lang_table.py export          writes swirl/lang/en.json (the reference for translators) and keys.txt (the
                                table order the Card Manager compiles by) from ui/swirl/sw_lang.h
  lang_table.py check           checks every swirl/lang/*.json against the table: unknown keys, missing keys,
                                printf specifiers that differ from the English, characters the fonts lack
  lang_table.py dat OUT         builds a LANG.DAT from every swirl/lang/*.json except en (the Card Manager does
                                the same in Go: swirl/cardmanager/lang.go; lang_test.go keeps them equal)

The key order of sw_lang.h is the file order; the table hash in LANG.DAT (FNV-1a of the key names joined by
newlines) ties a file to the table it was made for."""
import json
import os
import re
import struct
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
HEADER = os.path.join(ROOT, "ui", "swirl", "sw_lang.h")
LANG_DIR = os.path.join(ROOT, "swirl", "lang")
# language code -> SW_LANG_* value (sw_lang.h) and its own name
CODES = {"en": 1, "de": 2, "fr": 3, "es": 4, "it": 5, "pt": 6}
SPEC = re.compile(r"%[0-9]*[a-z]")


def table():
    src = open(HEADER, encoding="utf-8").read()
    pairs = re.findall(r'X\((S_[A-Z0-9_]+), "((?:[^"\\]|\\.)*)"\)', src)
    out = []
    for key, text in pairs:
        out.append((key, text.encode("utf-8").decode("unicode_escape").encode("latin-1").decode("utf-8")))
    return out


def table_hash(keys):
    h = 2166136261
    for k in keys:
        for b in (k + "\n").encode():
            h = ((h ^ b) * 16777619) & 0xFFFFFFFF
    return h


def export():
    t = table()
    os.makedirs(LANG_DIR, exist_ok=True)
    path = os.path.join(LANG_DIR, "en.json")
    with open(path, "w", encoding="utf-8") as f:
        json.dump(dict(t), f, ensure_ascii=False, indent=2)
        f.write("\n")
    with open(os.path.join(LANG_DIR, "keys.txt"), "w", encoding="utf-8") as f:
        f.write("".join(k + "\n" for k, _ in t))
    print("%s: %d strings (keys.txt holds the order)" % (path, len(t)))


def check():
    t = dict(table())
    bad = 0
    for name in sorted(os.listdir(LANG_DIR)):
        if not name.endswith(".json"):
            continue
        code = name[:-5]
        if code not in CODES:
            print("%s: unknown language code" % name)
            bad += 1
            continue
        d = json.load(open(os.path.join(LANG_DIR, name), encoding="utf-8"))
        for k, v in d.items():
            if k not in t:
                print("%s: unknown key %s" % (name, k))
                bad += 1
            elif SPEC.findall(v) != SPEC.findall(t[k]):
                print("%s: %s has specifiers %s, English has %s" % (name, k, SPEC.findall(v), SPEC.findall(t[k])))
                bad += 1
            elif any(ord(c) > 255 for c in v):
                print("%s: %s has characters the fonts lack: %s" % (name, k, "".join(c for c in v if ord(c) > 255)))
                bad += 1
        if code != "en":
            missing = [k for k in t if k not in d or not d[k]]
            if missing:
                print("%s: %d strings fall back to English: %s" % (name, len(missing), " ".join(missing[:8])))
    print("%d problems" % bad)
    return 1 if bad else 0


def build_dat(out):
    t = table()
    keys = [k for k, _ in t]
    langs = []
    for name in sorted(os.listdir(LANG_DIR)):
        code = name[:-5]
        if not name.endswith(".json") or code == "en" or code not in CODES:
            continue
        d = json.load(open(os.path.join(LANG_DIR, name), encoding="utf-8"))
        strings = b""
        offsets = []
        for k in keys:
            v = d.get(k, "")
            if v:
                offsets.append(4 * len(keys) + len(strings))
                strings += v.encode("utf-8") + b"\0"
            else:
                offsets.append(0)
        block = b"".join(struct.pack("<I", o) for o in offsets) + strings
        langs.append((CODES[code], block))
    hdr = b"SWL1" + struct.pack("<III", table_hash(keys), len(keys), len(langs))
    off = 16 + 16 * len(langs)
    entries = b""
    body = b""
    for code, block in langs:
        entries += struct.pack("<BBBBIII", code, 0, 0, 0, off + len(body), len(block), 0)
        body += block
    with open(out, "wb") as f:
        f.write(hdr + entries + body)
    print("%s: %d languages, %d bytes" % (out, len(langs), 16 + len(entries) + len(body)))


if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else ""
    if cmd == "export":
        export()
    elif cmd == "check":
        sys.exit(check())
    elif cmd == "dat" and len(sys.argv) > 2:
        build_dat(sys.argv[2])
    else:
        print(__doc__)
        sys.exit(2)
