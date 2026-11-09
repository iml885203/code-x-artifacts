// Tables: Insert table, and the row of tools that shows while the caret is in one.
import type { EditorState } from "prosemirror-state";
import type { Key } from "../../i18n";
import { TableIcon } from "../../icons/table";
import { button, type Feature } from "../feature";
import { keepCellsApart, maxCells, maxColumns, nextCell, previousCell, tableCommand, tableInfo, type TableInfo, type TableTool } from "../table";

const tableTools: { tool: TableTool; label: Key }[] = [
  { tool: "rowAbove", label: "editor.rowAbove" },
  { tool: "rowBelow", label: "editor.rowBelow" },
  { tool: "columnLeft", label: "editor.columnLeft" },
  { tool: "columnRight", label: "editor.columnRight" },
  { tool: "deleteRow", label: "editor.deleteRow" },
  { tool: "deleteColumn", label: "editor.deleteColumn" },
  { tool: "deleteTable", label: "editor.deleteTable" },
  { tool: "header", label: "editor.headerRow" },
  { tool: "left", label: "editor.alignLeft" },
  { tool: "center", label: "editor.alignCenter" },
  { tool: "right", label: "editor.alignRight" },
];

const pressed = (table: TableInfo, tool: TableTool): boolean | undefined =>
  tool === "header" ? table.header : tool === table.align ? true : tool === "left" || tool === "center" || tool === "right" ? false : undefined;

function disabled(table: TableInfo, tool: TableTool): boolean {
  if (tool === "deleteRow") return table.rows < 2;
  if (tool === "deleteColumn") return table.columns < 2;
  if (tool === "rowAbove" || tool === "rowBelow") return (table.rows + 1) * table.columns > maxCells;
  if (tool === "columnLeft" || tool === "columnRight") return table.columns >= maxColumns || table.rows * (table.columns + 1) > maxCells;
  return false;
}

export const table: Feature = {
  id: "table",
  tools: [
    button({
      id: "table-insert",
      group: "insert",
      label: "editor.tableInsert",
      face: <TableIcon />,
      disabled: (state: EditorState) => tableInfo(state) !== null,
      run: (ctx) => {
        ctx.run(tableCommand("insert"));
      },
    }),
  ],
  rows: [
    {
      id: "table-tools",
      render: (state, ctx, t) => {
        const info = tableInfo(state);
        if (info === null) return null;
        return (
          <div className="ed-toolbar ed-table-tools" role="toolbar" aria-label={t("editor.tableTools")}>
            {tableTools.map(({ tool, label }) => (
              <button
                key={tool}
                type="button"
                className="quiet"
                aria-pressed={pressed(info, tool)}
                disabled={disabled(info, tool)}
                onClick={() => {
                  ctx.run(tableCommand(tool));
                }}
              >
                {t(label)}
              </button>
            ))}
          </div>
        );
      },
    },
  ],
  plugins: () => [keepCellsApart],
  keymap: () => ({ Tab: nextCell, "Shift-Tab": previousCell }),
};
