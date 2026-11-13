#!/usr/bin/env python3
"""Builds the website from site/ into build/site/: the Simplified Chinese
page at /, Traditional Chinese at /zh-tw/, English at /en/ and Japanese at
/ja/, each holding one language only.

site/index.html carries Chinese and English, three ways:
  <span class="zh">…</span><span class="en">…</span>   text
  <!--zh-->…<!--/zh-->  <!--en-->…<!--/en-->          blocks, in <head>
  attr="…" en:attr="…"                                 attributes, Chinese first
Opened as it is, it shows the Chinese page.

Traditional Chinese and Japanese are in scripts/site-i18n.json, keyed by
the Chinese: {"中文": {"zh-Hant": "…", "ja": "…"}}. Their pages are the
Chinese one with every string looked up there, and the build stops at a
string the table lacks or one the page no longer has.
"""
import json
import re
import shutil
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "site"
OUT = ROOT / "build" / "site"
TABLE = ROOT / "scripts" / "site-i18n.json"
SITE = "https://bowel.dev/"

# Each language: its directory, its <html lang>, its Open Graph locale, and
# its name in itself. The key is also what its screenshots and card are
# named by (shots/home-ja.webp, og-ja.png), as the app names its languages.
LANGS = {
    "zh": {"dir": "", "html": "zh-CN", "og": "zh_CN", "name": "简体中文"},
    "zh-Hant": {"dir": "zh-tw/", "html": "zh-TW", "og": "zh_TW", "name": "繁體中文"},
    "en": {"dir": "en/", "html": "en", "og": "en_US", "name": "English"},
    "ja": {"dir": "ja/", "html": "ja", "og": "ja_JP", "name": "日本語"},
}
WRITTEN = ("zh", "en")  # the languages index.html holds

SPAN = {l: re.compile(r'<span class="%s">(.*?)</span>' % l, re.S) for l in WRITTEN}
BLOCK = {l: re.compile(r"[ \t]*<!--%s-->\n?(.*?)<!--/%s-->\n?" % (l, l), re.S) for l in WRITTEN}
TAG_WITH_EN = re.compile(r"<[a-zA-Z][^<>]*\sen:[^<>]*>")
EN_ATTR = re.compile(r'\sen:([\w-]+)="([^"]*)"')
RELATIVE = re.compile(r'\b(src|href)="(?!#|/|\.\./|[a-z][a-z0-9+.-]*:)([^"]+)"')
# The Chinese in a head block: the title, and quoted values.
HEAD_TEXT = re.compile(r'(<title>)([^<]*)(</title>)|(")([^"\n]*[一-鿿][^"\n]*)(")')
LOCALE = re.compile(r'<meta property="og:type"')
HTML_TAG = re.compile(r'<html lang="zh-CN" class="zh">')
NOW = re.compile(r"(<summary[^>]*>.*?<b>)[^<]*(</b></summary>)", re.S)


class Table:
    """site-i18n.json for one language, remembering which strings were asked for."""

    def __init__(self, lang):
        self.lang = lang
        self.rows = json.loads(TABLE.read_text(encoding="utf-8"))
        self.used = set()

    def __call__(self, zh):
        row = self.rows.get(zh)
        if row is None or not row.get(self.lang):
            sys.exit("build-site: site-i18n.json has no %s for: %s" % (self.lang, zh))
        self.used.add(zh)
        return row[self.lang]

    def unused(self):
        return [k for k in self.rows if k not in self.used]


def attrs(tag, lang, tr):
    """Drops a tag's en: attributes. In English they go over the plain ones,
    and in a looked-up language the plain ones are looked up."""
    pairs = EN_ATTR.findall(tag)
    tag = EN_ATTR.sub("", tag)
    if lang == "zh":
        return tag
    for name, value in pairs:
        plain = re.compile(r'(\s)%s="([^"]*)"' % re.escape(name))
        m = plain.search(tag)
        if not m:
            sys.exit("build-site: en:%s has no %s to replace in %s" % (name, name, tag))
        if tr:
            value = tr(m.group(2))
        tag = plain.sub(lambda m: '%s%s="%s"' % (m.group(1), name, value), tag, count=1)
    return tag


