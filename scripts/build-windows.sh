#!/usr/bin/env bash
# Cross-compiles the real Windows build, injecting the session-encryption
# key and the download token at build time (neither is ever written to a
# tracked file — see internal/sessionstore/crypt.go and
# internal/config/config.go for why). Needs mingw-w64-gcc on this dev
# machine (Wails' Windows webview binding uses cgo); the resulting exe has
# no such dependency on the target machine.
set -euo pipefail
cd "$(dirname "$0")/.."

KEY_FILE="internal/config/buildkey.txt"
if [[ ! -f "$KEY_FILE" ]]; then
  echo "Missing $KEY_FILE — generate one with: openssl rand -hex 32 > $KEY_FILE" >&2
  exit 1
fi
BUILD_KEY="$(tr -d '[:space:]' < "$KEY_FILE")"

TOKEN_FILE="internal/config/downloadtoken.txt"
if [[ ! -f "$TOKEN_FILE" ]]; then
  echo "Missing $TOKEN_FILE — generate one with: openssl rand -hex 128 > $TOKEN_FILE" >&2
  exit 1
fi
DOWNLOAD_TOKEN="$(tr -d '[:space:]' < "$TOKEN_FILE")"

# APP_VERSION is optional for local/manual builds ("dev" if unset); the
# publish pipeline that calls this script always sets it.
APP_VERSION="${APP_VERSION:-dev}"

WAILS="$(command -v wails || echo "$HOME/go/bin/wails")"

# All -X vars must go in a single -ldflags string — repeating the flag on
# the command line means only the last one wins.
"$WAILS" build -platform windows/amd64 \
  -ldflags "-X liforra-tool/internal/sessionstore.buildKey=${BUILD_KEY} -X liforra-tool/internal/config.DownloadToken=${DOWNLOAD_TOKEN} -X liforra-tool/internal/config.AppVersion=${APP_VERSION}" \
  "$@"
