import { readdirSync } from "node:fs";
import { posix } from "node:path";
import { defineConfig } from "oxlint";

// 実行のたびにディレクトリを調べ、新しい機能にも同じ依存関係の制約を適用する
const root = new URL("./", import.meta.url);

function directories(path: string): string[] {
  return readdirSync(new URL(path, root), { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => `${path}/${entry.name}`)
    .sort();
}

function descendants(path: string): string[] {
  return [path, ...directories(path).flatMap(descendants)];
}

const features = directories("src/features");

const escapeRegex = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// @エイリアスと、参照元ディレクトリからの相対パスの両方を検査する
function restriction(directory: string, target: string, message: string) {
  const relative = posix.relative(directory, target);
  const source = relative.startsWith(".") ? relative : `./${relative}`;
  return {
    regex: `^(?:${escapeRegex(target.replace(/^src\//, "@/"))}|${escapeRegex(source)})(?:/|$)`,
    message,
    allowTypeImports: false,
  };
}

const boundaries: NonNullable<Parameters<typeof defineConfig>[0]["overrides"]> = descendants(
  "src",
).flatMap((directory) => {
  const feature = features.find((path) => directory === path || directory.startsWith(`${path}/`));

  const isUi =
    feature !== undefined &&
    (directory === `${feature}/ui` || directory.startsWith(`${feature}/ui/`));

  const isShared = /^(src\/lib|src\/components)(\/|$)/.test(directory);
  const patterns = [];
  if (feature) {
    for (const other of features.filter((path) => path !== feature)) {
      patterns.push(
        restriction(
          directory,
          other,
          "他機能の型・処理は直接参照できません。Propsで渡すか、共通責務の配置を検討してください。",
        ),
      );
    }
  }
  if (feature || isShared) {
    patterns.push(
      restriction(
        directory,
        "src/routes",
        "routesへの逆参照は禁止です。必要な値は引数やPropsで渡してください。",
      ),
    );
  }
  if (isShared) {
    patterns.push(
      restriction(
        directory,
        "src/features",
        "共通部品から業務機能は参照できません。必要な値は引数やPropsで渡してください。",
      ),
    );
  }
  if (isUi && feature) {
    patterns.push(
      restriction(
        directory,
        `${feature}/api`,
        "UIはmodelのquery・mutationを利用してください。apiの直接参照は禁止です。",
      ),
    );
    patterns.push(
      restriction(
        directory,
        "src/lib/http",
        "UIはmodelを経由してください。HTTPクライアントの直接参照は禁止です。",
      ),
    );
    patterns.push({
      regex: "^axios(?:/|$)",
      message: "通信はapi層へ移し、UIはmodelを経由してください。",
      allowTypeImports: false,
    });
  }
  if (patterns.length === 0) return [];
  return [
    {
      files: [`${directory}/*.{ts,tsx,js,jsx,mts,mjs,cts,cjs}`],
      rules: {
        "eslint/no-restricted-imports": ["error", { patterns }],
        ...(isUi
          ? {
              "eslint/no-restricted-globals": [
                "error",
                {
                  globals: [
                    {
                      name: "fetch",
                      message: "UIで直接通信せず、model経由でapi層を利用してください。",
                    },
                    {
                      name: "XMLHttpRequest",
                      message: "UIで直接通信せず、model経由でapi層を利用してください。",
                    },
                  ],
                  checkGlobalObject: true,
                },
              ],
            }
          : {}),
      },
    },
  ];
});

const base = defineConfig({
  plugins: ["typescript", "unicorn", "oxc", "react", "import", "promise"],
  jsPlugins: ["@stylistic/eslint-plugin"],
  categories: {
    correctness: "error",
  },
  options: {
    typeAware: true,
  },
  rules: {
    "@stylistic/padding-line-between-statements": [
      "error",
      {
        blankLine: "always",
        prev: "*",
        next: {
          selector:
            "FunctionDeclaration, ExportNamedDeclaration[declaration.type='FunctionDeclaration'], ExportDefaultDeclaration[declaration.type='FunctionDeclaration']",
        },
      },
      {
        blankLine: "always",
        prev: {
          selector:
            "FunctionDeclaration, ExportNamedDeclaration[declaration.type='FunctionDeclaration'], ExportDefaultDeclaration[declaration.type='FunctionDeclaration']",
        },
        next: "*",
      },
      {
        blankLine: "always",
        prev: "*",
        next: {
          selector:
            "VariableDeclaration, ExportNamedDeclaration[declaration.type='VariableDeclaration']",
          lineMode: "multiline",
        },
      },
      {
        blankLine: "always",
        prev: {
          selector:
            "VariableDeclaration, ExportNamedDeclaration[declaration.type='VariableDeclaration']",
          lineMode: "multiline",
        },
        next: "*",
      },
    ],
    "import/no-cycle": "error",
    "import/no-self-import": "error",
    "promise/no-multiple-resolved": "error",
    "promise/no-return-in-finally": "error",
    "promise/valid-params": "error",
    "react/rules-of-hooks": "error",
    "react/exhaustive-deps": "error",
    "typescript/no-explicit-any": "error",
    "typescript/ban-ts-comment": [
      "error",
      {
        "ts-ignore": true,
        "ts-nocheck": true,
        "ts-expect-error": "allow-with-description",
      },
    ],
    "typescript/no-floating-promises": [
      "error",
      {
        ignoreVoid: false,
      },
    ],
    "typescript/no-misused-promises": "error",
    "typescript/no-unsafe-assignment": "error",
    "typescript/switch-exhaustiveness-check": [
      "error",
      { considerDefaultExhaustiveForUnions: false },
    ],
  },
  overrides: [
    {
      files: ["**/*.test.ts", "**/*.test.tsx"],
      plugins: ["typescript", "unicorn", "oxc", "react", "import", "promise", "vitest"],
      rules: {
        "vitest/no-focused-tests": "error",
        "vitest/valid-expect": "error",
      },
    },
  ],
  ignorePatterns: ["dist/**", ".tanstack/**", "src/routeTree.gen.ts"],
  env: {
    builtin: true,
  },
});

export default defineConfig({
  ...base,
  overrides: [...(base.overrides ?? []), ...boundaries],
});
