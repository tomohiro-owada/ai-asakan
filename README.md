# AI朝刊 (ai-asakan)

Daily Japanese-newspaper-style AI news front page, published as a static site at <https://news.abalol.xyz/> via GitHub Pages.

- `issues/digest-YYYY-MM-DD.json` — source of truth, one file per issue (schema: see any file; optional `tomo` string on any item, optional `numbers` array for the 「きょうの数字」 box).
- `main.go`, `template.html`, `archive.html`, `style.css` — Go generator (stdlib only).
- `og/YYYY-MM-DD.jpg` — OGP image per issue (rendered locally by `publish.sh`; `og/default.jpg` is the fallback).
- `.github/workflows/pages.yml` — on push to `main`: `go run . -site issues -out _site` → deploy to Pages.
- `publish.sh` — add an issue, validate, commit, push (see header of the script).

```
go run . -site issues -out _site              # build the whole site
go run . -in issues/digest-2026-10-30.json    # single page -> ai-shimbun-2026-10-30.html
go run . -in X.json -render                   # + PNG/PDF (node + playwright, see render.mjs)
```

Output: `/` (latest), `/YYYY/MM/DD/`, `/archive/`, `/feed.xml`, `/sitemap.xml`, `/robots.txt`, `CNAME`.
