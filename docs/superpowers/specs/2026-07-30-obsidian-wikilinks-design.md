# Obsidian-style wikilinks

## Problem

Documents written in Obsidian link to each other with `[[other-document]]`
rather than `[other](other.md)`. goldmark has no notion of the syntax, so such a
link renders as the literal text `[[other-document]]` — visible, unclickable
noise in the middle of otherwise correct prose.

## Constraint that shapes the design

Obsidian resolves a wikilink against a **vault**: it tries the target relative
to the source file's folder, then relative to the vault root, then searches the
whole tree by basename. mdview has no vault concept — `RenderWithTheme` receives
only `baseDir`, the directory of the file being viewed.

A vault root can nevertheless be discovered without any configuration: the
`.obsidian/` folder *is* the marker. Walking up from the document's directory to
the nearest ancestor containing it recovers exactly the root Obsidian would use.
So a link written from `vault/daily/today.md` as `[[projects/alpha/spec]]`
resolves to `vault/projects/alpha/spec.md`, not to
`vault/daily/projects/alpha/spec.md`.

The second shaping constraint is that the rest of the pipeline already works. If
the extension emits a plain `<a href="…">`, `classifyLink`
(`src/internal/viewer/links.go`) resolves it, `Viewer.open` pushes history and
toasts `"Not found: …"`, and the delegated click handler in `template.html`
intercepts it. **The viewer needs no changes at all.** The design's job is to
produce the right `href` and stop.

## Behaviour

| Markdown | Renders as | Click |
|---|---|---|
| `[[other]]` | `other` | Opens `other.md` inside mdview |
| `[[notes/other]]` | `notes/other` | Opens the file in the subfolder |
| `[[other.md]]` | `other.md` | Opens `other.md`; `.md` is not appended twice |
| `[[other\|the label]]` | `the label` | Opens `other.md` |
| `[[diagram.png]]` | `diagram.png` | Registered application, via the existing `linkExternal` path |
| `[[missing]]` | `missing`, dimmed and dotted | Toast `"Not found: missing.md"` |

Resolved wikilinks deliberately look like every other link in the document.
Only unresolved ones are visually distinct.

## Resolution

For target `T`, base directories are tried in order — the document's directory,
then the vault root — and within each, `base/T` then `base/T.md`. The first
**existing** file wins:

```
dir/T          dir/T.md          root/T          root/T.md
```

The `.md` candidate is skipped when `T` already ends in `.md` or `.markdown`
(case-insensitive). The root candidates are skipped when no vault root was
found, or when the root is the document's own directory.

Trying `T` verbatim before appending `.md` removes any need to guess at
extensions from the name. `[[diagram.png]]` is found as-is on the first
candidate; `[[v1.2 release notes]]` — whose `filepath.Ext` is the meaningless
`.2 release notes` — is found on the second. No extension whitelist to maintain.

When nothing exists, the link is marked broken and its `href` is the *first*
candidate that carried an `.md` extension: `dir/T.md`, or `dir/T` when `T`
already had the extension. That is the most likely intent, and it makes the
toast name the file the author meant.

`vaultRoot(dir)` walks from `dir` up through its ancestors, returning the first
that contains a `.obsidian` **directory**. It stops when `filepath.Dir(d) == d`
— the volume root — and returns `""` when there is no marker anywhere above.
With no vault, resolution therefore reduces to "the document's own directory",
which is how ordinary Markdown links in mdview already behave.

### Fragments

`[[other#heading]]` strips `#heading` before resolution and keeps it in the
`href`. Jumping to the heading is out of scope, but the two lines that do this
are what keep such a link from rendering as falsely broken: `classifyLink`
already truncates an href at `#`, so the document simply opens at the top.

## Components

### `src/internal/renderer/wikilink.go` (new)

A goldmark extension built on the same three pieces as `math.go` — parser, node,
node renderer:

```go
type wikilinkNode struct {
    gast.BaseInline
    Label  string
    Href   string
    Broken bool
}

var Wikilink = &wikilinkExtension{}
```

The inline parser triggers on `[` and registers at **priority 150**. This
placement is load-bearing: goldmark's own link parser sits at 200
(`vendor/github.com/yuin/goldmark/parser/parser.go:607`), so a lower number is
required to see `[[` first. Code spans are at 100 and keep their precedence,
which is what makes `` `[[foo]]` `` stay literal for free.

Parsing rules:

