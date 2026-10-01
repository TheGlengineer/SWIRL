#!/usr/bin/env python3
"""SWIRL Card Manager window: the language files.

  cm_lang.py export     writes swirl/cardmanager/web/lang/keys.json, every English string the window can
                        show: the page as written (read by the page's own translateDOM in a headless
                        Chromium, so the keys are exactly what it looks up) plus every t("...") and
                        tn(n, "...", "...") in the script
  cm_lang.py check      checks swirl/cardmanager/web/lang/<code>.json against the keys: entries for
                        strings the window no longer has, strings not translated, {placeholders} and
                        inline tags that differ from the English

A language file maps the English string to its translation. {name} placeholders and tags such as <b>
stay as they are; {pc} becomes PC or Mac on the user's machine. An entry left empty shows the English.
Needs playwright (pip install playwright) for export; check needs only Python."""
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
WEB = os.path.join(ROOT, "swirl", "cardmanager", "web")
LANG_DIR = os.path.join(WEB, "lang")
CODES = ["de", "fr", "es", "it", "pt"]


def script_keys(html):
    js = html.split("<script>", 1)[1].split("</script>", 1)[0]
    keys = []
    for m in re.finditer(r'\bt\(\s*("(?:[^"\\]|\\.)*"|\'(?:[^\'\\]|\\.)*\')', js):
        keys.append(json.loads(m.group(1)) if m.group(1)[0] == '"' else m.group(1)[1:-1].replace("\\'", "'"))
    for m in re.finditer(r'\btn\([^,]+,\s*("(?:[^"\\]|\\.)*"),\s*("(?:[^"\\]|\\.)*")', js):
        keys.append(json.loads(m.group(1)))
        keys.append(json.loads(m.group(2)))
    return keys


def page_keys(html_path):
    from playwright.sync_api import sync_playwright
    with sync_playwright() as p:
        b = p.chromium.launch()
        pg = b.new_page()
        pg.add_init_script("window.__i18nDump = [];")
        pg.goto("file://" + html_path)
        pg.wait_for_timeout(1500)
        keys = pg.evaluate("window.__i18nDump")
        b.close()
    return keys


def export():
    html = open(os.path.join(WEB, "index.html"), encoding="utf-8").read()
    keys = []
    for k in page_keys(os.path.join(WEB, "index.html")) + script_keys(html):
        k = k.strip()
        if not k or k in keys:
            continue
        # not text: version stamps, numbers, file names, URLs
        if re.fullmatch(r"[\d.:/\\ ×x%+-]*|v?__VERSION__|__TOKEN__", k) or k.startswith("http"):
            continue
        if not re.search(r"[A-Za-z]{2}", k):
            continue
        keys.append(k)
    os.makedirs(LANG_DIR, exist_ok=True)
    with open(os.path.join(LANG_DIR, "keys.json"), "w", encoding="utf-8") as f:
        json.dump(keys, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print("%d keys" % len(keys))


def tags(s):
    return sorted(re.findall(r"<[^>]+>", s))


def holes(s):
    return sorted(re.findall(r"\{[a-z]+\}", s))


def check():
    keys = json.load(open(os.path.join(LANG_DIR, "keys.json"), encoding="utf-8"))
    bad = 0
    for code in CODES:
        path = os.path.join(LANG_DIR, code + ".json")
        if not os.path.exists(path):
            print("%s: missing" % code)
            bad += 1
            continue
        d = json.load(open(path, encoding="utf-8"))
        stale = [k for k in d if k not in keys]
        missing = [k for k in keys if not d.get(k)]
        for k in stale:
            print("%s: not in the window any more: %r" % (code, k[:70]))
        for k, v in d.items():
            if not v or k not in keys:
                continue
            # a translation may say {pc} where the English says PC (the page swaps PC for Mac on a Mac)
            if [h for h in holes(v) if h != "{pc}" or "{pc}" in k] != holes(k) or ("{pc}" in v and "PC" not in k and "{pc}" not in k):
                print("%s: placeholders differ: %r" % (code, k[:70]))
                bad += 1
            if tags(v) != tags(k):
                print("%s: tags differ: %r" % (code, k[:70]))
                bad += 1
        print("%s: %d of %d translated, %d missing, %d stale" % (code, len(keys) - len(missing), len(keys), len(missing), len(stale)))
        for k in missing[:20]:
            print("   missing: %r" % k[:90])
        bad += len(missing)
    print("%d problems" % bad)
    return 1 if bad else 0


if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else ""
    if cmd == "export":
        export()
    elif cmd == "check":
        sys.exit(check())
    else:
        print(__doc__)
        sys.exit(2)
