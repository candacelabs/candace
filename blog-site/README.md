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
go run . serve
```

renders the site into a temporary directory and serves it on
`127.0.0.1:1313`. `go run . render -out public` writes the static output;
add `-offline` to skip the GitHub API call.

The site ships no client-side JavaScript, no trackers, and no remote assets;
`scripts/check-public-content.sh` enforces the privacy boundary on every
publish.
