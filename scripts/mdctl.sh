#!/usr/bin/env bash
# mdctl — drive a running mdview through its control interface.
#
# Start mdview with the interface on:
#   ./mdview.exe -control-port=0 file.md &
# then:
#   scripts/mdctl.sh state
#   scripts/mdctl.sh key right
#   scripts/mdctl.sh shot /tmp/window.png
#
# The port is read from the file mdview writes at startup, so no port needs
# to be passed around.
set -u

port_file="${TEMP:-/tmp}/mdview-control.port"
port_file="${port_file//\\//}"

if [ ! -f "$port_file" ]; then
  echo "mdctl: no running mdview: $port_file not found" >&2
  exit 1
fi
base="http://127.0.0.1:$(cat "$port_file")"

get()  { curl -sS --max-time 30 "$base$1"; }
post() { curl -sS --max-time 30 -X POST "$base$1"; }

cmd="${1:-help}"
shift || true

case "$cmd" in
  state)   get /state ;;
  metrics) get /metrics ;;

  # key <name> [mods]   e.g. key right | key r ctrl | key left alt
  key)     post "/key?k=${1:?key name}&mods=${2:-}" ;;

  # mouse <action> [x] [y] [dx] [dy] [delta]
  mouse)   post "/mouse?a=${1:?action}&x=${2:-0}&y=${3:-0}&dx=${4:-0}&dy=${5:-0}&delta=${6:-120}" ;;
  wheel)   post "/mouse?a=wheel&x=${2:-400}&y=${3:-300}&delta=${1:?delta}" ;;
  drag)    post "/mouse?a=drag&x=${1:?x}&y=${2:?y}&dx=${3:?dx}&dy=${4:?dy}" ;;

  # eval <javascript>   returns the value, JSON-encoded
  eval)    curl -sS --max-time 30 -X POST --data-binary "${1:?js}" "$base/eval" ;;

  open)    post "/open?path=$(printf %s "${1:?path}" | sed 's/ /%20/g')" ;;
  resize)  post "/resize?w=${1:?width}&h=${2:?height}" ;;
  focus)   post /focus ;;

  # shot <path> [client]
  shot)    get "/screenshot?path=${1:?path}&client=${2:-0}" ;;

  quit)    post /quit ;;

  *)
    sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
    exit 1 ;;
esac
echo
