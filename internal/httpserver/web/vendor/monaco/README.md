# Monaco Editor (Vendored)

- **Version:** `0.52.2`
- **Source:** npm package `monaco-editor@0.52.2` (extracted from `min/vs`)
- **License:** MIT (see `LICENSE`)
- **Source maps:** Removed (`*.map` excluded)

## How to update

To update or re-vendor this package:

```sh
TMPDIR=$(mktemp -d)
cd "$TMPDIR"
npm pack monaco-editor@<version>
tar -xzf monaco-editor-<version>.tgz
mkdir -p <repo>/internal/httpserver/web/vendor/monaco
cp -r package/min/vs <repo>/internal/httpserver/web/vendor/monaco/
cp package/LICENSE <repo>/internal/httpserver/web/vendor/monaco/
find <repo>/internal/httpserver/web/vendor/monaco/ -name "*.map" -delete
rm -rf "$TMPDIR"
```