- Requires `[[`, a closing `]]` on the same line, and non-empty content between
  them. Anything else returns `nil` and the text stays literal — this covers
  `[[]]`, `[[ ]]`, and an unclosed `[[foo`.
- Splits the alias on the **first** `|`: `[[a|b|c]]` has target `a` and label
  `b|c`, matching Obsidian.
- The label is plain text. Inline Markdown inside it is not parsed, as in
  Obsidian.

Resolution happens at parse time, where `parser.Context` is available.

### Resolution functions, same file

```go
func resolveWikilink(target, baseDir string) (href string, ok bool)
func vaultRoot(dir string) string
```

Free functions rather than an injected interface: there are two of them, both
are pure with respect to their arguments, and both are directly testable against
a `t.TempDir()`. A `LinkResolver` interface would buy nothing here.

### `src/internal/renderer/renderer.go` (modified)

`Wikilink` joins `extension.GFM` and `Math` in the `goldmark.New` extension
list.

`baseDir` reaches the parser through `parser.Context`, since the `md` instance is
a package-level singleton and node renderers get no context of their own:

```go
ctx := parser.NewContext()
ctx.Set(baseDirKey, baseDir)
md.Convert(markdown, &body, parser.WithContext(ctx))
```

When `baseDir` is empty — `Render()` and callers that pass no directory — the
parser touches the filesystem not at all, emits `href = "T.md"`, and marks
nothing broken. Marking every link broken in that case would produce false
dimming in the one situation where existence genuinely cannot be checked.

### `src/internal/renderer/assets/template.html` (modified)

One CSS rule, inline in the `<style>` block per `src/internal/renderer/AGENTS.md`:

```css
a.wikilink.broken { color: var(--fg); opacity: .55; text-decoration-style: dotted; }
```

Written against `var(--fg)`, which every one of the seven `body[data-theme=…]`
blocks already defines, so no theme block is touched. Resolved wikilinks fall
through to the existing `a { color: var(--link) }` at `template.html:183`.

No JavaScript changes: the delegated `click` handler already matches every
`a[href]`.

## Emitted HTML

```html
<a class="wikilink" href="C:\notes\other-document.md">other-document</a>
<a class="wikilink broken" href="C:\notes\missing.md">missing</a>
```

The `href` is an absolute path with backslashes. Nothing navigates to it — the
click handler calls `preventDefault` and passes the raw attribute to Go — and
`classifyLink` handles absolute paths directly. A Windows path is not mistaken
for a URL scheme because `schemeRe` requires a scheme of two or more characters,
the guard documented at `src/internal/viewer/links.go:18`.

Label and `href` are escaped with `template.HTMLEscape`, following `math.go`.

## Error handling

| Situation | Result |
|---|---|
| Target resolves to no existing file | Dimmed dotted link; click toasts `"Not found: T.md"` via the existing `Viewer.open` |
| `[[]]`, `[[ ]]`, unclosed `[[foo` | Left as literal text, no link |
| `[[foo]]` inside a code span or fenced block | Left as literal text |
| `baseDir` empty | Relative `href`, never marked broken |

## Testing

`src/internal/renderer/wikilink_test.go`, headless, using `t.TempDir()` with
real files and a real `.obsidian/` directory:

- Basic `[[note]]`, alias `[[note|label]]`, explicit subfolder `[[sub/note]]`.
- Vault-root path from a nested document: `.obsidian/` at the root, document in
  `daily/`, link `[[projects/spec]]`.
- Nearest-wins: the same basename present both beside the document and at the
  vault root resolves to the one beside the document.
- Broken link carries `class="wikilink broken"` and an `.md` href.
- `[[note.md]]` does not become `note.md.md`.
- `[[diagram.png]]` resolves to the png, not to `diagram.png.md`.
- `[[v1.2 release notes]]` resolves via the `.md` candidate.
- `[[note#heading]]` resolves the file and keeps the fragment in the href.
- Literal cases: backticked, fenced, `[[]]`, `[[ ]]`, unclosed.
- Escaping: a label containing `<` and `&`.
- `vaultRoot` on its own: marker directly above, several levels above, absent
  entirely, and a `.obsidian` *file* rather than a directory (not a root).

## Out of scope

- Embeds: `![[note]]` and `![[image.png]]`.
- Scrolling to the heading in `[[note#heading]]`.
- Recursive basename search across the vault. Most `[[bare-name]]` links in a
  real vault point at notes in *other* folders and will render as broken until
  this is added; the resolution chain is the single place that would change.
- Creating a missing note on click, and link autocomplete. mdview is a viewer.
