#!/usr/bin/env sh
set -e

REPO="ZatTwilight/flareduct"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY="flareduct"

# Detect OS and architecture.
OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
  Linux) OS="Linux" ;;
  Darwin) OS="Darwin" ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Resolve version (latest or pinned).
VERSION="${VERSION:-latest}"
if [ "$VERSION" = "latest" ]; then
  if [ -n "$GITHUB_TOKEN" ]; then
    TAG=$(curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" "https://api.github.com/repos/${REPO}/releases/latest" | awk -F'"' '/"tag_name":/ {print $4}')
  else
    TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | awk -F'"' '/"tag_name":/ {print $4}')
  fi
  if [ -z "$TAG" ]; then
    echo "Could not determine latest release version"
    exit 1
  fi
  VERSION="${TAG}"
fi

ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Downloading ${BINARY} ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL -o "${TMP_DIR}/${ARCHIVE}" "$URL"

tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "$TMP_DIR"

if [ ! -f "${TMP_DIR}/${BINARY}" ]; then
  echo "Extracted archive did not contain ${BINARY}"
  ls -la "$TMP_DIR"
  exit 1
fi

# Determine install destination.
if [ -w "$INSTALL_DIR" ] || [ "$INSTALL_DIR" = "$HOME/.local/bin" ]; then
  mkdir -p "$INSTALL_DIR"
  mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} requires sudo..."
  sudo mkdir -p "$INSTALL_DIR"
  sudo mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"

echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
"${INSTALL_DIR}/${BINARY}" version
