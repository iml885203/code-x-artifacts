import react from "@vitejs/plugin-react";
import { cpSync, mkdirSync } from "node:fs";
import { readFile } from "node:fs/promises";
import type { IncomingMessage, ServerResponse } from "node:http";
import { defineConfig, type Plugin } from "vitest/config";

// The Go server this dev server fronts: the API, /raw, /static and the form
// posts all come from it.
const pictor = process.env.PICTOR_URL ?? "http://127.0.0.1:8080";

// Builds land in the Go package that embeds them. emptyOutDir clears the folder
// first, so the placeholder that keeps the folder in git is re-emitted.
const outDir = "../internal/web/dist";
function keepPlaceholder(): Plugin {
  return {
    name: "keep-placeholder",
    generateBundle() {
      this.emitFile({ type: "asset", fileName: ".gitkeep", source: "" });
    },
  };
}

// Excalidraw (a board's editor) loads its fonts from EXCALIDRAW_ASSET_PATH
// (src/board/assetPath.ts); they are served with the bundle under
// /assets/excalidraw/, so nothing is fetched from another site.
function excalidrawFonts(): Plugin {
  return {
    name: "excalidraw-fonts",
    apply: "build",
    closeBundle() {
      cpSync("node_modules/@excalidraw/excalidraw/dist/prod/fonts", `${outDir}/assets/excalidraw/fonts`, { recursive: true, filter: (source) => !source.endsWith("/Liberation") });
      // Use the explicitly licensed OFL fallback instead of the legacy font binary.
      const fallback = `${outDir}/assets/excalidraw/fonts/Liberation`;
      mkdirSync(fallback, { recursive: true });
      cpSync("fonts/Arimo.ttf", `${fallback}/Arimo.ttf`);
      cpSync("fonts/OFL-Arimo.txt", `${fallback}/OFL-Arimo.txt`);
      cpSync("../docs/licenses/fonts", `${outDir}/assets/excalidraw/licenses`, { recursive: true });
    },
  };
}

// Excalidraw lists a CDN (esm.sh) after EXCALIDRAW_ASSET_PATH as a fallback
// for every font, and the page's CSP reports each one it blocks. The fallback is
// pointed at the site's own copy instead; a version whose code differs fails the
// build here rather than reaching a CDN.
const excalidrawFallback = /`https:\/\/esm\.sh\/\$\{(\w+)\.PKG_NAME\?`\$\{\1\.PKG_NAME\}@\$\{\1\.PKG_VERSION\}`:"@excalidraw\/excalidraw"\}\/dist\/prod\/`/;
function excalidrawOwnFallback(): Plugin {
  return {
    name: "excalidraw-own-fallback",
    transform(code, id) {
      if (!id.includes("@excalidraw/excalidraw/dist/prod/") || !code.includes("ASSETS_FALLBACK_URL")) return null;
      if (!excalidrawFallback.test(code)) this.error("Excalidraw's font fallback is not the one vite.config.ts rewrites; check the new version");
      return { code: code.replace(excalidrawFallback, '`${window.location.origin}/assets/excalidraw/`').replaceAll('LiberationSans-Regular.woff2', 'Arimo.ttf'), map: null };
    },
  };
}

// In development the shell is still rendered by Go, because only Go knows the
// viewer, their language, the strings and what the page shows. This asks Go for
// the same address with the browser's own cookie and language and takes the
// boot block it answers with; the scripts then come from Vite so edits hot-reload.
const shellPaths = /^\/(login|connect|a\/[^/]+)?$/;
function shellFromGo(): Plugin {
  return {
    name: "shell-from-go",
    configureServer(server) {
      const serve = async (req: IncomingMessage, res: ServerResponse) => {
        const headers: Record<string, string> = {};
        for (const h of ["cookie", "accept-language"] as const) {
          const v = req.headers[h];
          if (typeof v === "string") headers[h] = v;
        }
        const upstream = await fetch(pictor + (req.url ?? "/"), { headers, redirect: "manual" });
        if (upstream.status >= 300 && upstream.status < 400) {
          res.statusCode = upstream.status;
          res.setHeader("Location", upstream.headers.get("location") ?? "/login");
          res.end();
          return;
        }
        const html = await upstream.text();
        const boot = /<script type="application\/json" id="pictor-boot">([\s\S]*?)<\/script>/.exec(html)?.[1];
        const lang = /<html lang="([^"]+)"/.exec(html)?.[1] ?? "en";
        if (!boot) {
          res.statusCode = 502;
          res.end(`${pictor}${req.url ?? "/"} did not answer with the shell (status ${String(upstream.status)}). Run "make web" once so the Go server has a bundle to build the shell from.`);
          return;
        }
        const template = (await readFile("index.html", "utf8"))
          .replace(/<script type="application\/json" id="pictor-boot">[\s\S]*?<\/script>/, () => `<script type="application/json" id="pictor-boot">${boot}</script>`)
          .replace('<html lang="en">', `<html lang="${lang}">`);
        res.setHeader("Content-Type", "text/html; charset=utf-8");
        res.end(await server.transformIndexHtml("/", template));
      };
      server.middlewares.use((req: IncomingMessage, res: ServerResponse, next) => {
        if (req.method !== "GET" || !shellPaths.test(req.url?.split("?")[0] ?? "")) {
          next();
          return;
        }
        serve(req, res).catch(next);
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), keepPlaceholder(), shellFromGo(), excalidrawFonts(), excalidrawOwnFallback()],
  // Excalidraw checks it to choose between its React and Preact builds.
  define: { "process.env.IS_PREACT": JSON.stringify("false") },
  resolve: {
    // The editor imports the schema the Go bridge shares (internal/docroom/interop),
    // which has its own node_modules: one copy of each of these must be in the bundle.
    dedupe: ["prosemirror-model", "prosemirror-state", "prosemirror-view", "prosemirror-transform", "yjs", "lib0", "y-protocols"],
  },
  build: {
    outDir,
    emptyOutDir: true,
    manifest: true,
    target: "es2023",
  },
  server: {
    proxy: {
      "^/(api|raw|static|login|logout|lang|tokens|a|healthz)(/|$)": { target: pictor, changeOrigin: false },
    },
  },
  test: {
    // Fresh workers preserve file isolation without forked-process startup.
    pool: "threads",
    isolate: true,
    environment: "jsdom",
    setupFiles: ["src/setup.ts"],
    restoreMocks: true,
  },
});
