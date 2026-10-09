// Only used when updating vendored assets; normal Go builds need no JS tooling.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { SourceTextModule } from "node:vm";

const destination = new URL("../web/static/vendor/components/", import.meta.url);
const manifestURL = new URL("manifest.json", destination);

function download(url) {
  return execFileSync("curl", [
    "--fail", "--silent", "--show-error", "--location",
    "--proto", "=https", "--proto-redir", "=https",
    "--connect-timeout", "15", "--max-time", "120", "--retry", "2", url,
  ], { encoding: "utf8", maxBuffer: 10 * 1024 * 1024 });
}

async function update() {
  const previous = await readFile(manifestURL, "utf8")
    .then(JSON.parse).catch(error => {
      if (error.code === "ENOENT") return {};
      throw error;
    });
  const version = process.env.COMPONENTS_VERSION || previous.version;
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version || "")) {
    throw new Error("Supply an exact release: task components:update VERSION=x.y.z");
  }

  const source = `https://esm.sh/jsr/@myrkvi/components@${version}?standalone&target=es2022`;
  const entry = download(source);
  // esm.sh's public URL is a re-export wrapper, not the standalone module.
  // Fail if the wrapper gains imports (e.g. an unbundled peer dependency).
  const wrapper = entry.match(/^\/\*[^]*?\*\/\s*export \* from "(\/[^"\s]+\.bundle\.mjs)";?\s*$/);
  if (!wrapper) throw new Error("Unexpected esm.sh wrapper; inspect it before updating");
  const resolved = new URL(wrapper[1], source);
  if (resolved.origin !== "https://esm.sh") throw new Error("Unexpected bundle origin");
  const bundle = download(resolved.href).replace(/\n\/\/# sourceMappingURL=[^\n]*\s*$/, "\n");
  const module = new SourceTextModule(bundle);
  if (module.dependencySpecifiers.length || /\bimport\s*(?:\(|\.)/.test(bundle)) {
    throw new Error("Bundle contains external imports or import.meta; refusing to vendor it");
  }
  if (!bundle.includes('customElements.define("discord-message-editor"') ||
      !bundle.includes('customElements.define("discord-message-preview"')) {
    throw new Error("Bundle does not register the expected editor and preview elements");
  }

  const metadataSource = `https://jsr.io/@myrkvi/components/${version}/deno.json`;
  const metadata = JSON.parse(download(metadataSource));
  if (metadata.license !== "MIT" || metadata.version !== version) {
    throw new Error("Package version or license changed; review before updating");
  }
  const manifest = {
    package: "@myrkvi/components", version, license: metadata.license,
    source, resolved: resolved.href, metadataSource,
    target: "es2022", bytes: Buffer.byteLength(bundle),
    sha256: createHash("sha256").update(bundle).digest("hex"),
  };
  // Download and validate everything before touching the checked-in files.
  await mkdir(destination, { recursive: true });
  const files = [
    ["components.mjs", bundle],
    ["manifest.json", JSON.stringify(manifest, null, 2) + "\n"],
  ];
  try {
    for (const [name, contents] of files) {
      await writeFile(new URL(name + ".tmp", destination), contents);
    }
    for (const [name] of files) {
      await rename(new URL(name + ".tmp", destination), new URL(name, destination));
    }
  } finally {
    for (const [name] of files) await rm(new URL(name + ".tmp", destination), { force: true });
  }
  console.log(`Vendored @myrkvi/components@${version}: ${manifest.bytes} bytes, SHA-256 ${manifest.sha256}`);
  console.log("Review the bundle, license notices, and manifest diff before committing.");
}

update().catch(error => {
  console.error(error.message);
  process.exitCode = 1;
});
