#!/usr/bin/env bash
set -euo pipefail

OUT=dist
mkdir -p "$OUT"

# app name -> package path
build_app() {
  local APP=$1 PKG=$2
  echo "Building $APP for macOS (arm64)...";   GOOS=darwin  GOARCH=arm64 go build -o "$OUT/${APP}-darwin-arm64"      "$PKG"
  echo "Building $APP for macOS (amd64)...";   GOOS=darwin  GOARCH=amd64 go build -o "$OUT/${APP}-darwin-amd64"      "$PKG"
  echo "Building $APP for Linux (amd64)...";   GOOS=linux   GOARCH=amd64 go build -o "$OUT/${APP}-linux-amd64"       "$PKG"
  echo "Building $APP for Linux (arm64)...";   GOOS=linux   GOARCH=arm64 go build -o "$OUT/${APP}-linux-arm64"       "$PKG"
  echo "Building $APP for Windows (amd64)..."; GOOS=windows GOARCH=amd64 go build -o "$OUT/${APP}-windows-amd64.exe" "$PKG"
}

build_app ba-generator .
build_app ba-deleter   ./cmd/ba-deleter

echo ""
echo "Artifacts in $OUT/:"
ls -lh "$OUT/"
