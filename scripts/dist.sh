#!/usr/bin/env bash
# Build release archives for every supported platform and a SHA-256 manifest.
set -euo pipefail

VERSION="${1:-2.0.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
WORK="$DIST/work"
LDFLAGS="-s -w -X github.com/petealeon/tbible/v2/internal/version.Version=$VERSION"

rm -rf "$DIST"
mkdir -p "$WORK"

build() {
  local goos="$1" goarch="$2" out="$3"
  echo "building $goos/$goarch"
  (cd "$ROOT" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$WORK/$out" ./cmd/tbible)
}

build linux amd64 tbible-linux-amd64
build linux arm64 tbible-linux-arm64
build darwin amd64 tbible-darwin-amd64
build darwin arm64 tbible-darwin-arm64
build windows amd64 tbible-windows-amd64.exe

python3 - "$VERSION" "$DIST" "$WORK" <<'PY'
import hashlib
import os
import sys
import tarfile
import zipfile

version, dist, work = sys.argv[1], sys.argv[2], sys.argv[3]

unix = [
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
]
windows = [("windows", "amd64")]


def executable(info):
    info.mode = 0o755
    return info


assets = []
for goos, goarch in unix:
    binary = os.path.join(work, f"tbible-{goos}-{goarch}")
    asset = f"tbible-{version}-{goos}-{goarch}.tar.gz"
    with tarfile.open(os.path.join(dist, asset), "w:gz") as tf:
        tf.add(binary, arcname="tbible", filter=executable)
    assets.append(asset)

for goos, goarch in windows:
    binary = os.path.join(work, f"tbible-{goos}-{goarch}.exe")
    asset = f"tbible-{version}-{goos}-{goarch}.zip"
    with zipfile.ZipFile(os.path.join(dist, asset), "w", zipfile.ZIP_DEFLATED) as zf:
        zf.write(binary, arcname="tbible.exe")
    assets.append(asset)

manifest = os.path.join(dist, f"tbible-{version}-checksums.txt")
with open(manifest, "w", encoding="utf-8") as fh:
    for asset in sorted(assets):
        digest = hashlib.sha256()
        with open(os.path.join(dist, asset), "rb") as src:
            for chunk in iter(lambda: src.read(1024 * 1024), b""):
                digest.update(chunk)
        fh.write(f"{digest.hexdigest()}  {asset}\n")

print(f"wrote {len(assets)} archives and {os.path.basename(manifest)}")
PY

rm -rf "$WORK"
echo "release artifacts: $DIST"
