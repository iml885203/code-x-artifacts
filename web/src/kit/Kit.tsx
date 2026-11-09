import type { Boot } from "../boot";
import { Header } from "../Header";
import { useT } from "../i18n";
import { useDocumentTitle } from "../title";

const stylesheet = "/kit/v4/pictor.css";
const sample = "/kit/v4/sample.html";

/**
 * The kit's reference: the sample page in each theme, side by side. The frames
 * carry the empty sandbox, so the kit and its fonts reach them the way they
 * reach an artifact: from an origin that is null.
 */
export function Kit({ boot }: { boot: Boot }) {
  const t = useT();
  useDocumentTitle(t("kit.title"));
  return (
    <>
      <Header boot={boot} />
      <main className="page kit">
        <h1>{t("kit.title")}</h1>
        <p className="lead">{t("kit.intro")}</p>
        <pre>{`<link rel="stylesheet" href="${stylesheet}">`}</pre>
        <p>
          <a href={stylesheet}>{t("kit.stylesheet")}</a> · <a href={sample} target="_blank" rel="noopener">{t("kit.openSample")}</a>
        </p>
        <div className="kit-frames">
          <Frame theme="light" title={t("kit.light")} />
          <Frame theme="dark" title={t("kit.dark")} />
        </div>
      </main>
    </>
  );
}

function Frame({ theme, title }: { theme: "light" | "dark"; title: string }) {
  return (
    <section>
      <h2>{title}</h2>
      <iframe sandbox="" title={title} src={`${sample}?theme=${theme}`} />
    </section>
  );
}
