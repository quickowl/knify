import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const siteAPIBaseURL = env.SITE_API_BASE_URL || "https://knify.dev";
  const directHubBaseURL = env.SITE_HUB_BASE_URL || env.VITE_AGENTCANVAS_HUB_URL || "";
  const directHubToken = env.SITE_HUB_TOKEN || env.VITE_AGENTCANVAS_HUB_TOKEN || env.HUB_TOKEN || "";
  const apiTarget = directHubBaseURL || siteAPIBaseURL;

  return {
    plugins: [react()],
    server: {
      proxy: {
        "/api/hub": {
          target: apiTarget,
          changeOrigin: true,
          rewrite: directHubBaseURL ? (path) => path.replace(/^\/api\/hub/, "/v1") : undefined,
          headers: directHubBaseURL && directHubToken ? { Authorization: `Bearer ${directHubToken}` } : undefined,
        },
      },
    },
  };
});
