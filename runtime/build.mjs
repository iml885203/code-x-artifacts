// Builds runtime v1: the libraries pictor serves to artifact pages, one ESM file
// per import specifier, into ../internal/jsruntime/v1, which the Go build embeds.
//
//   node build.mjs           write the output
//   node build.mjs --check   build into a scratch folder and fail when it differs
//                            from what is committed (make verify-runtime)
//
// Node is needed here and only here: the binary carries the finished files.
// The output must depend on nothing but package-lock.json, package.json, src/
// and this script, so the same inputs give the same bytes on a laptop, in CI
// and in the Docker build.
process.env.NODE_ENV = "production"; // the exports of React's production builds are what ships

import * as esbuild from "esbuild";
import { createHash } from "node:crypto";
import { createRequire } from "node:module";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.dirname(fileURLToPath(import.meta.url));
process.chdir(root); // esbuild reports and hashes paths relative to here
const require = createRequire(import.meta.url);
const committed = path.join(root, "..", "internal", "jsruntime", "v1");
const check = process.argv.includes("--check");

const sha256 = (buf) => createHash("sha256").update(buf).digest("hex");
const identifier = /^[A-Za-z_$][\w$]*$/;
const namedExports = (spec) => Object.keys(require(spec)).filter((k) => k !== "default" && identifier.test(k)).sort();

// React and React DOM are CommonJS and have no static named exports, so each
// gets an ESM wrapper that lists them. Everything else imports them as
// externals, which the page's import map points at the one copy.
const commonjs = {
  react: "react",
  "react/jsx-runtime": "react/jsx-runtime",
  "react-dom": "react-dom",
  "react-dom/client": "react-dom/client",
};
const external = Object.keys(commonjs);

