// Every feature of the Docs editor, in the order the toolbar shows their tools.
// A new feature is a module in features/ and one line here.
import type { Feature } from "./feature";
import { insertion } from "./features/insertion";
import { blockType } from "./features/blockType";
import { date } from "./features/date";
import { dropdown } from "./features/dropdown";
import { format } from "./features/format";
import { hint } from "./features/hint";
import { history } from "./features/history";
import { image } from "./features/image";
import { link } from "./features/link";
import { lists } from "./features/lists";
import { mention } from "./features/mention";
import { rule } from "./features/rule";
import { table } from "./features/table";

export const features: Feature[] = [insertion, date, dropdown, blockType, format, table, image, lists, link, rule, mention, history, hint];
