import foxglove from "@foxglove/eslint-plugin";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: ["**/dist"],
  },
  ...foxglove.configs.base,
  ...foxglove.configs.typescript.map((config) => ({
    ...config,
    files: ["**/*.ts"],
  })),
  {
    files: ["**/*.ts"],
    languageOptions: {
      parserOptions: {
        project: "tsconfig.eslint.json",
      },
    },
  },
);
