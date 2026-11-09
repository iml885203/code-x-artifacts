// Mentions: @ and a colleague's name makes a reference to that person in the text (docs/doc-format.md). It is
// a node of the shared schema; this feature is the way to type one and the rule that keeps it bare.
import type { Feature } from "../../feature";
import { suggest } from "./suggest";
import { bareMentions } from "./trigger";

export const mention: Feature = {
  id: "mention",
  plugins: (hooks) => [bareMentions, suggest(hooks)],
};

export { holds } from "./trigger";
