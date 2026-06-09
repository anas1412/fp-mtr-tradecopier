#!/bin/bash
set -e

echo "==> Installing FundingPips Trade Copier"
echo ""

# Check Go is installed
if ! command -v go &>/dev/null; then
  echo "Error: Go is not installed."
  echo ""
  echo "Install it using your package manager:"
  echo "  macOS:    brew install go"
  echo "  Ubuntu:   sudo apt install golang-go"
  echo "  Arch:     sudo pacman -S go"
  echo "  Fedora:   sudo dnf install golang"
  echo ""
  echo "Or download from: https://go.dev/dl/"
  exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "Go version: $GO_VERSION"

# Fetch dependencies
echo ""
echo "==> Fetching dependencies..."
go mod tidy

# Build
echo ""
echo "==> Building..."
go build -o copier .

echo ""
echo "==> Done! Run with: ./copier"
