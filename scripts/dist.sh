#!/usr/bin/env bash
# Build release archives for every supported platform, data-only translation
# packs, offline bundles, and a SHA-256 manifest.
set -euo pipefail

VERSION="${1:-2.1.1}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
WORK="$DIST/work"
LDFLAGS="-s -w -X github.com/petealeon/tbible/v2/internal/version.Version=$VERSION"

rm -rf "$DIST"
mkdir -p "$WORK"

echo "building translation packs"
(cd "$ROOT" && go run ./cmd/tbpack)
cp "$ROOT/internal/catalog/data/catalog.json" "$DIST/packs/catalog.json"

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
import glob
import hashlib
import os
import sys
import tarfile
import zipfile

version, dist, work = sys.argv[1], sys.argv[2], sys.argv[3]
pack_dir = os.path.join(dist, "packs")

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


def digest(path):
    h = hashlib.sha256()
    with open(path, "rb") as src:
        for chunk in iter(lambda: src.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


assets = []
for goos, goarch in unix:
    binary = os.path.join(work, f"tbible-{goos}-{goarch}")

    asset = f"tbible-{version}-{goos}-{goarch}.tar.gz"
    with tarfile.open(os.path.join(dist, asset), "w:gz") as tf:
        tf.add(binary, arcname="tbible", filter=executable)
    assets.append(asset)

    offline = f"tbible-{version}-{goos}-{goarch}-offline.tar.gz"
    with tarfile.open(os.path.join(dist, offline), "w:gz") as tf:
        tf.add(binary, arcname="tbible", filter=executable)
        tf.add(os.path.join(pack_dir, "catalog.json"), arcname="catalog.json")
        for path in sorted(glob.glob(os.path.join(pack_dir, "*.tbp"))):
            tf.add(path, arcname="packs/" + os.path.basename(path))
    assets.append(offline)

for goos, goarch in windows:
    binary = os.path.join(work, f"tbible-{goos}-{goarch}.exe")

    asset = f"tbible-{version}-{goos}-{goarch}.zip"
    with zipfile.ZipFile(os.path.join(dist, asset), "w", zipfile.ZIP_DEFLATED) as zf:
        zf.write(binary, arcname="tbible.exe")
    assets.append(asset)

    offline = f"tbible-{version}-{goos}-{goarch}-offline.zip"
    with zipfile.ZipFile(os.path.join(dist, offline), "w", zipfile.ZIP_DEFLATED) as zf:
        zf.write(binary, arcname="tbible.exe")
        zf.write(os.path.join(pack_dir, "catalog.json"), arcname="catalog.json")
        for path in sorted(glob.glob(os.path.join(pack_dir, "*.tbp"))):
            zf.write(path, arcname="packs/" + os.path.basename(path))
    assets.append(offline)

# Standalone pack assets are published under the packs-v1 release tag. Their
# checksums are also recorded here so a release can be verified in one place.
for path in sorted(glob.glob(os.path.join(pack_dir, "*.tbp"))):
    assets.append(os.path.relpath(path, dist))
assets.append(os.path.relpath(os.path.join(pack_dir, "catalog.json"), dist))

manifest = os.path.join(dist, f"tbible-{version}-checksums.txt")
with open(manifest, "w", encoding="utf-8") as fh:
    for asset in sorted(assets):
        fh.write(f"{digest(os.path.join(dist, asset))}  {asset}\n")

print(f"wrote {len(assets)} assets and {os.path.basename(manifest)}")
PY

rm -rf "$WORK"
echo "release artifacts: $DIST"
