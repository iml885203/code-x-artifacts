// The schema the Go bridge shares with the editor is plain JavaScript in the
// docroom interop package; this is its type.
declare module "*/interop/schema.mjs" {
  import type { Schema } from "prosemirror-model";
  export const schema: Schema;
}
