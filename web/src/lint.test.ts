import { ESLint, Linter } from "eslint";
import { describe, expect, it } from "vitest";

// The rule that bans building DOM from strings, taken from the real config
// (eslint.config.js) and run on code that does it.
async function lintWithTheRealRule(code: string): Promise<string[]> {
  const config = (await new ESLint({ cwd: process.cwd() }).calculateConfigForFile("src/editor/DocEditor.tsx")) as { rules?: Record<string, unknown> };
  const rule = config.rules?.["no-restricted-syntax"];
  expect(rule, "no-restricted-syntax is configured for the editor").toBeDefined();
  const messages = new Linter().verify(code, [
    { files: ["**/*.jsx"], languageOptions: { ecmaVersion: "latest", sourceType: "module", parserOptions: { ecmaFeatures: { jsx: true } } }, rules: { "no-restricted-syntax": rule as never } },
  ], "bad.jsx");
  return messages.map((m) => m.message);
}

describe("the lint gate on markup built from strings", () => {
  it.each([
    ["innerHTML", "el.innerHTML = value;"],
    ["innerHTML with brackets", 'el["innerHTML"] = value;'],
    ["outerHTML", "el.outerHTML = value;"],
    ["insertAdjacentHTML", 'el.insertAdjacentHTML("beforeend", value);'],
    ["createContextualFragment", "range.createContextualFragment(value);"],
    ["document.write", "document.write(value);"],
    ["dangerouslySetInnerHTML", "const x = <div dangerouslySetInnerHTML={{ __html: value }} />;"],
    ["dangerouslySetInnerHTML as a prop object", "const props = { dangerouslySetInnerHTML: { __html: value } };"],
  ])("fails on %s", async (_name, code) => {
    expect(await lintWithTheRealRule(code)).not.toEqual([]);
  });

  it("lets DOM be built the way the editor builds it", async () => {
    expect(await lintWithTheRealRule('const el = document.createElement("span"); el.textContent = value; el.append(other);')).toEqual([]);
  });
});
