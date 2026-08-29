#!/usr/bin/env bash
# check-images.sh [outdir]
#
# Drives one mdview through the image view and reports what it did: opening a
# link to an image, zoom, pan, background toggle, arrow-key navigation between
# a Markdown file and images, and Back. Saves a screenshot at each step.
#
# Run from the repository root. Needs mdview.exe built and testdata present.
set -u

# Keep the run's theme/font/background changes out of the user's own
# preferences file.
export MDVIEW_CONFIG_DIR="${MDVIEW_CONFIG_DIR:-$(mktemp -d)}"

out="${1:-/c/tmp/mdview-check}"
mkdir -p "$out"
outwin=$(cygpath -w "$out" 2>/dev/null || echo "$out")

err=$(mktemp)
./mdview.exe -control-port=0 testdata/images.md >/dev/null 2>"$err" &
pid=$!

base=""
for _ in $(seq 1 600); do
  port=$(grep -o '127\.0\.0\.1:[0-9]*' "$err" 2>/dev/null | head -1 | cut -d: -f2)
  if [ -n "$port" ] && curl -sS --max-time 2 "http://127.0.0.1:$port/state" >/dev/null 2>&1; then
    base="http://127.0.0.1:$port"; break
  fi
done
[ -z "$base" ] && { echo "mdview did not start" >&2; exit 1; }

get()  { curl -sS --max-time 30 "$base$1"; }
post() { curl -sS --max-time 30 -X POST "$base$1"; }
brief() { tr -d ' \n' | grep -o '"file":"[^"]*"\|"mode":"[^"]*"\|"zoomPercent":[0-9]*\|"fit":[a-z]*\|"bg":"[^"]*"\|"panX":[-0-9.]*\|"title":"[^"]*"' | tr '\n' ' '; echo; }
step() { echo; echo "--- $1"; }
shot() { get "/screenshot?path=$outwin\\$1.png" >/dev/null; }

step "markdown document"
get /state | brief
shot 01-markdown

step "open a link to an image (grid 1200x900)"
post "/open?path=testdata/grid-1200x900.png" | tr -d ' \n'; echo
get /state | brief
shot 02-image-fit

step "wheel zoom in x5 at (400,300)"
for i in 1 2 3 4 5; do post "/mouse?a=wheel&x=400&y=300&delta=120" >/dev/null; done
get /state | brief
shot 03-zoomed

step "drag to pan by (-200,-150)"
post "/mouse?a=drag&x=500&y=400&dx=-200&dy=-150" >/dev/null
get /state | brief
shot 04-panned

step "middle click resets to fit"
post "/mouse?a=middle" >/dev/null
get /state | brief

step "D toggles the letterbox background"
post "/key?k=d" >/dev/null
get /state | brief
shot 05-white-bg
post "/key?k=d" >/dev/null

step "right arrow: next file in the directory"
post "/key?k=right" >/dev/null
get /state | brief
shot 06-next

step "right arrow again"
post "/key?k=right" >/dev/null
get /state | brief
shot 07-next2

step "backspace: back"
post "/key?k=backspace" >/dev/null
get /state | brief

step "open the transparent PNG, check both backgrounds"
post "/open?path=testdata/alpha-400x300.png" >/dev/null
get /state | brief
shot 08-alpha-black
post "/key?k=d" >/dev/null
shot 09-alpha-white

step "metrics"
get /metrics | tr -d ' \n' | sed 's/},/},\n/g'

post /quit >/dev/null 2>&1
for _ in $(seq 1 50); do kill -0 "$pid" 2>/dev/null || break; done
kill "$pid" 2>/dev/null
rm -f "$err"
echo
echo "screenshots in $out"
