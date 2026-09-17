#!/usr/bin/env bash
#
# Build the structure models: run each build_*.py through Blender headless and
# write the glTF into assets/models.
#
#   tools/models/build.sh
#   BLENDER="/path/to/blender" tools/models/build.sh
#
# The scripts are procedural — they build their geometry from primitives rather
# than opening a .blend — so the source of truth for a model is a readable
# Python file in this directory, not a binary nobody can diff. Each one
# validates its own footprint, ground contact, height and triangle budget and
# fails the build rather than exporting something that will not sit on a tile.
#
# The game loads whatever is in assets/models and falls back to the procedural
# Go geometry in internal/meshgen for anything missing, so this can be run for
# one model at a time.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../.." && pwd)"
out="$root/assets/models"

# Blender is not on PATH in a default Windows install, so look where it lands.
if [ -z "${BLENDER:-}" ]; then
  for candidate in \
    "$(command -v blender 2>/dev/null || true)" \
    "/c/Program Files/Blender Foundation/Blender 5.0/blender.exe" \
    "/c/Program Files/Blender Foundation/Blender 4.4/blender.exe" \
    "/c/Program Files/Blender Foundation/Blender 4.2/blender.exe"
  do
    if [ -n "$candidate" ] && [ -x "$candidate" ]; then BLENDER="$candidate"; break; fi
  done
fi
if [ -z "${BLENDER:-}" ]; then
  echo "blender not found; set BLENDER=/path/to/blender" >&2
  exit 1
fi

mkdir -p "$out"

shopt -s nullglob
scripts=("$here"/build_*.py)
if [ ${#scripts[@]} -eq 0 ]; then
  echo "no build_*.py in $here" >&2
  exit 1
fi

for script in "${scripts[@]}"; do
  name="$(basename "$script" .py)"   # build_habitat
  name="${name#build_}"              # habitat

  # The numeric prefix has to match the one the game looks for, which is the
  # structure's position in colony.Buildable.
  case "$name" in
    habitat)    n=1 ;;
    solar)      n=2 ;;
    mine)       n=3 ;;
    extractor)  n=4 ;;
    condenser)  n=5 ;;
    greenhouse) n=6 ;;
    geothermal) n=7 ;;
    battery)    n=8 ;;
    *) echo "no slot number known for '$name'; add one to build.sh" >&2; exit 1 ;;
  esac

  target="$out/$n-$name.glb"
  echo "== $name -> $(basename "$target")"

  # Blender exits 0 even when a --python script raises, so the export is
  # verified by checking the file rather than by trusting the status.
  before=""
  [ -f "$target" ] && before="$(md5sum "$target" | cut -d' ' -f1)"

  "$BLENDER" --background --python "$script" -- "$target" 2>&1 \
    | grep -Ev "^(Blender|Read prefs|ℹ️|INFO:|\s*$)" \
    | sed 's/^/   /' || true

  if [ ! -f "$target" ]; then
    echo "   FAILED: $target was not written" >&2
    exit 1
  fi
  after="$(md5sum "$target" | cut -d' ' -f1)"
  if [ -n "$before" ] && [ "$before" = "$after" ]; then
    echo "   warning: output unchanged; the script may have failed before export" >&2
  fi
done

echo
echo "models in $out:"
ls -la "$out"
