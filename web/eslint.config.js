import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import { defineConfig } from "eslint/config";
import tseslint from "typescript-eslint";

const htmlFromStrings = [
  { selector: 'AssignmentExpression[left.type="MemberExpression"][left.property.name=/^(innerHTML|outerHTML)$/]', message: "Do not assign innerHTML or outerHTML: build DOM from the schema." },
  { selector: 'AssignmentExpression[left.type="MemberExpression"][left.property.value=/^(innerHTML|outerHTML)$/]', message: "Do not assign innerHTML or outerHTML: build DOM from the schema." },
  { selector: 'CallExpression[callee.property.name=/^(insertAdjacentHTML|createContextualFragment)$/]', message: "Do not make DOM from a string of markup: build it from the schema." },
  { selector: 'CallExpression[callee.object.name="document"][callee.property.name=/^(write|writeln)$/]', message: "Do not use document.write." },
  { selector: 'JSXAttribute[name.name="dangerouslySetInnerHTML"]', message: "Do not use dangerouslySetInnerHTML: render the schema's nodes." },
  { selector: 'Property[key.name="dangerouslySetInnerHTML"]', message: "Do not use dangerouslySetInnerHTML: render the schema's nodes." },
];

export default defineConfig(
  { ignores: ["node_modules", "../internal/web/dist"] },
  js.configs.recommended,
  tseslint.configs.strictTypeChecked,
  tseslint.configs.stylisticTypeChecked,
  reactHooks.configs.flat.recommended,
  {
    languageOptions: {
      globals: globals.browser,
      parserOptions: { projectService: { allowDefaultProject: ["eslint.config.js"] }, tsconfigRootDir: import.meta.dirname },
    },
    rules: {
      // console is for errors a person should see in the browser's console.
      "no-console": ["error", { allow: ["error", "warn"] }],
      // A modal Radix menu locks scroll by injecting a <style> element, which
      // the site CSP (default-src 'self') blocks and logs on every open.
      "no-restricted-imports": [
        "error",
        { patterns: [{ group: ["@radix-ui/react-alert-dialog", "@radix-ui/react-dialog"], message: "Radix's modal dialogs inject an inline <style> the CSP blocks; use <dialog> and showModal() (see ConfirmDialog, #46)." }] },
      ],
      "no-restricted-syntax": [
        "error",
        {
          selector: 'JSXOpeningElement[name.object.name="DropdownMenu"][name.property.name="Root"]:not(:has(JSXAttribute[name.name="modal"]))',
          message: "Write <DropdownMenu.Root modal={false}>: the modal menu injects an inline <style> the CSP blocks (#46).",
        },
        // Markup built from a string is how a pasted or stored string becomes script: the editor and the
        // renderer make DOM only from the validated schema (docs/editor.md). lint.test.ts holds this list to its word.
        ...htmlFromStrings,
      ],
    },
  },
  {
    files: ["vite.config.ts", "eslint.config.js"],
    languageOptions: { globals: globals.node },
  },
);