// specifier -> the module that provides it. The key is also the start of the file name.
const modules = {
  react: "wrap:react",
  "react/jsx-runtime": "wrap:react/jsx-runtime",
  "react-dom": "wrap:react-dom",
  "react-dom/client": "wrap:react-dom/client",
  recharts: "node_modules/recharts/es6/index.js",
  "lucide-react": "node_modules/lucide-react/dist/esm/lucide-react.mjs",
  "@/lib/utils": "src/lib/utils.ts",
};
for (const f of readdirSync("src/ui").sort()) {
  modules[`@/components/ui/${f.replace(/\.tsx$/, "")}`] = `src/ui/${f}`;
}
const stem = (specifier) => specifier.replace(/^@\//, "").replace(/[^A-Za-z0-9]+/g, "-");

const wrappers = {
  name: "commonjs-wrappers",
  setup(b) {
    b.onResolve({ filter: /^wrap:/ }, (a) => ({ path: a.path.slice("wrap:".length), namespace: "wrap" }));
    b.onLoad({ filter: /.*/, namespace: "wrap" }, (a) => ({
      contents: `import m from ${JSON.stringify(require.resolve(a.path))};\nexport default m;\nexport const { ${namedExports(a.path).join(", ")} } = m;\n`,
      loader: "js",
      resolveDir: root,
    }));
    // A dependency that is CommonJS calls require("react") (Recharts' Redux
    // does). Keep import statements external, and send require calls to a
    // module that re-exports the external one; without it the page is blank
    // ("Dynamic require of react is not supported").
    for (const spec of external) {
      b.onResolve({ filter: new RegExp(`^${spec}$`) }, (a) => (a.kind === "require-call" ? { path: spec, namespace: "require-shim" } : undefined));
    }
    b.onLoad({ filter: /.*/, namespace: "require-shim" }, (a) => ({
      contents: `export { ${namedExports(a.path).join(", ")} } from ${JSON.stringify(a.path)};`,
      loader: "js",
      resolveDir: root,
    }));
  },
};

const scratch = mkdtempSync(path.join(tmpdir(), "pictor-runtime-"));
const out = path.join(scratch, "v1");
try {
  const entryPoints = Object.fromEntries(Object.entries(modules).map(([s, m]) => [stem(s), m]));
  const result = await esbuild.build({
    entryPoints,
    outdir: out,
    bundle: true,
    splitting: true,
    format: "esm",
    minify: true,
    target: "es2022",
    jsx: "automatic",
    external,
    define: { "process.env.NODE_ENV": '"production"' },
    entryNames: "[name]-[hash]",
    chunkNames: "chunk-[hash]",
    legalComments: "none",
    metafile: true,
    logLevel: "warning",
    plugins: [wrappers],
  });

  // specifier -> { file, exports }: what the compiler allows pages to import
  // and what the import map points at.
  const byEntry = {};
  for (const [file, o] of Object.entries(result.metafile.outputs)) {
    if (o.entryPoint) byEntry[o.entryPoint] = { file: path.basename(file), exports: [...o.exports].sort() };
  }
  const specifiers = {};
  for (const [s, m] of Object.entries(modules)) {
    const entry = byEntry[m];
    if (!entry) throw new Error(`runtime: no output for ${s} (${m})`);
    if (entry.exports.length === 0) throw new Error(`runtime: ${s} exports nothing`);
    specifiers[s] = entry;
  }

  // Tailwind's browser build is a classic script, not a module: copied as it is, named by its hash.
  const tailwind = readFileSync("node_modules/@tailwindcss/browser/dist/index.global.js");
  const tailwindFile = `tailwind-browser-${sha256(tailwind).slice(0, 8)}.js`;
  writeFileSync(path.join(out, tailwindFile), tailwind);

  // Every bundled package's license text travels with the files.
  const packages = new Map();
  const note = (dir) => {
    const pkg = JSON.parse(readFileSync(path.join(dir, "package.json"), "utf8"));
    const author = typeof pkg.author === "string" ? pkg.author : pkg.author?.name;
    packages.set(pkg.name, { dir, version: pkg.version, license: pkg.license, author, repository: pkg.repository?.url ?? pkg.repository });
  };
  for (const input of Object.keys(result.metafile.inputs)) {
    const m = /^(.*node_modules\/(?:@[^/]+\/)?[^/]+)\//.exec(input);
    if (m) note(m[1]);
  }
  for (const spec of external) note(path.dirname(require.resolve(`${spec.split("/")[0]}/package.json`)));
  note("node_modules/@tailwindcss/browser");
  let notices = "Third-party software in pictor runtime v1. Generated by runtime/build.mjs.\n";
  for (const [name, p] of [...packages].sort(([a], [b]) => (a < b ? -1 : 1))) {
    const licenseFile = readdirSync(p.dir).find((f) => /^licen[cs]e(\.|$)/i.test(f));
    // Some packages ship no license file; their declared license and author are
    // all there is to carry. A package with neither fails the build.
    if (!licenseFile && !p.license) throw new Error(`runtime: ${name} declares no license and ships no license file; do not bundle it`);
    const text = licenseFile
      ? readFileSync(path.join(p.dir, licenseFile), "utf8").replace(/\r\n?/g, "\n").trim()
      : `No license file is shipped. Declared license: ${p.license}. Author: ${p.author ?? "not declared"}. Source: ${p.repository ?? "not declared"}.`;
    notices += `\n${"=".repeat(72)}\n${name}@${p.version} (${p.license ?? "see text"})\n${"=".repeat(72)}\n\n${text}\n`;
  }
  writeFileSync(path.join(out, "NOTICES.txt"), notices);

  // The manifest names every file with its hash so a test can check the
  // embedded folder without Node, and records how the runtime is used.
  const files = {};
  for (const f of readdirSync(out).sort()) files[f] = sha256(readFileSync(path.join(out, f)));
  const manifest = { runtime: "v1", specifiers, scripts: { tailwind: tailwindFile }, notices: "NOTICES.txt", files };
  writeFileSync(path.join(out, "manifest.json"), JSON.stringify(manifest, null, 1) + "\n");

  const built = readdirSync(out).sort();
  if (check) {
    const have = existsSync(committed) ? readdirSync(committed).sort() : [];
    const problems = [];
    for (const f of built) {
      if (!have.includes(f)) problems.push(`missing from internal/jsruntime/v1: ${f}`);
      else if (!readFileSync(path.join(out, f)).equals(readFileSync(path.join(committed, f)))) problems.push(`differs from a fresh build: ${f}`);
    }
    for (const f of have) if (!built.includes(f)) problems.push(`not produced by a fresh build: ${f}`);
    if (problems.length > 0) {
      console.error(problems.join("\n"));
      console.error("runtime: internal/jsruntime/v1 does not match runtime/package-lock.json and runtime/src. Run `make runtime` and commit the result.");
      process.exitCode = 1;
    } else {
      console.log(`runtime: internal/jsruntime/v1 matches a fresh build (${String(built.length)} files)`);
    }
  } else {
    rmSync(committed, { recursive: true, force: true });
    mkdirSync(path.dirname(committed), { recursive: true });
    cpSync(out, committed, { recursive: true });
    console.log(`runtime: wrote ${String(built.length)} files to internal/jsruntime/v1`);
  }
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
