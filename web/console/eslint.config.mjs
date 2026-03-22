import tsParser from "@typescript-eslint/parser";
import reactHooks from "eslint-plugin-react-hooks";

const MUTED_TEXT_TOKENS = [
  "text-[var(--muted)]",
  "text-[var(--control-fg-muted)]",
];

const ACTIVE_TEXT_TOKENS = [
  "text-[var(--accent-contrast)]",
  "text-[var(--control-fg-active)]",
];

function collectStringLiterals(node, acc) {
  if (!node) return;

  if (node.type === "Literal" && typeof node.value === "string") {
    acc.push(node.value);
    return;
  }

  if (node.type === "TemplateLiteral") {
    for (const quasi of node.quasis) {
      if (quasi.value.cooked) acc.push(quasi.value.cooked);
    }
    for (const expr of node.expressions) {
      collectStringLiterals(expr, acc);
    }
    return;
  }

  if (node.type === "BinaryExpression" || node.type === "LogicalExpression") {
    collectStringLiterals(node.left, acc);
    collectStringLiterals(node.right, acc);
    return;
  }

  if (node.type === "ConditionalExpression") {
    collectStringLiterals(node.consequent, acc);
    collectStringLiterals(node.alternate, acc);
  }
}

function hasAnyToken(value, tokens) {
  return tokens.some((token) => value.includes(token));
}

const noConflictingControlTextRule = {
  meta: {
    type: "problem",
    docs: {
      description: "Disallow className templates that mix base muted text with conditional active text.",
    },
    schema: [],
    messages: {
      conflict:
        "Avoid base muted text classes with conditional active text classes in the same template. Use exclusive branches or SegmentedTabs.",
    },
  },
  create(context) {
    return {
      JSXAttribute(node) {
        if (node.name.type !== "JSXIdentifier" || node.name.name !== "className") return;
        if (!node.value || node.value.type !== "JSXExpressionContainer") return;
        if (node.value.expression.type !== "TemplateLiteral") return;

        const template = node.value.expression;
        const baseStatic = template.quasis.map((q) => q.value.cooked ?? "").join(" ");
        if (!hasAnyToken(baseStatic, MUTED_TEXT_TOKENS)) return;

        const stringLiterals = [];
        for (const expression of template.expressions) {
          if (expression.type !== "ConditionalExpression") continue;
          collectStringLiterals(expression, stringLiterals);
        }
        if (stringLiterals.length === 0) return;

        if (stringLiterals.some((literal) => hasAnyToken(literal, ACTIVE_TEXT_TOKENS))) {
          context.report({ node, messageId: "conflict" });
        }
      },
    };
  },
};

export default [
  {
    ignores: [
      "node_modules/**",
      ".next/**",
      "playwright-report/**",
      "test-results/**",
      "e2e/**",
    ],
  },
  {
    files: ["app/**/*.{ts,tsx}"],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaVersion: "latest",
        sourceType: "module",
        ecmaFeatures: { jsx: true },
      },
    },
    plugins: {
      "react-hooks": reactHooks,
      labtether: {
        rules: {
          "no-conflicting-control-text": noConflictingControlTextRule,
        },
      },
    },
    rules: {
      "react-hooks/exhaustive-deps": "error",
      "react-hooks/rules-of-hooks": "error",
      "labtether/no-conflicting-control-text": "error",
    },
  },
];
