#!/bin/sh
# Builds the playground engine into public/assets. wasm_exec.js is the glue the
# Go toolchain ships, and it has to come from the same Go that built the .wasm.
set -eu
cd "$(dirname "$0")"
GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o public/assets/engine.wasm ./wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" public/assets/wasm_exec.js
