import { defineConfig, globalIgnores } from "eslint/config";
import betterTailwindcss from "eslint-plugin-better-tailwindcss";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  {
    files: ["**/*.{ts,tsx}"],
    plugins: { "better-tailwindcss": betterTailwindcss },
    settings: {
      "better-tailwindcss": { entryPoint: "app/globals.css" },
    },
    rules: {
      "better-tailwindcss/enforce-canonical-classes": [
        "warn",
        {
          collapse: false,
          rootFontSize: 16,
          // Keep these fixed radii independent of the theme and root font size.
          ignore: ["^rounded-\\[2px\\]$", "^rounded-\\[4px\\]$"],
        },
      ],
      "no-restricted-syntax": [
        "error",
        {
          selector: 'ImportDeclaration[source.value="react"] > ImportSpecifier[imported.name="FormEvent"]',
          message: "Use React's SubmitEvent<HTMLFormElement> for form submissions, or the event type specific to the handler.",
        },
        {
          selector: 'JSXOpeningElement[name.name="select"]',
          message: "Use the shared shadcn Select from @/components/ui/select.",
        },
        {
          selector: 'JSXOpeningElement[name.name=/^(input|Input)$/] > JSXAttribute[name.name="type"][value.value=/^(date|datetime-local)$/]',
          message: "Use DatePicker from @/components/date-picker for calendar fields.",
        },
      ],
    },
  },
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",

    "components/ui/",
  ]),
]);

export default eslintConfig;
