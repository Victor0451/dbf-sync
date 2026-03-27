#!/bin/sh
# DBF-SYNC Installer for Linux
# Usage: curl -fsSL https://raw.githubusercontent.com/Victor0451/dbf-sync/main/install.sh | sh

set -e

REPO="Victor0451/dbf-sync"
APP="dbf-sync"
INSTALL_DIR="${HOME}/.local/bin"

echo ""
echo "  DBF-SYNC Installer"
echo "  ─────────────────────────────────────"
echo ""

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH_SUFFIX="amd64" ;;
  aarch64) ARCH_SUFFIX="arm64" ;;
  *)
    echo "  ERROR: Arquitectura no soportada: $ARCH"
    exit 1
    ;;
esac

# Get latest release version
echo "  Buscando ultima version..."
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')

if [ -z "$VERSION" ]; then
  echo "  ERROR: No se pudo obtener la version desde GitHub."
  exit 1
fi

BINARY="dbf-sync-linux-${ARCH_SUFFIX}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}"

echo "  Version : $VERSION"
echo "  Destino : $INSTALL_DIR/$APP"
echo ""

# Create install dir
mkdir -p "$INSTALL_DIR"

# Download binary
echo "  Descargando $BINARY..."
curl -fsSL "$URL" -o "$INSTALL_DIR/$APP"
chmod +x "$INSTALL_DIR/$APP"

# Install example config if no config exists yet
CONFIG_DIR="${HOME}/.dbf-sync"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
EXAMPLE_URL="https://raw.githubusercontent.com/${REPO}/main/config/config.example.yaml"

if [ ! -f "$CONFIG_FILE" ]; then
  echo "  Instalando config de ejemplo en $CONFIG_FILE..."
  mkdir -p "$CONFIG_DIR"
  curl -fsSL "$EXAMPLE_URL" -o "$CONFIG_FILE"
  echo "  Config instalado. Editalo con tus credenciales antes de usar."
else
  echo "  Config existente encontrado en $CONFIG_FILE — no se sobreescribe."
fi

# Check PATH
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo ""
    echo "  Agrega esto a tu ~/.bashrc o ~/.zshrc o ~/.config/fish/config.fish:"
    echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
    ;;
esac

echo ""
echo "  ─────────────────────────────────────"
echo "  Instalacion completada! v$VERSION"
echo ""
echo "  Proximo paso: edita tu config:"
echo "    nano ${CONFIG_FILE}"
echo ""
echo "  Luego ejecuta:"
echo "    dbf-sync interactive"
echo ""
