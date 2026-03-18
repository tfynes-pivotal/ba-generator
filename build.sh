#!/usr/bin/env bash
set -euo pipefail

APP=ba-generator
OUT=dist

mkdir -p "$OUT"

echo "Building $APP for macOS (arm64)..."
GOOS=darwin  GOARCH=arm64  go build -o "$OUT/${APP}-darwin-arm64"  .

echo "Building $APP for macOS (amd64)..."
GOOS=darwin  GOARCH=amd64  go build -o "$OUT/${APP}-darwin-amd64"  .

echo "Building $APP for Linux (amd64)..."
GOOS=linux   GOARCH=amd64  go build -o "$OUT/${APP}-linux-amd64"   .

echo "Building $APP for Linux (arm64)..."
GOOS=linux   GOARCH=arm64  go build -o "$OUT/${APP}-linux-arm64"   .

echo "Building $APP for Windows (amd64)..."
GOOS=windows GOARCH=amd64  go build -o "$OUT/${APP}-windows-amd64.exe" .

echo ""
echo "Artifacts in $OUT/:"
ls -lh "$OUT/"
