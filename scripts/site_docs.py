"""The website's documentation: Markdown in docs/, as pages under /docs/
(Simplified Chinese) and /en/docs/ (English). build-site.py calls build().

A page is written once per language beside each other, as the repository
already names them: cli.md in English, cli.zh-CN.md in Chinese. A page
with one language only is shown in it on both sites, with a line saying
so, and its canonical address is the language it is written in.

Links are written as they work on GitHub, relative to the file, and the
build turns them into the site's: a link to another page becomes that
page in the same language, one into site/ the site's own file, and any
other file in the repository its page on GitHub. A link to a file or a
heading that does not exist stops the build.

Traditional Chinese and Japanese have no documentation of their own:
the landing page in those languages links to the Chinese and English
docs, as the app falls back to them for strings it lacks.
"""
import html as htmlmod
import posixpath
import re
import sys
from pathlib import Path

try:
    import markdown
    from markdown.extensions.toc import slugify_unicode
except ImportError:
    sys.exit("build-site: the docs need Python-Markdown: pip install -r scripts/site-requirements.txt")

ROOT = Path(__file__).resolve().parent.parent
SITE = "https://bowel.dev"
REPO = "https://github.com/21stware/bowel"

# The pages, in the order the sidebar lists them: (slug, source without
# its .md or .zh-CN.md, the sidebar section). The first is /docs/ itself.
PAGES = [
    ("", "docs/guide/README", "guide"),
    ("install", "docs/guide/install", "guide"),
    ("routes", "docs/guide/routes", "guide"),
    ("takeover", "docs/guide/takeover", "guide"),
    ("faq", "docs/guide/faq", "guide"),
    ("cli", "docs/cli", "reference"),
    ("gateway", "docs/gateway", "reference"),
]

LANGS = {
    "zh": {"suffix": ".zh-CN.md", "prefix": "/docs/", "html": "zh-CN", "name": "简体中文", "home": "/"},
    "en": {"suffix": ".md", "prefix": "/en/docs/", "html": "en", "name": "English", "home": "/en/"},
}

WORDS = {
    "zh": {
        "docs": "文档", "guide": "使用指南", "reference": "参考", "download": "下载",
        "source": "在 GitHub 上查看和修改这一页", "readme": "README",
        "only": "这一页目前只有英文。", "menu": "语言", "pages": "文档目录",
        "suffix": "bowel 文档",
    },
    "en": {
        "docs": "Docs", "guide": "Guide", "reference": "Reference", "download": "Download",
        "source": "View and edit this page on GitHub", "readme": "README",
        "only": "This page is only in Chinese for now.", "menu": "Language", "pages": "Pages",
        "suffix": "bowel docs",
    },
}

H1 = re.compile(r"^# (.+)$", re.M)
LINK = re.compile(r'(<(?:a|img)\s[^>]*?\b(?:href|src)=")([^"]*)(")')
IDS = re.compile(r'\sid="([^"]+)"')


def source(base, lang):
    """The file a page is written in for lang, and the language it is in."""
    own = ROOT / (base + LANGS[lang]["suffix"])
    if own.exists():
        return own, lang
    other = "en" if lang == "zh" else "zh"
    theirs = ROOT / (base + LANGS[other]["suffix"])
    if theirs.exists():
        return theirs, other
    sys.exit("build-site: docs page %s has neither %s nor %s" % (base, own.name, theirs.name))


def url(slug, lang):
    return LANGS[lang]["prefix"] + (slug + "/" if slug else "")


