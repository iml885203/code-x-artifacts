// The horizontal rule.
import { RuleIcon } from "../../icons/blocks";
import { insertRule } from "../format";
import { button, type Feature } from "../feature";

export const rule: Feature = {
  id: "rule",
  tools: [button({ id: "rule", group: "blocks", label: "editor.rule", face: <RuleIcon />, run: (ctx) => { ctx.run(insertRule); } })],
};
