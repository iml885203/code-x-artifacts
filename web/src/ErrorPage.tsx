import type { Boot } from "./boot";
import { Easel } from "./Easel";
import { Header } from "./Header";
import { useT } from "./i18n";
import { useDocumentTitle } from "./title";

/** The page for something that is not there. A missing artifact and one the viewer may not see look the same, so this says only what is true of both. */
export function ErrorPage({ boot }: { boot: Boot }) {
  const t = useT();
  useDocumentTitle(t("error.notFound.title"));
  return (
    <>
      <Header boot={boot} />
      <main className="notice">
        <Easel lit={false} />
        <h1>{t("error.notFound.title")}</h1>
        <p>{t("error.notFound.hint")}</p>
        <a className="btn" href="/">
          {t("nav.backToArtifacts")}
        </a>
      </main>
    </>
  );
}
