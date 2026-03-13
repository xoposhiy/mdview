# Renderer rules

## CSS: always inline in template.html

Do NOT use a separate style.css file or inject CSS via Go template variables.
WebView2 (`SetHtml` / `NavigateToString`) does not load external files —
only the HTML string passed to it is available. There is no base URL, so
`<link>` tags and external file references silently fail.

**Rule:** put all CSS inside a `<style>` block directly in `template.html`.
