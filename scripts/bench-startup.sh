#!/usr/bin/env bash
# bench-startup.sh <exe> <file.md> [runs]
#
# Measures how mdview behaves with a document on screen: how long it takes to
# show it, how fast the page repaints while scrolling, and how long a window
# resize takes. Each run launches its own mdview and closes it again.
#
# The port comes from the launched process's own stderr, not from the shared
# port file, so overlapping runs cannot read each other's port.
set -u

# Keep the run's theme/font/background changes out of the user's own
# preferences file.
export MDVIEW_CONFIG_DIR="${MDVIEW_CONFIG_DIR:-$(mktemp -d)}"

exe="${1:?usage: bench-startup.sh <exe> <file.md> [runs]}"
doc="${2:?missing document}"
runs="${3:-3}"

# fps measures how many frames the page paints in one second while it is
# scrolled every frame — the page's own view of "does this feel slow".
fps_js='(function(){return new Promise(function(res){var n=0,t0=performance.now();
function step(){n++;window.scrollBy(0,7);
if(performance.now()-t0<1000){requestAnimationFrame(step);}else{window.scrollTo(0,0);res(Math.round(n*1000/(performance.now()-t0)));}}
requestAnimationFrame(step);})})()'

loaded_js='(function(){var n=performance.getEntriesByType("navigation")[0];return (n&&n.loadEventEnd>0)?n.loadEventEnd:null})()'
page_js='(function(){var n=performance.getEntriesByType("navigation")[0];return {load:Math.round(n.loadEventEnd),dpr:devicePixelRatio,w:innerWidth,h:innerHeight}})()'

for run in $(seq 1 "$runs"); do
  err=$(mktemp)
  start=$(date +%s%N)
  "$exe" -control-port=0 "$doc" >/dev/null 2>"$err" &
  pid=$!

  base=""
  for _ in $(seq 1 600); do
    port=$(grep -o '127\.0\.0\.1:[0-9]*' "$err" 2>/dev/null | head -1 | cut -d: -f2)
    if [ -n "$port" ] && curl -sS --max-time 2 "http://127.0.0.1:$port/state" >/dev/null 2>&1; then
      base="http://127.0.0.1:$port"
      break
    fi
  done
  if [ -z "$base" ]; then
    echo "run $run: mdview did not start" >&2
    kill "$pid" 2>/dev/null
    continue
  fi

  ev() { curl -sS --max-time 30 -X POST --data-binary "$1" "$base/eval"; }
  num() { echo "$1" | tr -d ' \n' | grep -o "\"$2\":[0-9.]*" | head -1 | cut -d: -f2; }

  # Wait for the page to finish loading before measuring anything else.
  for _ in $(seq 1 600); do
    [ -n "$(num "$(ev "$loaded_js")" data)" ] && break
  done
  end=$(date +%s%N)

  page=$(ev "$page_js")
  fps=$(num "$(ev "$fps_js")" data)

  # Resize throughput: 10 alternating window sizes, wall-clock total.
  rs=$(date +%s%N)
  for i in $(seq 1 5); do
    curl -sS --max-time 10 -X POST "$base/resize?w=1100&h=800" >/dev/null
    curl -sS --max-time 10 -X POST "$base/resize?w=900&h=700" >/dev/null
  done
  re=$(date +%s%N)

  m=$(curl -sS --max-time 5 "$base/metrics")
  stage() { echo "$m" | tr -d ' \n' | grep -o "\"$1\":{[^}]*}" | grep -o '"lastMs":[0-9.]*' | cut -d: -f2; }

  printf 'run %s  toReady=%sms  read=%s render=%s write=%s  page.load=%s scrollFps=%s resize10=%sms  dpr=%s viewport=%sx%s\n' \
    "$run" "$(( (end-start)/1000000 ))" \
    "$(stage readFile)" "$(stage render)" "$(stage writeHTML)" \
    "$(num "$page" load)" "${fps:-?}" "$(( (re-rs)/1000000 ))" \
    "$(num "$page" dpr)" "$(num "$page" w)" "$(num "$page" h)"

  curl -sS --max-time 5 -X POST "$base/quit" >/dev/null 2>&1
  for _ in $(seq 1 50); do
    kill -0 "$pid" 2>/dev/null || break
  done
  kill "$pid" 2>/dev/null
  rm -f "$err"
done
