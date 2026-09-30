#!/bin/sh
set -eu
cd "$(dirname "$0")/c"

if [ ! -d "../../../../third_party/lwip/src" ]; then
  echo "third_party/lwip is missing. Run: git submodule update --init --recursive" >&2
  exit 1
fi

make -f Makefile clean
make -f Makefile check
make -f smoke.mk smoke
