# Editor integration checks

Controller checks use Node's built-in test runner:

```sh
node web/tests/post-editor.test.cjs
```

Browser tests render the real Go templates into a temporary directory, then
exercise the vendored editor with Alpine and HTMX. All HTTP requests are
intercepted: these tests never contact Discord or require a running bot.

```sh
npm ci --prefix web/tests
cd web/tests
npx playwright install chromium
npm run test:browser
```

Go and Node.js 22+ must be available. `CHROMIUM_PATH` optionally selects an
existing Chromium executable; `EDITOR_SCREENSHOT` writes a narrow-layout
screenshot. Test dependencies are development-only and do not participate in
the Go build or editor vendoring.
