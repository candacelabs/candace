# Candace Labs blog

The source for [blog.candace.cloud](https://blog.candace.cloud/): a small Go
program that renders markdown notes into a static site, published with GitHub
Pages on every update.

This repository is a generated snapshot exported from the Candace monorepo —
the monorepo is canonical, and changes made here will be overwritten by the
next export. The projects grid on the home page is derived from the
`candacelabs` public repository listing at render time; nothing on the site
is hand-maintained.

## Local development

```bash
go run . serve                      # renders to a temp dir, serves 127.0.0.1:1313
go run . render -out public         # writes the static output
go run . render -out public -offline  # ...without the GitHub API call
```

`serve` is offline by default; `render` is not, so `-offline` is what makes a
build reproducible without network.

## A post is a markdown file with front matter

Posts live in `content/*.md` and are compiled into the binary with
`//go:embed`, so the rendered site depends on no directory beside it:

```markdown
---
title: "Why this blog stays static"
date: 2025-08-24
lastmod: 2026-07-29
description: "Hugo keeps the public surface small, fast, and easier to reason about."
tags: ["hugo", "architecture", "privacy"]
aliases:
  - "/posts/first-post/"
---

A public blog does not need an application runtime for every request.
```

`aliases` is the one field that does more than describe: each alias becomes a
redirect stub at that path, which is how URLs the previous Hugo site published
keep resolving. `content/home.md` is the home page's prose rather than a post,
and is skipped by the post loader.

## What one render produces

```text
public/
├── index.html                  home: prose, five most recent posts, projects grid
├── posts/index.html            every post, newest first
├── posts/<slug>/index.html     one per post
├── <alias>/index.html          one redirect stub per alias
├── index.xml                   RSS
├── assets/blog.css             theme plus generated chroma classes
├── robots.txt
├── CNAME                       derived from Site.BaseURL, never hand-written
└── 404.html
```

Markdown is goldmark with GFM and syntax highlighting (chroma, `github-dark`,
emitted as CSS classes rather than inline styles — which is why the stylesheet
is generated rather than static).

The site ships no client-side JavaScript, no trackers, and no remote assets.
`scripts/check-public-content.sh` enforces that boundary on every publish: it
scans every generated HTML and XML page for contact details, private-profile
links, and private or tailnet addresses, and fails the publish rather than
warning.
