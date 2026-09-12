import { tanstackRouter } from "@tanstack/router-plugin/vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [tanstackRouter({ autoCodeSplitting: true }), react(), tailwindcss()],
  resolve: {
    alias: { "@": new URL("./src", import.meta.url).pathname },
  },
  test: {
    environment: "jsdom",
  },
});
