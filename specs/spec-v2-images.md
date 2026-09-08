# mdview v2 — Image Viewing Specification

## Scope

Extend mdview to view raster images (PNG, JPEG, BMP, GIF — first frame only,
no animation) alongside Markdown files, while keeping the behavioral
features that already work for Markdown: arrow-key navigation between files
in the current directory, Back/history, live reload on external file
changes, and most existing hotkeys.

## Architecture

- One top-level window, as today. A native Win32 child control (GDI,
  `syscall`-based, no cgo — matching the existing `viewer.go` style) is
  created as a sibling of the WebView2 content and shown/hidden depending on
  whether the current file is Markdown or an image. Switching between file
  types never creates or destroys the window: title, position and size
  persist.
- The parent window's `WndProc` is subclassed at runtime (`SetWindowLongPtrW`
  with `GWLP_WNDPROC`, forwarding everything to the original proc via
  `CallWindowProcW`) purely to relay `WM_SIZE` into a resize of the image
  control — the vendored `go-webview2` package is never modified.
- Decoding: `image/png`, `image/jpeg`, `image/gif` (stdlib) and
  `golang.org/x/image/bmp`, dispatched by `image.Decode`'s own magic-byte
  detection (package `internal/imgdecode`).
- Rendering: the decoded image is composited once over the current
  letterbox background color into a top-down 32bpp BGRA buffer, then blitted
  every repaint with `StretchDIBits` (`HALFTONE` stretch mode for quality
  scaling). No `HBITMAP`/`CreateDIBSection` lifecycle to manage.
- DPI: the process declares Per-Monitor-V2 DPI awareness
  (`SetProcessDpiAwarenessContext`) at startup, so `GetClientRect` returns
  physical pixels and 100% zoom means one image pixel per screen pixel on
  HiDPI monitors. Because every Win32 size is then in physical pixels, the
  window's opening size is scaled by the monitor's DPI (`scaleForDPI`);
  without that the window would open two thirds of its former size on a
  display scaled to 150%, and Markdown documents would get a much smaller
  viewport than before.
- The image control is created the first time an image is displayed, not at
  startup. A session that only shows Markdown then creates no extra window
  and, more importantly, does not subclass the parent `WndProc`.

## Navigation

- Arrow-key (and Backspace/Alt+Left Back) navigation walks **one combined,
  alphabetically sorted list** of Markdown and image files in the current
  directory — not two separate lists.
- A Markdown link to a supported image (e.g. `[fig](chart.png)`) now opens
  in mdview itself, pushing history, instead of shelling out to the OS's
  default image viewer.

## Zoom and pan

- Default is Fit: the image is scaled down proportionally if it doesn't fit
  the window, but never scaled up past 100% automatically.
- Resizing the window recomputes Fit — *unless* the user has manually
  zoomed with the mouse wheel, in which case the chosen scale sticks across
  resizes (Windows Photos-style) until reset.
- Mouse wheel zooms in/out, centered on the cursor position. Range ~10%–
  3200%, ~10% per wheel notch (fractional for high-resolution wheels).
- Left-click-and-drag pans when the image is larger than the viewport.
- Middle-click (mouse wheel button) resets back to Fit.
- The current zoom level is shown in the window title, e.g.
  `chart.png — mdview (137%)`.

### How a frame is drawn

- The frame is built in an off-screen bitmap and copied to the window in one
  BitBlt. Drawing straight to the window showed the background for as long as
  the scaling took, which is the black flash while zooming.
- Only the part of the image the window can show is handed to GDI. At 800% the
  full destination rectangle is tens of thousands of pixels wide, and scaling
  all of it just to clip it away took ~10 ms per frame; clipping first takes
  ~2.5 ms.
- Downscaling uses HALFTONE, which averages the pixels it drops. It reads
  every source pixel and costs ~50 ms on a 12-megapixel image, so while the
  user is zooming or panning the cheap mode is used instead, and a HALFTONE
  repaint follows 90 ms after the gesture stops. Upscaling always uses the
  cheap mode: at over 100% it shows the real pixels instead of blurring them.

## Background / transparency

- A simple black/white letterbox background (default: black), toggled with
  `D` — not the 9 Markdown CSS themes, which don't apply to images.
- PNG alpha transparency is composited onto this background; no
  checkerboard pattern.

## Saving and showing the file

- `S` and `Ctrl+S` open the standard Save-as dialog, starting in the image's
  own folder with its own name. Saving copies the file byte for byte: the
  same format and quality, no re-encoding, and zoom/pan/background do not
  change what is written.
- `O` opens Explorer on the image's folder with the image selected.
- The Markdown view has the same two keys: `Ctrl+S` (save as HTML) now asks
  where the file goes instead of writing next to the source, and `O` shows
  the Markdown file in Explorer. `S` stays the appearance panel there.

## Clipboard

- `Ctrl+C` copies the whole original image (natural resolution, ignoring
  current zoom/pan) to the clipboard as a standard bitmap (`CF_DIB`).

## Keyboard shortcuts while viewing an image

| Key | Behavior |
|---|---|
| ← / → | Previous / next file (combined Markdown + image list) |
| Backspace, Alt+← | Back |
| Escape | Quit |
| F5, Ctrl+R | Reload from disk |
| E | Open in editor (same as Markdown) |
| D | Toggle black/white letterbox background |
| Ctrl+C | Copy image to clipboard |
| S, Ctrl+S | Save a copy of the image (a Save-as dialog asks where) |
| O | Show the image in Explorer, selected in its folder |
| W | No-op (Markdown-only: wide layout) |

The image control takes the keyboard focus when it is shown, when it is
clicked, and every time the window is activated again. Without the last one,
WebView2 takes the focus back into its own child window on every alt-tab,
click-from-another-app, or closing dialog, and the shortcuts above would go to
the hidden page instead of the image.

Native keyboard/mouse handling for the image control calls the same Go
methods (`navigate`, `back`, `reload`, ...) that Markdown's JS hotkeys call
through `webview.Bind` — bypassing the WebView entirely, since it may be
hidden behind the image control. A navigation failure (e.g. flipping to a
corrupt file) shows a transient message drawn directly in the image control,
mirroring the Markdown view's HTML toast for the one case where that toast
would otherwise be invisible.

## Out of scope for v2

- Animated GIF playback (only the first frame is shown)
- WEBP, TIFF, and other formats not already free via the stdlib + one
  `x/image` package
- A visible zoom-percentage HUD over the image (title bar suffix only)
- Multi-monitor DPI-change re-layout (`WM_DPICHANGED`)
