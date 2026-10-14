import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The Go server listens here in development (LISTEN_ADDR).
const api = "http://127.0.0.1:8094";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    // Same-origin /api keeps the session cookie first-party, as it is in
    // production where Go serves both.
    proxy: { "/api": { target: api } },
  },
  build: { sourcemap: true },
  test: {
    include: ["src/**/*.test.ts"],
    coverage: { include: ["src/lib/**/*.ts"], reporter: ["text-summary"] },
  },
});
