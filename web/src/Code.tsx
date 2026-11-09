import { CopyButton } from "./Copy";
import { useT } from "./i18n";

/** A block of text to paste somewhere, with its own copy button. */
export function Code({ text }: { text: string }) {
  const t = useT();
  return (
    <div className="code">
      <pre>{text}</pre>
      <CopyButton className="quiet" text={() => text}>
        {t("copy.label")}
      </CopyButton>
    </div>
  );
}