class Docs:
    def __init__(self):
        # Every file a page is written in, to the page's slug.
        self.slugs = {}
        for slug, base, _ in PAGES:
            for info in LANGS.values():
                f = ROOT / (base + info["suffix"])
                if f.exists():
                    self.slugs[f.relative_to(ROOT).as_posix()] = slug
        self.ids = {}       # (slug, lang) -> the ids its headings have
        self.anchors = []   # (page it is on, slug, lang, fragment) to check at the end

    def link(self, href, src, lang, here):
        """A link as written in src, as the page in lang shows it."""
        if not href or href.startswith(("#", "http:", "https:", "mailto:", "/")):
            if href.startswith("#") and len(href) > 1:
                written = "zh" if src.endswith(".zh-CN.md") else "en"
                self.anchors.append((here, self.slugs[src], written, href[1:]))
            return href
        path, _, frag = href.partition("#")
        target = posixpath.normpath(posixpath.join(posixpath.dirname(src), path))
        if target.startswith("../") or not (ROOT / target).exists():
            sys.exit("build-site: %s links to %s, which does not exist" % (src, href))
        if target in self.slugs:
            slug = self.slugs[target]
            # The page in the language it was linked in, if the link names a
            # translation; the anchor is that file's heading.
            linked = "zh" if target.endswith(".zh-CN.md") else "en"
            out = url(slug, lang)
            if slug == here[0] and linked != lang:
                # This page's translation, named as such ("中文版见 …").
                out = url(slug, linked)
            if frag:
                self.anchors.append((here, slug, linked, frag))
                if linked != lang:
                    # A heading of the other language's file is only on its page.
                    out = url(slug, linked)
                out += "#" + frag
            return out
        if target.startswith("site/"):
            return "/" + target[len("site/"):] + ("#" + frag if frag else "")
        kind = "tree" if (ROOT / target).is_dir() else "blob"
        return "%s/%s/main/%s%s" % (REPO, kind, target, "#" + frag if frag else "")

    def page(self, slug, base, lang):
        f, written = source(base, lang)
        src = f.relative_to(ROOT).as_posix()
        text = f.read_text(encoding="utf-8")
        m = H1.search(text)
        if not m:
            sys.exit("build-site: %s has no title (a line starting with '# ')" % src)
        md = markdown.Markdown(
            extensions=["tables", "fenced_code", "sane_lists", "toc"],
            extension_configs={"toc": {"slugify": slugify_unicode}},
        )
        body = md.convert(text)
        if "```" in body:
            # Python-Markdown, unlike GitHub, leaves a fence inside a list item as text.
            sys.exit("build-site: %s has a code block the site cannot show; take it out of its list" % src)
        body = LINK.sub(lambda l: l.group(1) + htmlmod.escape(self.link(htmlmod.unescape(l.group(2)), src, lang, (slug, lang)), quote=True) + l.group(3), body)
        self.ids[(slug, written)] = set(IDS.findall(body))
        return {"title": plain(m.group(1)), "body": body, "src": src, "written": written, "lede": lede(body)}


def plain(s):
    """Markdown inline text with its marks taken out."""
    s = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", s)
    return re.sub(r"[`*_]", "", s).strip()


def lede(body):
    """The page's first paragraph as text, for its description."""
    m = re.search(r"<p>(.*?)</p>", body, re.S)
    text = htmlmod.unescape(re.sub(r"<[^>]+>", "", m.group(1))) if m else ""
    text = " ".join(text.split())
    return text if len(text) <= 160 else text[:157].rstrip() + "…"


def render(slug, lang, pages, titles):
    p = pages[(slug, lang)]
    w = WORDS[lang]
    info = LANGS[lang]
    esc = htmlmod.escape
    title = p["title"] if slug == "" else "%s · %s" % (p["title"], w["suffix"])
    canonical = SITE + url(slug, p["written"])
    alternates = ""
    if all(pages[(slug, l)]["written"] == l for l in LANGS):
        alternates = "".join('<link rel="alternate" hreflang="%s" href="%s%s">\n' % (LANGS[l]["html"], SITE, url(slug, l)) for l in LANGS)
        alternates += '<link rel="alternate" hreflang="x-default" href="%s%s">\n' % (SITE, url(slug, "en"))

    side = []
    for section in ("guide", "reference"):
        side.append('<p class="doc-sec">%s</p>\n<ul>' % w[section])
        for s, _, sec in PAGES:
            if sec != section:
                continue
            cur = ' aria-current="page"' if s == slug else ""
            tag = ""
            if pages[(s, lang)]["written"] != lang:
                other = pages[(s, lang)]["written"]
                tag = ' <span class="doc-tag" lang="%s">%s</span>' % (LANGS[other]["html"], "EN" if other == "en" else "中文")
            side.append('<li><a href="%s"%s>%s%s</a></li>' % (url(s, lang), cur, esc(titles[(s, lang)]), tag))
        side.append("</ul>")

    menu = "".join(
        '<a href="%s" hreflang="%s" lang="%s"%s>%s</a>' % (url(slug, l), LANGS[l]["html"], LANGS[l]["html"], ' aria-current="page"' if l == lang else "", LANGS[l]["name"])
        for l in LANGS
    )
    only = '<p class="doc-only">%s</p>\n' % w["only"] if p["written"] != lang else ""
    article_lang = ' lang="%s"' % LANGS[p["written"]]["html"] if p["written"] != lang else ""

    return """<!DOCTYPE html>
<html lang="{html}" class="{lang} docs">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title}</title>
<meta name="description" content="{desc}">
<link rel="canonical" href="{canonical}">
{alternates}<meta property="og:type" content="article">
<meta property="og:site_name" content="bowel">
<meta property="og:title" content="{title}">
<meta property="og:description" content="{desc}">
<meta property="og:url" content="{canonical}">
<meta property="og:image" content="{site}/og-{card}.png">
<meta name="twitter:card" content="summary_large_image">
<meta name="theme-color" content="#f6f4ef" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#171615" media="(prefers-color-scheme: dark)">
<link rel="icon" href="/icon.png">
<link rel="stylesheet" href="/style.css">
</head>
<body>

<header class="nav scrolled">
  <a class="brand" href="{home}"><img src="/icon.png" alt="" width="28" height="28"><span>bowel</span></a>
  <nav class="links">
    <a href="{docs}" aria-current="page">{w_docs}</a>
    <a href="{home}#download">{w_download}</a>
  </nav>
  <div class="nav-end">
    <details class="lang">
      <summary aria-label="{w_menu}"><svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.6 2.7 3.9 5.7 3.9 9s-1.3 6.3-3.9 9c-2.6-2.7-3.9-5.7-3.9-9S9.4 5.7 12 3z"/></svg><b>{name}</b></summary>
      <div class="lang-menu">{menu}</div>
    </details>
    <a class="gh" href="{repo}" aria-label="GitHub"><svg viewBox="0 0 16 16" width="20" height="20" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg></a>
  </div>
</header>

<div class="doc">
<aside class="doc-side" aria-label="{w_pages}">
{side}
</aside>
<main class="doc-main">
{only}<article class="doc-body"{article_lang}>
{body}
</article>
<p class="doc-src"><a href="{repo}/blob/main/{src}">{w_source}</a></p>
</main>
</div>

</body>
</html>
""".format(
        html=info["html"], lang=lang, title=esc(title), desc=esc(p["lede"], quote=True), canonical=canonical,
        alternates=alternates, site=SITE, card="zh" if lang == "zh" else "en", home=info["home"], docs=url("", lang),
        w_docs=w["docs"], w_download=w["download"], w_menu=w["menu"], name=info["name"], menu=menu, repo=REPO,
        w_pages=w["pages"], side="\n".join(side), only=only, article_lang=article_lang, body=p["body"],
        src=p["src"], w_source=w["source"],
    )


