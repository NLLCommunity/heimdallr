// Post pages own an ordered document of explicitly bounded Discord messages.
document.addEventListener("alpine:init", () => {
  Alpine.data("postEditor", () => ({
    messages: [],
    name: "",
    channelId: "",
    version: 0,
    busy: false,
    message: "",
    initialError: false,
    saveUrl: "",
    publishUrl: "",
    unpublishUrl: "",
    deleteUrl: "",
    previewUrl: "",
    redirectUrlPrefix: "",
    _nextKey: 0,
    _savedSnapshot: "",
    _root: null,
    _previewTimer: null,
    _previewAbort: null,
    init() {
      this._root = this.$el;
      const ds = this.$el.dataset;
      this.name = ds.name || "";
      this.channelId = ds.channelId || "";
      this.version = parseInt(ds.version || "0", 10);
      this.saveUrl = ds.saveUrl || "";
      this.publishUrl = ds.publishUrl || "";
      this.unpublishUrl = ds.unpublishUrl || "";
      this.deleteUrl = ds.deleteUrl || "";
      this.previewUrl = ds.previewUrl || "";
      this.redirectUrlPrefix = ds.redirectUrlPrefix || "";
      try {
        const parsed = JSON.parse(ds.initial);
        if (parsed.version !== 1 || !Array.isArray(parsed.messages) ||
            !parsed.messages.every(entry => entry && Array.isArray(entry.components))) {
          throw new Error("Invalid post document");
        }
        this.messages = parsed.messages.map(entry => ({ key: this._nextKey++, components: entry.components }));
        if (!this.messages.length) this.addMessage();
      } catch (e) {
        this.initialError = true;
        this.message = "Could not load this post. Reload the page before saving.";
        return;
      }
      const sel = this.$el.querySelector('select[name="channel_id"]');
      if (sel) {
        this.channelId = sel.value;
        sel.addEventListener("change", () => { this.channelId = sel.value; });
      }
      this._savedSnapshot = this.snapshot();
      this.$watch("messages", () => this.schedulePreviewRefresh());
      this.refreshPreview();
    },
    destroy() {
      clearTimeout(this._previewTimer);
      this._previewAbort?.abort();
    },
    get dirty() {
      return this.snapshot() !== this._savedSnapshot;
    },
    snapshot() {
      return JSON.stringify({ name: this.name, channelId: this.channelId, document: this.serialize() });
    },
    serialize() {
      return { version: 1, messages: this.messages.map(entry => ({ components: entry.components })) };
    },
    addMessage() {
      this.messages.push({ key: this._nextKey++, components: [] });
    },
    removeMessage(index) {
      if (this.messages.length > 1) this.messages.splice(index, 1);
    },
    moveMessage(index, direction) {
      const target = index + direction;
      if (target < 0 || target >= this.messages.length) return;
      const [entry] = this.messages.splice(index, 1);
      this.messages.splice(target, 0, entry);
    },
    schedulePreviewRefresh() {
      clearTimeout(this._previewTimer);
      // Abort now, rather than after the delay, to avoid displaying stale results.
      this._previewAbort?.abort();
      this._previewTimer = setTimeout(() => {
        this._previewTimer = null;
        this.refreshPreview();
      }, 500);
    },
    async save() {
      if (this.busy || this.initialError) return;
      if (!HeimdallrEditors.ready(this._root)) {
        this.message = "Wait for the message editors to load before saving. Reload if an editor failed.";
        return;
      }
      const submittedSnapshot = this.snapshot();
      this.busy = true;
      this.message = "";
      try {
        const body = new URLSearchParams({
          version: String(this.version),
          name: this.name,
          components_json: JSON.stringify(this.serialize()),
          channel_id: this.channelId || "",
        });
        const resp = await fetch(this.saveUrl, {
          method: "POST",
          headers: {
            "Content-Type": "application/x-www-form-urlencoded",
            "X-Requested-With": "XMLHttpRequest",
          },
          body,
        });
        if (resp.status === 401) {
          window.location.assign("/login");
          return;
        }
        if (resp.status === 409) {
          this.message =
            "This post was updated elsewhere. Reload to see the latest version.";
          return;
        }
        if (!resp.ok) {
          this.message = (await resp.text()) || "Save failed.";
          return;
        }
        const data = await resp.json().catch(() => null);
        // New posts start with version 0 in the data attribute; the server
        // assigned a real ID on first save and the client must navigate so
        // publish/delete/etc. become available. If the response can't be
        // parsed or lacks an id, treat it as an error rather than silently
        // staying on /posts/new — clicking Save again would otherwise insert
        // a second row.
        const isNewPost = this.version === 0;
        if (isNewPost) {
          if (!data || typeof data.id !== "number" || !this.redirectUrlPrefix) {
            this.message =
              "Save returned an unexpected response. Reload the post list to see if it was saved.";
            return;
          }
          window.location.assign(this.redirectUrlPrefix + data.id);
          return;
        }
        // Existing-post save: bump version so the next save doesn't 409.
        if (data && typeof data.version === "number") {
          this.version = data.version;
        }
        this._savedSnapshot = submittedSnapshot;
        this.message = "Saved.";
      } catch (e) {
        // fetch() rejects on network failures (offline, DNS, TLS, CORS); without
        // this catch the rejection bubbles to an unhandled rejection and the
        // user sees no feedback at all.
        this.message = "Network error — check your connection and try again.";
      } finally {
        this.busy = false;
      }
    },
    async publish() {
      if (this.busy || this.initialError) return;
      if (this.dirty) {
        this.message = "Save your changes before publishing.";
        return;
      }
      await this._postAction(this.publishUrl, "Published.");
    },
    async unpublish() {
      await this._postAction(this.unpublishUrl, "Unpublished from Discord.");
    },
    async del() {
      if (
        !confirm(
          "Delete this post permanently? Discord messages will also be removed.",
        )
      )
        return;
      try {
        const resp = await fetch(this.deleteUrl, {
          method: "POST",
          headers: { "X-Requested-With": "XMLHttpRequest" },
        });
        if (resp.status === 401) {
          window.location.assign("/login");
          return;
        }
        if (resp.ok) {
          window.location.assign(
            this.deleteUrl.replace(/\/posts\/\d+\/delete$/, "/posts"),
          );
        } else {
          this.message = (await resp.text()) || "Delete failed.";
        }
      } catch (e) {
        this.message = "Network error — check your connection and try again.";
      }
    },
    async _postAction(url, okMsg) {
      if (this.busy) return;
      this.busy = true;
      this.message = "";
      try {
        const resp = await fetch(url, {
          method: "POST",
          headers: { "X-Requested-With": "XMLHttpRequest" },
        });
        if (resp.status === 401) {
          window.location.assign("/login");
          return;
        }
        if (!resp.ok) {
          this.message = (await resp.text()) || "Request failed.";
          return;
        }
        this.message = okMsg;
      } catch (e) {
        // Same rationale as save(): fetch network errors must surface to the
        // user instead of becoming an unhandled rejection.
        this.message = "Network error — check your connection and try again.";
      } finally {
        this.busy = false;
      }
    },
    async refreshPreview() {
      if (!this.previewUrl) return;
      // Cancel any in-flight preview so a slow response from a stale state
      // can't clobber a faster one from a newer state.
      if (this._previewAbort) this._previewAbort.abort();
      const controller = new AbortController();
      this._previewAbort = controller;
      try {
        const resp = await fetch(this.previewUrl, {
          method: "POST",
          headers: {
            "Content-Type": "application/x-www-form-urlencoded",
            "X-Requested-With": "XMLHttpRequest",
          },
          body: new URLSearchParams({
            components_json: JSON.stringify(this.serialize()),
          }),
          signal: controller.signal,
        });
        if (controller.signal.aborted) return;
        if (resp.status === 401) {
          // Match save/_postAction/del: redirect on session expiry instead
          // of silently swallowing. Without this, the preview just stops
          // updating after the session times out and the user has no idea
          // why their typing isn't reflected — meanwhile the next Save would
          // redirect, but only after they noticed the preview was frozen.
          // Any unsaved edits are lost either way (the next Save would 401
          // too); redirecting from here makes the failure mode obvious.
          window.location.assign("/login");
          return;
        }
        // Success path returns a templ partial (text/html). On 4xx/5xx the
        // handler emits text/plain via http.Error; swapping that into the
        // preview pane would replace the last good preview with a raw error
        // string. Leave the previous content in place instead.
        if (!resp.ok) return;
        const ct = resp.headers.get("content-type") || "";
        if (!ct.includes("text/html")) return;
        const html = await resp.text();
        if (controller.signal.aborted) return;
        const target = this._root.querySelector("[data-publish-preview]");
        if (target) target.innerHTML = html;
      } catch (e) {
        /* swallow */
      }
    },
  }));
});
