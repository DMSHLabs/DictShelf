---
title: Home
description: "DictShelf - a dictionary application for your own machine: dictionary groups, mask search, every match listed, and full-text search over the articles."
---

<style>
.md-typeset .hero{text-align:center;margin:1rem 0 2.2rem}
.md-typeset .hero img{width:min(200px,42vw);height:auto;margin:0 auto;display:block}
.md-typeset .hero h1{margin:.8rem 0 .6rem;font-size:2.1rem;font-weight:700;letter-spacing:-.01em}
.md-typeset .hero p{max-width:46rem;margin:0 auto 1.6rem;font-size:1.05rem;line-height:1.65}
.md-typeset .features{max-width:52rem;margin:0 auto 1.8rem}
.md-typeset .features ul{margin:0;padding:0;list-style:none}
.md-typeset .features li{margin:.7rem 0;padding-left:1.3rem;position:relative;line-height:1.6}
.md-typeset .features li::before{content:"—";position:absolute;left:0;color:var(--md-primary-fg-color)}
.md-typeset .cta{text-align:center;margin:0 0 2rem}
/* A landing page, not an article. The theme's reading chrome - the manual's
   sidebar, its prev/next pagination, and the copy/edit actions - belongs to the
   manual's pages; this page has none of it. Without :has() the rules simply do
   not apply and nothing breaks. */
body:has(.hero) .md-sidebar--primary,
body:has(.hero) .md-sidebar--secondary,
body:has(.hero) .md-footer__inner,
body:has(.hero) .md-content__button{display:none}
</style>

<div class="hero" markdown>

![DictShelf](assets/logo.svg)

# DictShelf

A dictionary application for your own machine. DictShelf indexes the
dictionaries you already own and searches them locally: no account, no upload,
nothing sent anywhere.

</div>

<div class="cta" markdown>

[GitHub](https://github.com/DMSHLabs/DictShelf){ .md-button .md-button--primary }
[Releases](https://github.com/DMSHLabs/DictShelf/releases/latest){ .md-button }

</div>

<div class="features" markdown>

-   **Dictionary groups** — keep your dictionaries in named groups, and search
    one group, one dictionary, or all of them at once.
-   **Mask search** — `?` stands for one character, `*` for any number of them,
    across the whole headword list.
-   **Every match, no ceiling** — a search reports what it found and how much
    there is in all, and the whole list is there to page through; nothing is cut
    off at a fixed number.
-   **Full text** — search the bodies of the articles, not only their headwords,
    with phrase and proximity readings when the exact wording is not what you
    remember.
-   **Supported formats** — MDict (`.mdx`), StarDict (`.ifo`), AARD2 (`.slob`),
    Babylon (`.bgl`), ZIM (`.zim`), Lingvo (`.lsd`/`.dsl`/`.dsl.dz`/`.dsl.dz2`),
    Wudict Markdown (`.wudict.md`).
-   **OS** — Android, Windows.

</div>
