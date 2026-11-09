import { useState } from "react";
import { useT } from "../../i18n";
import type { DesignEdits } from "../edits";
import { setToken, useTokens } from "../tokens";
import { Field, Section } from "./controls";
import "./tokens.css";

/** Shared theme values commit once, in the same undo history as element edits. */
export function TokenSection({ edits }: { edits: DesignEdits }) {
  const t = useT();
  const tokens = useTokens(edits);
  const [refused, setRefused] = useState(false);
  const names = Object.keys(tokens).sort();
  if (!names.length) return null;
  return <div className="design-tokens"><Section title={t("props.tokens")}>
    <p className="code-note">{t("props.tokensShared")}</p>
    {refused && <p className="error" role="alert">{t("props.invalid")}</p>}
    {names.map((name) => <Field key={name} wide short={name} label={name} value={tokens[name] ?? ""} onCommit={(value) => {
      const accepted = setToken(edits, name, value);
      setRefused(!accepted);
      return accepted;
    }} />)}
  </Section></div>;
}
