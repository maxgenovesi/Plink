import { defineConfig } from "vite";

// In dev, Vite serves the client on :5173 and forwards WebSocket traffic to
// the Go server on :8080, so the client can always connect to a relative /ws.
export default defineConfig({
  server: {
    proxy: {
      "/ws": {
        target: "http://localhost:8080",
        ws: true,
      },
    },
  },
});