def sitemap(out):
    """Adds the docs to the sitemap the site already has."""
    f = out / "sitemap.xml"
    xml = f.read_text(encoding="utf-8")
    rows = []
    for slug, base, _ in PAGES:
        both = all((ROOT / (base + LANGS[l]["suffix"])).exists() for l in LANGS)
        for lang in LANGS:
            _, written = source(base, lang)
            if written != lang:
                continue  # its canonical address is the other language's
            row = "  <url>\n    <loc>%s%s</loc>\n" % (SITE, url(slug, lang))
            if both:
                row += "".join('    <xhtml:link rel="alternate" hreflang="%s" href="%s%s"/>\n' % (LANGS[l]["html"], SITE, url(slug, l)) for l in LANGS)
            rows.append(row + "  </url>\n")
    if "</urlset>" not in xml:
        sys.exit("build-site: site/sitemap.xml has no </urlset>")
    f.write_text(xml.replace("</urlset>", "".join(rows) + "</urlset>"), encoding="utf-8")


# Files outside the docs that link into them; a link that leads nowhere stops the build.
LINKERS = ["README.md", "README.zh-CN.md", "site/llms.txt"]
INTO = re.compile(re.escape(SITE) + r"(/(?:en/)?docs/[^)\s\"'#>]*)(?:#([^)\s\"'>]+))?")


def check_linkers(docs):
    at = {}
    for slug, _, _ in PAGES:
        for lang in LANGS:
            at[url(slug, lang)] = (slug, lang)
    for name in LINKERS:
        for m in INTO.finditer((ROOT / name).read_text(encoding="utf-8")):
            path, frag = m.group(1), m.group(2)
            if path not in at:
                sys.exit("build-site: %s links to %s%s, which is no docs page" % (name, SITE, path))
            slug, lang = at[path]
            _, written = source(dict((s, b) for s, b, _ in PAGES)[slug], lang)
            if frag and frag not in docs.ids.get((slug, written), ()):
                sys.exit("build-site: %s links to %s%s#%s, which has no such heading" % (name, SITE, path, frag))


def build(out):
    docs = Docs()
    pages = {}
    for slug, base, _ in PAGES:
        for lang in LANGS:
            pages[(slug, lang)] = docs.page(slug, base, lang)
    for here, slug, lang, frag in docs.anchors:
        if frag not in docs.ids.get((slug, lang), ()):
            sys.exit("build-site: the docs page %s (%s) links to #%s on %s (%s), which has no such heading" % (here[0] or "index", here[1], frag, slug or "index", lang))
    check_linkers(docs)
    titles = {k: v["title"] for k, v in pages.items()}
    for slug, _, _ in PAGES:
        for lang in LANGS:
            d = out / url(slug, lang).strip("/")
            d.mkdir(parents=True, exist_ok=True)
            (d / "index.html").write_text(render(slug, lang, pages, titles), encoding="utf-8")
    sitemap(out)
