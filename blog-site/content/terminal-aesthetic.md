---
title: "Designing a hand-drawn systems notebook"
date: 2024-08-23
lastmod: 2026-07-29
description: "Pairing handwritten texture with precise technical structure without sacrificing readability or accessibility."
tags: ["design", "hugo", "accessibility"]
---

AI and infrastructure sites often reach for the same visual shorthand: black backgrounds, neon gradients, glowing terminals, and orbital diagrams. This redesign takes a different route. It treats the site like an engineer's field notebook—warm paper, graph lines, blue drafting ink, and annotations that feel written by hand.

## Keep the technical character in the structure

The theme does not need a terminal costume to feel technical. Monospace type still identifies navigation, metadata, and code, while a locally served handwritten display face gives the wordmark and annotations a less manufactured voice. Long-form text stays quiet and readable.

The palette has three jobs:

- cobalt acts as drafting ink for links and primary actions;
- rust and amber add restrained marks, underlines, and status accents;
- charcoal text sits on warm paper surfaces for comfortable reading.

Muted ink, graph lines, and imperfect borders do most of the remaining work.

## One visual system

The home page, lists, and individual notes now share the same paper surfaces, spacing scale, navigation, and typography. Hand-drawn details are accents rather than decoration layered over every element.

That consistency also reduces CSS complexity: the design is a small set of reusable panels, cards, metadata rows, and prose rules.

## Accessibility is part of the aesthetic

The theme includes a skip link, visible keyboard focus, semantic navigation, responsive layouts, reduced-motion support, and high-contrast text. The notebook can feel informal without asking readers to trade away the behavior of a modern reading interface.
