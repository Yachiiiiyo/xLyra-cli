#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION="${VERSION:-$(git describe --tags --exact-match 2>/dev/null || echo "dev")}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
BUILD_TIME="${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
PROTOCOL_VERSION="$(awk -F= '/const ProtocolVersion/ { gsub(/[^0-9]/, "", $2); print $2; exit }' internal/version/version.go)"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-1}"

OUT_DIR="${OUT_DIR:-$ROOT_DIR/dist}"
TARGETS=("linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64")

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "error: neither sha256sum nor shasum found" >&2
    exit 1
  fi
}

file_size() {
  if stat -c%s "$1" >/dev/null 2>&1; then
    stat -c%s "$1"
  else
    stat -f%z "$1"
  fi
}

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

LDFLAGS="-s -w -X 'github.com/Yachiiiiyo/xLyra-cli/internal/version.Version=${VERSION}' -X 'github.com/Yachiiiiyo/xLyra-cli/internal/version.Commit=${COMMIT}' -X 'github.com/Yachiiiiyo/xLyra-cli/internal/version.BuildTime=${BUILD_TIME}'"

CHECKSUMS_FILE="$OUT_DIR/checksums.txt"
MANIFEST_FILE="$OUT_DIR/manifest.json"
: > "$CHECKSUMS_FILE"

ARTIFACT_ENTRIES=""

for target in "${TARGETS[@]}"; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  STAGE_DIR="$OUT_DIR/.stage-${GOOS}-${GOARCH}"
  ARCHIVE_NAME="xlyra-cli_${VERSION}_${GOOS}_${GOARCH}.tar.gz"
  ARCHIVE_PATH="$OUT_DIR/$ARCHIVE_NAME"

  mkdir -p "$STAGE_DIR"
  echo "Building ${GOOS}/${GOARCH} (${VERSION})..."
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -trimpath -ldflags="$LDFLAGS" -o "$STAGE_DIR/xlyra-cli" ./cmd/xlyra-cli

  chmod 0755 "$STAGE_DIR/xlyra-cli"
  COPYFILE_DISABLE=1 tar -czf "$ARCHIVE_PATH" -C "$STAGE_DIR" xlyra-cli
  rm -rf "$STAGE_DIR"

  SHA256="$(sha256_file "$ARCHIVE_PATH")"
  SIZE="$(file_size "$ARCHIVE_PATH")"
  printf '%s  %s\n' "$SHA256" "$ARCHIVE_NAME" >> "$CHECKSUMS_FILE"

  ENTRY=$(cat <<EOF
    "${GOOS}/${GOARCH}": {
      "os": "${GOOS}",
      "arch": "${GOARCH}",
      "filename": "${ARCHIVE_NAME}",
      "sha256": "${SHA256}",
      "size": ${SIZE}
    }
EOF
)
  if [ -n "$ARTIFACT_ENTRIES" ]; then
    ARTIFACT_ENTRIES="${ARTIFACT_ENTRIES},
${ENTRY}"
  else
    ARTIFACT_ENTRIES="${ENTRY}"
  fi
done

cat > "$MANIFEST_FILE" <<EOF
{
  "version": "${VERSION}",
  "protocol_version": ${PROTOCOL_VERSION},
  "commit": "${COMMIT}",
  "build_time": "${BUILD_TIME}",
  "artifacts": {
${ARTIFACT_ENTRIES}
  }
}
EOF

echo "Release artifacts written to $OUT_DIR:"
ls -lh "$OUT_DIR"
