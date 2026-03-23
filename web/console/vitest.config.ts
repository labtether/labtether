import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "jsdom",
    include: [
      "app/**/__tests__/**/*.test.{ts,tsx}",
      "lib/__tests__/**/*.test.{ts,tsx}",
    ],
  },
});