def head(block, lang, tr):
    """A Chinese head block in a looked-up language, about that language's page."""
    def text(m):
        if m.group(1):
            return m.group(1) + tr(m.group(2)) + m.group(3)
        return m.group(4) + tr(m.group(5)) + m.group(6)
    block = HEAD_TEXT.sub(text, block)
    block = block.replace('"%s"' % SITE, '"%s%s"' % (SITE, LANGS[lang]["dir"]))
    return block.replace("og-zh.png", "og-%s.png" % lang)


def render(html, lang):
    info = LANGS[lang]
    tr = None if lang in WRITTEN else Table(lang)
    base = lang if lang in WRITTEN else "zh"
    other = "en" if base == "zh" else "zh"
    for l in WRITTEN:
        for m in SPAN[l].finditer(html):
            if "<span" in m.group(1):
                sys.exit("build-site: a %s span holds another span: %s" % (l, m.group(0)[:80]))
    html = SPAN[other].sub("", html)
    html = SPAN[base].sub(lambda m: tr(m.group(1)) if tr else m.group(1), html)
    html = BLOCK[other].sub("", html)
    html = BLOCK[base].sub(lambda m: head(m.group(1), lang, tr) if tr else m.group(1), html)
    html = TAG_WITH_EN.sub(lambda m: attrs(m.group(0), lang, tr), html)

    # The page says which language it is in, and which others there are.
    html, n = HTML_TAG.subn('<html lang="%s" class="%s">' % (info["html"], lang if lang in WRITTEN else lang.lower()), html)
    if n != 1:
        sys.exit("build-site: index.html's <html> tag is not the one this script expects")
    locales = '<meta property="og:locale" content="%s">\n' % info["og"]
    locales += "".join('<meta property="og:locale:alternate" content="%s">\n' % v["og"] for k, v in LANGS.items() if k != lang)
    html, n = LOCALE.subn(lambda m: locales + m.group(0), html, count=1)
    if n != 1:
        sys.exit("build-site: no og:type to put the locales before")
    html, n = NOW.subn(lambda m: m.group(1) + info["name"] + m.group(2), html)
    here = '<a href="%s" hreflang="%s" lang="%s">' % (info["dir"] or "./", info["html"], info["html"])
    if n != 1 or html.count(here) != 1:
        sys.exit("build-site: the language menu is not the one this script expects")
    html = html.replace(here, here[:-1] + ' aria-current="page">')

    if info["dir"]:
        # The page sits one directory down, and shows its own screenshots.
        html = RELATIVE.sub(lambda m: '%s="../%s"' % (m.group(1), m.group(2)), html)
        html = html.replace('".././"', '"../"')
        html = re.sub(r"-zh(-dark)?\.webp", lambda m: "-%s%s.webp" % (lang, m.group(1) or ""), html)
    left = re.search(r'<span class="(zh|en)"|<!--/?(zh|en)-->|\sen:[\w-]+=', html)
    if left:
        sys.exit("build-site: %s page still has language markup: %s" % (lang, left.group(0)))
    if tr and tr.unused():
        sys.exit("build-site: site-i18n.json has strings the page no longer has: %s" % "; ".join(tr.unused()))
    for f in re.findall(r'"(?:\.\./)?((?:shots/|og-)[^"]+\.(?:webp|png))"', html) + re.findall(re.escape(SITE) + r"((?:shots/|og-)[^\"]+)", html):
        if not (SRC / f).exists():
            sys.exit("build-site: %s page shows site/%s, which does not exist" % (lang, f))
    return html


def main():
    if OUT.exists():
        shutil.rmtree(OUT)
    shutil.copytree(SRC, OUT)
    html = (SRC / "index.html").read_text(encoding="utf-8")
    for lang, info in LANGS.items():
        d = OUT / info["dir"]
        d.mkdir(exist_ok=True)
        (d / "index.html").write_text(render(html, lang), encoding="utf-8")
    print("built %s" % OUT.relative_to(ROOT))


if __name__ == "__main__":
    main()
