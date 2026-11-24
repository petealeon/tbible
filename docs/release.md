# Release process

## Versioning and layout

- The current source tree is v2. The Go module remains
  `github.com/petealeon/tbible/v2`.
- v1 is preserved in `legacy/v1/` and on the `v1` branch with tag `v1.0.0`.
- Application releases use tags `vX.Y.Z`. v2.1 is tagged `v2.1.1`.
- Translation packs are versioned independently under the `packs-v1` tag. Pack
  IDs and URLs are recorded in the committed catalog.

## Before a release

1. Update `Makefile VERSION`, `scripts/dist.sh`, `install.sh`,
   `scripts/install.ps1`, `internal/version.Version`, CI, and the README.
2. Refresh screenshots if the layout changed.
3. Run the full checks:

   ```sh
   gofmt -l .
   go vet ./...
   go test ./...
   go build ./...
   go run ./cmd/tbgen --verify
   ```

4. If source data changed, regenerate and verify the embedded artefacts and
   packs:

   ```sh
   make gen
   make verify
   make packs
   ```

   The generated catalog is committed. Its hashes must match the `.tbp` files.

## Building assets

```sh
make dist VERSION=2.1.1
cd dist
sha256sum -c tbible-2.1.1-checksums.txt
```

`scripts/dist.sh` produces:

- one standard archive per OS/architecture with only the executable;
- one `*-offline.tar.gz` or `*-offline.zip` per platform containing the
  executable, catalog, and all four translation packs;
- the standalone `.tbp` files and `catalog.json` for the `packs-v1` release;
- `tbible-2.1.1-checksums.txt` covering every asset.

All release builds set `CGO_ENABLED=0`, use `-trimpath`, and inject the version
with `-ldflags`.

## Publishing

Publish the application release first so installer URLs resolve:

```sh
gh release create v2.1.1 \
  dist/tbible-2.1.1-* \
  dist/tbible-2.1.1-checksums.txt \
  --title "tbible 2.1.1" \
  --notes-file RELEASE_NOTES.md
```

Then publish the pack release using the exact catalog URLs:

```sh
gh release create packs-v1 \
  dist/packs/asv.tbp dist/packs/web.tbp dist/packs/ylt.tbp dist/packs/wmb.tbp \
  dist/packs/catalog.json \
  --title "tbible translation packs v1" \
  --notes "Data-only public-domain translation packs. Hashes are in catalog.json."
```

Verify that each catalog `url` returns the file whose SHA-256 matches its
`sha256` field. The installers download only the standard binary archive; the
offline bundles are manual downloads described in the README.

## Post-release

- Confirm the CI checksum job passes for the tagged version.
- Confirm `--list-translations` works from a clean install.
- Confirm a fresh profile can install one pack, set it primary, and add a
  parallel view.
- Confirm `--ephemeral` refuses downloads and writes no state file.
- Do not add network workarounds to user docs. Restricted environments should
  use the offline bundle.
