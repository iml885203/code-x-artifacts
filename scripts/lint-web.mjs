import { createHash } from "node:crypto";
import { execFile, execFileSync } from "node:child_process";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, relative, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";

const require = createRequire(new URL("../web/package.json", import.meta.url));
const ts = require("typescript");
const { ESLint } = require("eslint");
const execute = promisify(execFile);
const checksum = (key, output) => createHash("sha256").update(JSON.stringify([key, output])).digest("hex");

// Hooks export repository-local variables even when a child changes cwd.
// https://git-scm.com/docs/githooks#_description
export function foreignGitEnvironment(environment = process.env) {
  const env = { ...environment };
  const names = execFileSync("git", ["rev-parse", "--local-env-vars"], { encoding: "utf8" }).trim().split("\n");
  for (const name of names) delete env[name];
  return env;
}

// Type-aware lint depends on imported types. Reuse only a successful whole run
// with the same resolved program, repository configs and pinned tooling.
export async function lintWeb(directory, cacheDirectory) {
  const inputs = new Map();
  const remember = (file) => {
    const text = ts.sys.readFile(file);
    if (text !== undefined) inputs.set(resolve(file), text);
    return text;
  };
  const fail = (error) => { throw new Error(ts.flattenDiagnosticMessageText(error.messageText, "\n")); };
  const parsed = ts.getParsedCommandLineOfConfigFile(resolve(directory, "tsconfig.json"), {}, {
    ...ts.sys, readFile: remember, onUnRecoverableConfigFileDiagnostic: fail,
  });
  if (parsed.errors.length) fail(parsed.errors[0]);
  const host = ts.createCompilerHost(parsed.options);
  host.readFile = remember;
  ts.createProgram({ rootNames: parsed.fileNames, options: parsed.options, host });
  for (const file of ["package.json", "package-lock.json", "eslint.config.js"]) {
    const path = resolve(directory, file);
    inputs.set(path, await readFile(path, "utf8"));
  }
  const configs = [resolve(directory, "eslint.config.js")];
  const visited = new Set();
  while (configs.length) {
    const file = configs.pop();
    if (visited.has(file)) continue;
    visited.add(file);
    const text = remember(file);
    for (const imported of ts.preProcessFile(text, true, true).importedFiles) {
      const name = imported.fileName;
      if (!name.startsWith(".") && !name.startsWith("/") && !name.startsWith("file:")) continue;
      const dependency = createRequire(pathToFileURL(file)).resolve(name.startsWith("file:") ? fileURLToPath(name) : name);
      if (!dependency.includes("/node_modules/")) configs.push(dependency);
    }
  }
  const env = foreignGitEnvironment();
  const root = execFileSync("git", ["rev-parse", "--show-toplevel"], { cwd: directory, env }).toString().trim();
  const names = execFileSync("git", ["ls-files", "--cached", "--others", "--exclude-standard", "-z"], { cwd: root, env }).toString().split("\0").filter(Boolean);
  for (const name of names) {
    if (/\.(?:[cm]?[jt]sx?|json)$/.test(name) || name.endsWith("lock")) {
      const file = resolve(root, name);
      try {
        inputs.set(file, await readFile(file, "utf8"));
      } catch (error) {
        if (error.code !== "ENOENT") throw error;
      }
    }
  }
  // ESLint also sees untracked or git-ignored JS outside tsconfig's roots.
  const directories = [directory];
  while (directories.length) {
    const parent = directories.pop();
    for (const entry of await readdir(parent, { withFileTypes: true })) {
      const file = resolve(parent, entry.name);
      if (entry.isDirectory() && entry.name !== "node_modules") directories.push(file);
      else if (entry.isFile() && /\.(?:[cm]?[jt]sx?|json)$/.test(entry.name)) inputs.set(file, await readFile(file, "utf8"));
    }
  }
  const hash = createHash("sha256").update(process.version).update(ts.version).update(ESLint.version).update(await readFile(fileURLToPath(import.meta.url)));
  for (const [file, text] of [...inputs].sort(([a], [b]) => a.localeCompare(b))) {
    hash.update(JSON.stringify([relative(directory, file), text]));
  }
  const key = hash.digest("hex");
  await mkdir(cacheDirectory, { recursive: true });
  const record = resolve(cacheDirectory, "success.json");
  try {
    const previous = JSON.parse(await readFile(record, "utf8"));
    if (previous && previous.key === key && typeof previous.output === "string" && previous.checksum === checksum(key, previous.output)) {
      return { reused: true, output: previous.output, errors: 0, key };
    }
  } catch (error) {
    if (error.code !== "ENOENT" && !(error instanceof SyntaxError)) throw error;
  }
  await rm(record, { force: true });
  const command = resolve(dirname(require.resolve("eslint/package.json")), "bin/eslint.js");
  let output;
  let errors = 0;
  try {
    const result = await execute(process.execPath, [command, "."], { cwd: directory, encoding: "utf8" });
    output = result.stdout + result.stderr;
  } catch (error) {
    if (error.code !== 1) throw error;
    output = error.stdout + error.stderr;
    errors = 1;
  }
  if (!errors) await writeFile(record, JSON.stringify({ key, output, checksum: checksum(key, output) }));
  return { reused: false, output, errors, key };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const result = await lintWeb(resolve(root, "web"), resolve(root, ".cache/eslint"));
  console.log(`typed lint: ${result.reused ? "unchanged program" : "changed or uncached program"}`);
  if (result.output) process.stdout.write(result.output);
  process.exitCode = result.errors ? 1 : 0;
}
