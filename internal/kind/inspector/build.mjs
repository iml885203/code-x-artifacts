// Builds the inspector every design frame runs, from the modules in this folder
// into ../design_inspector.js, which the Go build embeds (internal/kind/design.go).
//
//   node build.mjs           write the output
//   node build.mjs --check   fail when the committed file is not what the modules build
//                            (make verify-inspector)
//
// The file is in every frame and is pinned by hash in the frame's CSP, so what
// ships is the bundle with compact whitespace; source modules and identifiers
// remain readable. The size limit is a Go test on the
// built file (TestTheInspectorStaysSmallBecauseItIsInEveryFrame).
//
// esbuild is the one the runtime build pins (runtime/package-lock.json).
import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const esbuild = createRequire(path.join(here, "../../../runtime/package.json"))("esbuild");
const out = path.join(here, "..", "design_inspector.js");

const result = await esbuild.build({
  entryPoints: [path.join(here, "main.js")],
  bundle: true,
  format: "iife",
  target: "es2022",
  charset: "utf8",
  minifyWhitespace: true,
  legalComments: "none",
  write: false,
  logLevel: "warning",
});
const built = "// Built from internal/kind/inspector by `make inspector`; do not edit.\n" + result.outputFiles[0].text;

if (process.argv.includes("--check")) {
  if (readFileSync(out, "utf8") !== built) {
    console.error("inspector: internal/kind/design_inspector.js is not what internal/kind/inspector builds; run `make inspector` and commit");
    process.exit(1);
  }
  console.log(`inspector: design_inspector.js matches a fresh build (${built.length} bytes)`);
} else {
  writeFileSync(out, built);
  console.log(`inspector: wrote design_inspector.js (${built.length} bytes)`);
}
