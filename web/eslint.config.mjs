import { defineConfig, globalIgnores } from "eslint/config";
import { fixupConfigRules } from "@eslint/compat";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTypescript from "eslint-config-next/typescript";

export default defineConfig([
  // Next's bundled React/import rules still use context APIs removed in ESLint 10.
  ...fixupConfigRules([...nextVitals, ...nextTypescript]),
  globalIgnores([".next/**", "node_modules/**", "next-env.d.ts"]),
]);
