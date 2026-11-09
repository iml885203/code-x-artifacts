import "./board/assetPath";
import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import "./app.css";
import { App } from "./App";
import { ArtifactsProvider } from "./artifacts";
import { readBoot } from "./boot";
import { I18nProvider } from "./i18n";
import { RouterProvider } from "./router";
import { ToastProvider } from "./Toast";

const boot = readBoot(document);
const root = document.getElementById("root");
if (!root) throw new Error("the page has no #root");

// Rendered at once, not on React's next tick: a page that arrives through a
// cross-document view transition should not first show an empty frame.
flushSync(() => {
  createRoot(root).render(
    <I18nProvider strings={boot.strings} lang={boot.lang}>
      <RouterProvider initial={location.pathname + location.search}>
        <ArtifactsProvider>
          <ToastProvider>
            <App boot={boot} />
          </ToastProvider>
        </ArtifactsProvider>
      </RouterProvider>
    </I18nProvider>,
  );
});
