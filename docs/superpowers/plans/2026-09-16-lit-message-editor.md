# Lit message editor implementation plan

**Goal:** Replace the shared V2 editor with the vendored Lit editor and make post message boundaries explicit.

**Approved design:** Conversation of 2026-09-16: pinned self-hosted bundle, Taskfile updater, shared Lit adapter, explicit ordered messages for posts, preserve existing content and backend workflows.

**Architecture:** The component owns editing; Alpine owns page state, persistence and actions. A small lazy-loading adapter assigns plain JSON objects to the editor, propagates change events to host state, and feeds a local preview. Settings/sandbox retain component-array payloads. Posts use a versioned document in the existing text column and request field.

**Contract:** `components_json` for posts is `{"version":1,"messages":[{"components":[]}]}`. Legacy arrays are read using the existing splitter to establish boundaries, then converted on save. GET renders the canonical document through the existing `data-initial` attribute. No eager database rewrite is needed. Explicit messages are validated individually and never automatically merged or split. Empty drafts may be saved; publishing empty messages is rejected before any Discord changes. Post messages remain ordered and the existing sync engine receives their component arrays.

**Constraints:** Preserve unsupported existing data rather than silently dropping it. Only link buttons and URL-based media are newly authorable. Keep server validation, template expansion, emoji resolution, authorization and optimistic locking. Avoid native form validation preventing unrelated settings saves when V2 is off. Async initialization must not overwrite data or enable saving before ready. Use stable UI keys when reordering messages. Retain the previous vendoring work. Work on `codex/lit-message-editor` in the current checkout so the existing uncommitted bundle stays with its integration.

## Tasks

- [x] Backend: add document parsing/canonicalization and per-message validation in `web/posts/document.go`; tests for legacy conversion, preserving boundaries, over-limit and empty messages. Update `web/handlers_posts.go` GET/save/preview/publish and handler tests; document the storage format in `model/post.go`. Preserve existing handlers' security and failure semantics.
- [x] Browser: replace `web/templates/components/message_editor.templ`, remove the old lossy editor-model conversion, and add a lazy module adapter. Update settings and sandbox bindings, including explicit change propagation into hidden inputs and dirty state. Use the exact vendored 0.1.2 API, not newer local package source.
- [x] Posts: update `post-editor.js` and page template for stable keyed message cards, add/remove/reorder, per-message Lit editors/previews and versioned serialization. Preserve existing save/publish actions and prevent publishing stale unsaved edits. Server preview remains a publish validation result, not a splitter.
- [x] Verification: generate templ code; run targeted Go tests and full suite if practical. Browser smoke tests with actual Alpine/HTMX and the vendored bundle cover edit/save, multiple instances, V2 toggle, async loading, HTMX replacement, explicit message ordering and load failures. Review final diff and fix actionable findings.

## Progress

- Initial review: backend owns Go handlers/document/tests; main agent owns JS/templates/CSS and generation. Shared interface is the JSON contract above.
- Ruling: use the current feature-branch checkout rather than a new worktree to retain the already-approved uncommitted vendor work.

- Backend complete: strict document parsing, legacy conversion on read/save, explicit-message publish/preview validation. Validation uses copies to preserve stored component fields.
- Browser complete: shared lazy adapter, local previews, stable message cards, failure handling and form synchronization. Explicit string bindings preserve disabled-editor state in Alpine. Form dirty baselines wait for child hidden-input initialization.
- Verified: `GOCACHE=/tmp/heimdallr-gocache go test ./... -count=1`; `go build -o /tmp/heimdallr-lit-editor .`; four Node controller tests; six Chromium integration checks. Browser checks use real generated templates, Alpine, HTMX and the bundle with intercepted HTTP. Narrow layout also inspected with the dashboard's actual Pico stylesheet.
- Final independent review: no actionable findings. Implementation completed on the feature branch; no live Discord publication was performed.
