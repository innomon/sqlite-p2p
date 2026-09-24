#!/usr/bin/env bash
set -euo pipefail

# ------------------------------------------------------------------------------
# sqlite-p2p End-to-End Replication Test Runner
# Automatically detects OS and Architecture and executes the matching binary.
# Supported targets:
#   - Linux ARM64 (e.g. Raspberry Pi 5, AWS Graviton)
#   - macOS ARM64 (e.g. Apple Silicon M4 / M3 / M2 / M1)
# ------------------------------------------------------------------------------

SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ]; do
  DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
  SOURCE="$(readlink "$SOURCE")"
  [[ $SOURCE != /* ]] && SOURCE="$DIR/$SOURCE"
done
SCRIPT_DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
WORKSPACE_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m | tr '[:upper:]' '[:lower:]')"

TARGET_BIN=""
DEFAULT_CONFIG=""

case "$OS" in
  linux)
    case "$ARCH" in
      aarch64|arm64)
        TARGET_BIN="$WORKSPACE_ROOT/bin/e2e-replication-linux-arm64"
        DEFAULT_CONFIG="$WORKSPACE_ROOT/config/node1.json"
        ;;
      *)
        echo "[ERROR] Unsupported Linux architecture: $ARCH (expected arm64/aarch64)." >&2
        echo "Building native binary with local Go toolchain..."
        mkdir -p "$WORKSPACE_ROOT/bin"
        go build -o "$WORKSPACE_ROOT/bin/e2e-replication-$OS-$ARCH" "$WORKSPACE_ROOT/cmd/e2e-replication"
        TARGET_BIN="$WORKSPACE_ROOT/bin/e2e-replication-$OS-$ARCH"
        DEFAULT_CONFIG="$WORKSPACE_ROOT/config/node1.json"
        ;;
    esac
    ;;
  darwin)
    case "$ARCH" in
      arm64)
        TARGET_BIN="$WORKSPACE_ROOT/bin/e2e-replication-darwin-arm64"
        DEFAULT_CONFIG="$WORKSPACE_ROOT/config/node2.json"
        ;;
      *)
        echo "[ERROR] Unsupported macOS architecture: $ARCH (expected arm64 for Apple Silicon / M4)." >&2
        echo "Building native binary with local Go toolchain..."
        mkdir -p "$WORKSPACE_ROOT/bin"
        go build -o "$WORKSPACE_ROOT/bin/e2e-replication-$OS-$ARCH" "$WORKSPACE_ROOT/cmd/e2e-replication"
        TARGET_BIN="$WORKSPACE_ROOT/bin/e2e-replication-$OS-$ARCH"
        DEFAULT_CONFIG="$WORKSPACE_ROOT/config/node2.json"
        ;;
    esac
    ;;
  *)
    echo "[ERROR] Unsupported operating system: $OS" >&2
    exit 1
    ;;
esac

if [ ! -f "$TARGET_BIN" ]; then
  echo "[WARN] Binary not found at $TARGET_BIN. Attempting to build..."
  mkdir -p "$WORKSPACE_ROOT/bin"
  (cd "$WORKSPACE_ROOT" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "$TARGET_BIN" ./cmd/e2e-replication)
fi

chmod +x "$TARGET_BIN"

echo "================================================================="
echo "  sqlite-p2p E2E Replication Runner"
echo "  OS:           $OS"
echo "  Architecture: $ARCH"
echo "  Executable:   $TARGET_BIN"
echo "================================================================="

# If no arguments provided, default to loading the platform default config if present
if [ $# -eq 0 ]; then
  if [ -f "$DEFAULT_CONFIG" ]; then
    echo "No arguments specified. Using default configuration: $DEFAULT_CONFIG"
    exec "$TARGET_BIN" -config "$DEFAULT_CONFIG"
  else
    exec "$TARGET_BIN"
  fi
else
  exec "$TARGET_BIN" "$@"
fi
