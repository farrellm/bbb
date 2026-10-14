import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";
import { defineConfig } from "vitest/config";

// The Go server listens here in development (LISTEN_ADDR).
const api = "http://127.0.0.1:8094";

export default defineConfig({
  plugins: [
    react(),
    // Builds src/sw.ts into /sw.js with the app shell's precache list.
    VitePWA({
      strategies: "injectManifest",
      srcDir: "src",
      filename: "sw.ts",
      registerType: "autoUpdate",
      // main.tsx registers it through virtual:pwa-register.
      injectRegister: false,
      // public/manifest.webmanifest is written by hand.
      manifest: false,
      injectManifest: {
        globPatterns: ["**/*.{js,css,html,svg,png,woff2,webmanifest}"],
      },
      // A worker in development too, so push can be tried on localhost.
      devOptions: { enabled: true, type: "module" },
    }),
  ],
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
