#!/bin/bash
# DBF Sync — Build script
# Usage: ./build.sh [version]
# Example: ./build.sh 0.1.0

set -e

VERSION=${1:-"dev"}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date +%Y-%m-%d)

echo "🔨 Building dbf-sync v${VERSION}..."
echo "   Commit: ${COMMIT}"
echo "   Date:   ${BUILD_DATE}"

export PATH=$PATH:/home/vlongo/go/bin
export GOROOT=/home/vlongo/go
export GOPATH=$HOME/go-workspace

go build -ldflags="-s -w \
  -X main.version=${VERSION} \
  -X main.buildDate=${BUILD_DATE} \
  -X main.commit=${COMMIT}" \
  -o dbf-sync .

echo "✅ Built: dbf-sync v${VERSION} ($(ls -lh dbf-sync | awk '{print $5}'))"
echo ""
echo "Test with: ./dbf-sync --version"
