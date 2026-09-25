#!/usr/bin/env python3
"""Builds assets/titles.tsv.gz: Dreamcast serial -> proper game title, from the Redump list that libretro
publishes (github.com/libretro/libretro-database, metadat/redump/Sega - Dreamcast.dat).

usage: tools/gen_titles.py [path to the .dat]   (downloads it when no path is given)
Each line: key, region, title, disc number, disc count (tab separated). The key is the serial with
everything but letters and digits removed, and a leading MK dropped (Sega USA discs say MK-51064 on the
disc and 51064 in Redump)."""
import collections, gzip, os, re, sys, urllib.request

URL = "https://raw.githubusercontent.com/libretro/libretro-database/master/metadat/redump/Sega%20-%20Dreamcast.dat"
OUT = os.path.join(os.path.dirname(__file__), "..", "assets", "titles.tsv.gz")

def key(serial):
    k = re.sub(r"[^A-Z0-9]", "", serial.upper())
    if re.match(r"^MK\d", k):
        k = k[2:]
    return k

def clean(name):
    base = re.split(r" \(", name, 1)[0].strip()
    # Redump moves leading articles to the end: "House of the Dead 2, The" -> "The House of the Dead 2"
    parts = base.split(" - ", 1)
    m = re.match(r"^(.*), (The|A|An)$", parts[0])
    if m:
        parts[0] = m.group(2) + " " + m.group(1)
    return " - ".join(parts)

def main():
    src = sys.argv[1] if len(sys.argv) > 1 else None
    text = open(src, encoding="utf-8").read() if src else urllib.request.urlopen(URL).read().decode("utf-8")
    games = re.findall(r'game \(\n\tname "(.*?)"\n\tregion "(.*?)"\n(?:\tserial "(.*?)"\n)?', text)
    discs = collections.defaultdict(set)
    for name, region, _ in games:
        m = re.search(r"\(Disc (\d)\)", name)
        if m:
            discs[(clean(name), region)].add(int(m.group(1)))
    rows = set()
    for name, region, serials in games:
        if not serials:
            continue
        m = re.search(r"\(Disc (\d)\)", name)
        d = int(m.group(1)) if m else 1
        n = max(discs.get((clean(name), region), {1}) | {d})
        for s in re.split(r",\s*", serials):
            k = key(s)
            if len(k) >= 4:
                rows.add((k, region, clean(name), d, n))
    with gzip.open(OUT, "wt", encoding="utf-8") as f:
        for r in sorted(rows):
            f.write("%s\t%s\t%s\t%d\t%d\n" % r)
    print(len(rows), "serials ->", os.path.relpath(OUT))

main()
