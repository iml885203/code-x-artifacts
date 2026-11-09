import { format, type Translate } from "../../i18n";
import type { Outcome } from "./rules";

/** What a refused change says, in the person's language, with the limit it broke. Null when the change was not refused. */
export function refusalText(t: Translate, outcome: Outcome): string | null {
  const why = outcome.kind === "refused" ? outcome.why : null;
  if (!why) return null;
  switch (why.rule) {
    case "pages":
      return format(t("structure.rulePages"), { limit: String(why.limit) });
    case "frames":
      return format(t("structure.ruleFrames"), { limit: String(why.limit) });
    case "name":
      return format(t("structure.ruleName"), { limit: String(why.limit) });
    case "side":
      return format(t("structure.ruleSide"), { field: t(why.field === "w" ? "structure.width" : "structure.height"), min: String(why.min), max: String(why.max) });
    case "lastPage":
      return t("structure.ruleLastPage");
    case "lastFrame":
      return t("structure.ruleLastFrame");
    case "missing":
      return t("structure.ruleMissing");
  }
}
