import js from "@eslint/js";
import globals from "globals";
import boundaries from "eslint-plugin-boundaries";
import reactHooks from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";
import { defineConfig, globalIgnores } from "eslint/config";

// Layers and their allowed dependencies: docs/08-frontend-architektura.md, 2.
//   app → layout → features → shared; another feature only through its index.ts.
const layers = [
  { type: "app", pattern: "src/app" },
  { type: "layout", pattern: "src/layout" },
  { type: "feature", pattern: "src/features/*", capture: ["feature"] },
  { type: "shared", pattern: "src/shared" },
];

const element = (type) => ({ element: { type } });
// A feature from outside it: only its public surface (index.ts).
const featureEntry = { element: { type: "feature", fileInternalPath: "index.ts" } };

const layerPolicies = [
  { from: element("app"), allow: { to: { element: { types: { anyOf: ["app", "layout", "shared"] } } } } },
  { from: element("app"), allow: { to: featureEntry } },
  { from: element("layout"), allow: { to: { element: { types: { anyOf: ["layout", "shared"] } } } } },
  { from: element("layout"), allow: { to: featureEntry } },
  { from: element("feature"), allow: { to: element("shared") } },
  { from: element("feature"), allow: { to: featureEntry } },
  // Inside one feature, its own files may import each other freely.
  { from: element("feature"), allow: { to: element("feature"), dependency: { relationship: { to: "internal" } } } },
  { from: element("shared"), allow: { to: element("shared") } },
];

export default defineConfig([
  globalIgnores(["dist", "node_modules", "src/shared/api/schema.d.ts"]),
  {
    files: ["src/**/*.{ts,tsx}"],
    extends: [js.configs.recommended, tseslint.configs.recommended, reactHooks.configs.flat.recommended],
    plugins: { boundaries },
    languageOptions: { globals: globals.browser },
    settings: {
      "boundaries/elements": layers,
      "boundaries/include": ["src/**/*"],
      "import/resolver": { typescript: { project: "./tsconfig.json" } },
    },
    rules: {
      "boundaries/dependencies": ["error", { default: "disallow", policies: layerPolicies }],
      // Every source file belongs to a layer.
      "boundaries/no-unknown-files": "error",
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
          message: "dangerouslySetInnerHTML is forbidden: render markdown through shared/lib (docs/08, 10.).",
        },
        {
          selector: "MemberExpression[property.name=/^(innerHTML|outerHTML)$/]",
          message: "innerHTML/outerHTML is forbidden (docs/08, 10.).",
        },
        {
          selector: "CallExpression[callee.object.name='crypto'][callee.property.name='randomUUID']",
          message: "crypto.randomUUID does not exist on plain HTTP (Tailscale): use randomId from shared/lib.",
        },
      ],
      "no-restricted-imports": [
        "error",
        { patterns: [{ group: ["../../*"], message: "Use the @/ alias instead of ../../ (docs/08, 11.)." }] },
      ],
    },
  },
  {
    files: ["vite.config.ts"],
    extends: [js.configs.recommended, tseslint.configs.recommended],
  },
  {
    files: ["sw/**/*.js"],
    extends: [js.configs.recommended],
    languageOptions: { globals: globals.serviceworker },
  },
  {
    files: ["scripts/**/*.mjs", "eslint.config.js"],
    extends: [js.configs.recommended],
    languageOptions: { globals: globals.node },
  },
]);
