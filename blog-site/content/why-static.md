---
title: "Why this blog stays static"
date: 2025-08-24
lastmod: 2026-07-29
description: "Hugo keeps the public surface small, fast, and easier to reason about."
tags: ["hugo", "architecture", "privacy"]
aliases:
  - "/posts/first-post/"
---

A public blog does not need an application runtime for every request. Hugo builds the pages ahead of time, which keeps the serving path small and predictable.

## Less machinery

The published site is HTML and CSS. There is no client-side application bundle, account system, comment widget, analytics script, or remote font request.

That gives the site a few useful properties:

- pages remain readable without JavaScript;
- builds are reproducible in a pinned container;
- the public output can be scanned before release;
- fewer third parties learn that someone visited a page.

## A narrow privacy boundary

The source focuses on projects and reusable engineering decisions. Individual names, contact details, personal profiles, precise locations, and private network topology stay outside the publication boundary.

The production check is deliberately simple:

```bash
hugo --minify --gc
./scripts/check-public-content.sh bloghome/public
```

Static does not automatically mean private, but it makes the public surface easier to inspect.
