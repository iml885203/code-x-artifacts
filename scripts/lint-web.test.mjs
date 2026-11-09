import assert from "node:assert/strict";
import { execFile, spawnSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, rename, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { describe, test } from "node:test";
import { pathToFileURL } from "node:url";
import { foreignGitEnvironment, lintWeb } from "./lint-web.mjs";

const require = createRequire(new URL("../web/package.json", import.meta.url));
const git = (directory, ...args) => {
  const result = spawnSync("git", args, { cwd: directory, env: foreignGitEnvironment(), encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.trim();
};

async function fixture(t) {
  const workspace = await mkdtemp(resolve(".tmp/lint-web-"));
  t.after(() => rm(workspace, { recursive: true, force: true }));
  git(workspace, "init", "--quiet");
  await writeFile(resolve(workspace, ".gitignore"), "cache\n.tmp\n");
  const directory = resolve(workspace, "project");
  const cache = resolve(workspace, "cache");
  await mkdir(directory);
  await writeFile(resolve(directory, "package.json"), '{"type":"module"}');
  await writeFile(resolve(directory, "package-lock.json"), "{}");
  // These fixtures use primitive types; unrelated standard libraries add work to every fresh program.
  await writeFile(resolve(directory, "tsconfig.json"), JSON.stringify({ compilerOptions: { strict: true, module: "esnext", moduleResolution: "bundler", lib: ["es5"], types: [] }, include: ["*.ts"] }));
  await writeFile(resolve(workspace, "config.mjs"), "export default {};\n");
  await writeFile(resolve(directory, "eslint.config.js"), `import extra from "../config.mjs";
import tseslint from ${JSON.stringify(pathToFileURL(require.resolve("typescript-eslint")).href)};
export default [{ ignores: ["eslint.config.js"] }, ...tseslint.configs.strictTypeChecked, { languageOptions: { parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname } }, rules: extra }];`);
  const consumer = 'import type { Value } from "../dependency";\nexport const accept = (value: Value): number => value;\n';
  await writeFile(resolve(directory, "consumer.ts"), consumer);
  await writeFile(resolve(workspace, "dependency.ts"), "export type Value = number;\n");
  return { workspace, directory, cache, consumer, run: () => lintWeb(directory, cache) };
}

async function track(f, path) {
  git(f.workspace, "add", "--", path);
  assert.equal(git(f.workspace, "rev-parse", "--show-toplevel"), f.workspace);
}

// Fixtures own their repositories, caches and fresh ESLint processes. Bound
// independent cases to two on the four-core CI runner.
describe("typed lint cache", { concurrency: 2 }, () => {
  test("an imported type change outside the lint directory rejects the unchanged consumer and removes the previous success", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    const first = await f.run();
    assert.equal(first.errors, 0, first.output);
    assert.equal(first.reused, false);
    assert.equal((await f.run()).reused, true);
    await writeFile(resolve(f.workspace, "dependency.ts"), "export type Value = any;\n");
    const changed = await f.run();
    assert.equal(await readFile(resolve(f.directory, "consumer.ts"), "utf8"), f.consumer);
    assert.equal(changed.reused, false);
    assert.match(changed.output, /no-unsafe-return/);
    assert.ok(changed.errors > 0);
    await assert.rejects(readFile(resolve(f.cache, "success.json")), { code: "ENOENT" });
  });

  test("failed lint results never create a reusable success record", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    await writeFile(resolve(f.workspace, "dependency.ts"), "export type Value = any;\n");
    const failed = await f.run();
    assert.equal(failed.reused, false);
    assert.match(failed.output, /no-unsafe-return/);
    assert.ok(failed.errors > 0);
    await assert.rejects(readFile(resolve(f.cache, "success.json")), { code: "ENOENT" });
    const repeated = await f.run();
    assert.equal(repeated.reused, false);
    assert.match(repeated.output, /no-unsafe-return/);
    assert.ok(repeated.errors > 0);
    await assert.rejects(readFile(resolve(f.cache, "success.json")), { code: "ENOENT" });
  });

  test("an imported untracked JS config change rejects previously successful code", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    assert.equal((await f.run()).errors, 0);
    await writeFile(resolve(f.workspace, "config.mjs"), 'export default { "no-restricted-syntax": ["error", "ArrowFunctionExpression"] };\n');
    const changed = await f.run();
    assert.equal(changed.reused, false);
    assert.match(changed.output, /no-restricted-syntax/);
  });

  test("corrupt success records recover through a full successful lint run", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    assert.equal((await f.run()).errors, 0);
    await writeFile(resolve(f.cache, "success.json"), "{broken");
    const corrupt = await f.run();
    assert.equal(corrupt.reused, false);
    assert.equal(corrupt.errors, 0, corrupt.output);
    assert.equal((await f.run()).reused, true);
  });

  test("newly added untracked inputs invalidate a successful lint result", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    assert.equal((await f.run()).errors, 0);
    assert.equal((await f.run()).reused, true);
    await writeFile(resolve(f.directory, "new.ts"), "export const value: any = 1;\n");
    const added = await f.run();
    assert.equal(added.reused, false);
    assert.ok(added.errors > 0);
  });

  test("dependency lock changes invalidate successful lint", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    assert.equal((await f.run()).errors, 0);
    await writeFile(resolve(f.directory, "package-lock.json"), '{"lockfileVersion":3}');
    const changed = await f.run();
    assert.equal(changed.reused, false);
    assert.equal(changed.errors, 0, changed.output);
  });

  test("warm success never hides a git-ignored JS file that ordinary ESLint rejects", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    assert.equal((await f.run()).errors, 0);
    assert.equal((await f.run()).reused, true);
    await mkdir(resolve(f.directory, ".tmp"));
    await writeFile(resolve(f.directory, ".tmp/bad.js"), "export const = ;\n");
    assert.match(git(f.workspace, "check-ignore", "--", resolve(f.directory, ".tmp/bad.js")), /bad\.js$/);
    const original = await new Promise((resolveResult) => {
      execFile(process.execPath, [resolve(dirname(require.resolve("eslint/package.json")), "bin/eslint.js"), "."], { cwd: f.directory, encoding: "utf8" }, (error, stdout, stderr) => {
        resolveResult({ status: error?.code ?? 0, stdout, stderr });
      });
    });
    assert.equal(original.status, 1, original.stdout + original.stderr);
    assert.match(original.stdout, /bad\.js/);
    const guarded = await f.run();
    assert.equal(guarded.reused, false);
    assert.ok(guarded.errors > 0);
    assert.match(guarded.output, /bad\.js/);
  });

  test("unstaged tracked rename lints the valid remaining tree, including after staging", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    await writeFile(resolve(f.directory, "extra.json"), "{}");
    await track(f, "project/extra.json");
    assert.equal((await f.run()).errors, 0);
    assert.equal((await f.run()).reused, true);
    await rename(resolve(f.directory, "extra.json"), resolve(f.directory, "renamed.json"));
    const renamed = await f.run();
    assert.equal(renamed.reused, false);
    assert.equal(renamed.errors, 0, renamed.output);
    git(f.workspace, "add", "--all");
    assert.equal((await f.run()).errors, 0);
    assert.equal((await f.run()).reused, true);
  });

  test("unstaged tracked deletion lints the valid remaining tree, including after staging", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    await writeFile(resolve(f.directory, "renamed.json"), "{}");
    await track(f, "project/renamed.json");
    assert.equal((await f.run()).errors, 0);
    assert.equal((await f.run()).reused, true);
    await rm(resolve(f.directory, "renamed.json"));
    const deleted = await f.run();
    assert.equal(deleted.reused, false);
    assert.equal(deleted.errors, 0, deleted.output);
    git(f.workspace, "add", "--all");
    assert.equal((await f.run()).reused, true);
  });

  test("deleting an indexed imported declaration before staging reruns lint on the unchanged consumer", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    await track(f, "dependency.ts");
    assert.equal((await f.run()).errors, 0);
    await rm(resolve(f.workspace, "dependency.ts"));
    const deleted = await f.run();
    assert.equal(await readFile(resolve(f.directory, "consumer.ts"), "utf8"), f.consumer);
    assert.equal(deleted.reused, false);
    assert.ok(deleted.errors > 0);
    assert.match(deleted.output, /no-unsafe-return/);
  });

  test("indexed input read errors other than absence fail closed", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    await writeFile(resolve(f.directory, "extra.json"), "{}");
    await track(f, "project/extra.json");
    assert.equal((await f.run()).errors, 0);
    await rm(resolve(f.directory, "extra.json"));
    await mkdir(resolve(f.directory, "extra.json"));
    await assert.rejects(f.run, { code: "EISDIR" });
  });

  test("Git enumeration fingerprints the fixture repository outside the TypeScript program", { timeout: 5000 }, async (t) => {
    const f = await fixture(t);
    await writeFile(resolve(f.workspace, "settings.json"), "{}");
    await track(f, "settings.json");
    assert.equal((await f.run()).errors, 0);
    assert.equal((await f.run()).reused, true);
    await writeFile(resolve(f.workspace, "settings.json"), '{"changed":true}');
    const changed = await f.run();
    assert.equal(changed.reused, false);
    assert.equal(changed.errors, 0, changed.output);
  });

  test("foreign Git discards every repository-local hook variable and preserves the caller environment", () => {
    const names = git(process.cwd(), "rev-parse", "--local-env-vars").split("\n");
    const inherited = { ...process.env, PICTOR_SENTINEL: "preserved" };
    for (const name of names) inherited[name] = "foreign-repository-must-not-use";
    const clean = foreignGitEnvironment(inherited);
    for (const name of names) {
      assert.equal(clean[name], undefined, name);
      assert.equal(inherited[name], "foreign-repository-must-not-use", name);
    }
    assert.equal(clean.PICTOR_SENTINEL, "preserved");
  });
});
