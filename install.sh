#!/bin/bash
set -e

echo "==> Installing FundingPips Trade Copier"
echo ""

# Check Go is installed
if ! command -v go &>/dev/null; then
  echo "Error: Go is not installed."
  echo "Install it with: sudo pacman -S go"
  exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "Go version: $GO_VERSION"

# Download deps
echo ""
echo "==> Fetching dependencies..."
GONOSUMDB="*" GOFLAGS="-mod=mod" go mod tidy

# Build
echo ""
echo "==> Building..."
go build -o copier .

echo ""
echo "==> Done! Run with: ./copier"
