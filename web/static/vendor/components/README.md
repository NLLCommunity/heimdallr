# Vendored Discord message editor

`components.mjs` is the browser ES module for `@myrkvi/components`, including
Lit, downloaded from esm.sh's standalone bundle. The application can import
`/static/vendor/components/components.mjs` without contacting a CDN.

Update from the repository root:

```sh
task components:update VERSION=0.1.2
```

Omit `VERSION` to fetch the version recorded in `manifest.json` again. Updates
require Node.js 22+ and curl. Normal builds require neither and do not download
this dependency. Commit the updated bundle and manifest together.

The updater follows esm.sh's re-export wrapper, parses the resulting module,
rejects remaining module dependencies, and removes its remote source-map
reference. License comments embedded in the bundle are preserved. Review new
dependencies and their licenses on each update.

The manifest records source URLs, browser target, byte size, and SHA-256 of the
vendored file. A pinned package version does not freeze esm.sh's build service
or the package's transitive version ranges; refetching can produce different
bytes. The committed artifact and its checksum identify the deployed version.

The upstream package declares MIT in its published `deno.json`; version 0.1.2
does not publish a separate copyright/license file. See `LICENSES.txt` for the
MIT terms and bundled Lit BSD license. Upstream copyright notices, where
provided, remain in the bundle.
