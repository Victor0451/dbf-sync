#!/bin/sh
# Updates the Homebrew tap formula with the latest release SHA256.
# Usage: ./scripts/update-brew.sh [version]
#   version defaults to latest GitHub release tag
#
# Requirements: curl, sha256sum (or shasum on macOS), git
# The tap repo must be cloned alongside this repo:
#   ~/git/homebrew-tap  OR  passed as TAP_DIR env var

set -e

REPO="Victor0451/dbf-sync"
TAP_DIR="${TAP_DIR:-${HOME}/git/homebrew-tap}"
FORMULA="${TAP_DIR}/Formula/dbf-sync.rb"

# --- resolve version ---
if [ -n "$1" ]; then
  VERSION="$1"
else
  echo "  Obteniendo ultima version de GitHub..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')
fi

if [ -z "$VERSION" ]; then
  echo "ERROR: no se pudo obtener la version." >&2
  exit 1
fi

echo "  Version : $VERSION"

# --- download binary and compute SHA256 ---
BINARY_URL="https://github.com/${REPO}/releases/download/${VERSION}/dbf-sync-linux-amd64"
TMP=$(mktemp)
echo "  Descargando binario para calcular SHA256..."
curl -fsSL "$BINARY_URL" -o "$TMP"

SHA256=$(sha256sum "$TMP" 2>/dev/null | awk '{print $1}' \
  || shasum -a 256 "$TMP" | awk '{print $1}')
rm -f "$TMP"

echo "  SHA256  : $SHA256"

# --- update formula ---
if [ ! -f "$FORMULA" ]; then
  echo "ERROR: formula no encontrada en $FORMULA" >&2
  echo "  Seteá TAP_DIR apuntando al repo homebrew-tap clonado." >&2
  exit 1
fi

# Replace version and sha256 in formula (two occurrences of each for linux+macos blocks)
sed -i \
  -e "s|releases/download/v[0-9.]*/dbf-sync-linux-amd64|releases/download/${VERSION}/dbf-sync-linux-amd64|g" \
  -e "s/sha256 \"[a-f0-9]*\"/sha256 \"${SHA256}\"/g" \
  -e "s/version \"[0-9.]*\"/version \"${VERSION#v}\"/" \
  "$FORMULA"

echo "  Formula actualizada: $FORMULA"

# --- commit and push tap ---
cd "$TAP_DIR"
git add Formula/dbf-sync.rb
git commit -m "chore: bump dbf-sync to ${VERSION}"
git push origin main

echo ""
echo "  ✓ Homebrew tap actualizado a ${VERSION}"
echo "  Usuarios pueden hacer: brew update && brew upgrade dbf-sync"
