# Renderer rules

## CSS: always inline in template.html

Do NOT use a separate style.css file or inject CSS via Go template variables.
WebView2 (`SetHtml` / `NavigateToString`) does not load external files —
only the HTML string passed to it is available. There is no base URL, so
`<link>` tags and external file references silently fail.

**Rule:** put all CSS inside a `<style>` block directly in `template.html`.

## Inline parser priorities are load-bearing

goldmark's own inline parsers sit at fixed priorities: code spans at 100, links
at 200, emphasis at 500 (`vendor/github.com/yuin/goldmark/parser/parser.go`).
An extension triggering on a byte goldmark already claims must be registered
ahead of the built-in parser or it never sees the input.

`wikilink.go` triggers on `[`, the same byte as the link parser, so it registers
at **150** — below links, above code spans. Raising it above 200 silently breaks
`[[…]]`; lowering it below 100 would make `` `[[…]]` `` inside backticks render
as a link.

## Per-document state travels through `parser.Context`

`md` is a package-level singleton, so it cannot hold anything about the document
being rendered. `baseDir` reaches the wikilink parser via `parser.NewContext()`
and `parser.WithContext` in `RenderWithTheme`. Anything else a parser needs to
know per document goes the same way.
