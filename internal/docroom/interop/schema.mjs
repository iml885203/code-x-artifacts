// The editor's document schema: the ProseMirror nodes and marks a doc is made
// of while it is being edited. The editor (R4) imports this file; the Go bridge
// in internal/docroom reads and writes what y-prosemirror stores for it, and
// the fixtures hold the two together (docs/doc-room.md).
//
// A doc is tabs of blocks. Block ids are attributes of the elements: the
// editor's id plugin keeps them, and the Go side keeps them across reads and
// writes. Parts the model has no id for (the paragraph of a list item or a
// table cell, a nested list) have the attribute too and leave it null.
import { Schema } from 'prosemirror-model'

const id = { default: null }

export const schema = new Schema({
  nodes: {
    doc: { content: 'tab+' },
    tab: { content: 'block*', attrs: { id, title: { default: '' } } },

    paragraph: { content: 'inline*', group: 'block content_block', attrs: { id } },
    heading: { content: 'inline*', group: 'block content_block', attrs: { id, level: { default: 1 } }, defining: true },
    bullet_list: { content: 'list_item+', group: 'block content_block', attrs: { id } },
    numbered_list: { content: 'list_item+', group: 'block content_block', attrs: { id, start: { default: 1 } } },
    task_list: { content: 'list_item+', group: 'block content_block', attrs: { id } },
    // The text of the item, then the items nested under it.
    list_item: { content: 'paragraph (bullet_list | numbered_list | task_list)?', attrs: { checked: { default: null } }, defining: true },
    quote: { content: 'content_block+', group: 'block content_block', attrs: { id }, defining: true },
    code_block: { content: 'text*', marks: '', code: true, group: 'block content_block', attrs: { id, lang: { default: null } }, defining: true },
    rule: { group: 'block content_block', attrs: { id } },
    table: { content: 'table_row+', group: 'block content_block', attrs: { id, header: { default: false } }, isolating: true },
    table_row: { content: 'table_cell+' },
    table_cell: { content: 'paragraph', attrs: { align: { default: null } }, isolating: true },
    image: { group: 'block content_block', attrs: { id, src: {}, alt: { default: null }, size: { default: null } }, draggable: true },
    // A placeholder an agent left for something it will write later.
    pending: { group: 'block', attrs: { id, intent: { default: '' } } },

    text: { group: 'inline' },
    hard_break: { inline: true, group: 'inline', selectable: false },
    // A person named in the text: the account and the name it had then. It has no
    // marks (docs/doc-format.md); the editor's mention feature keeps it so.
    dropdown: { inline: true, group: 'inline', atom: true, selectable: true, marks: '', attrs: { config: {} } },
    date: { inline: true, group: 'inline', atom: true, selectable: true, marks: '', attrs: { date: {} } },
    mention: { inline: true, group: 'inline', atom: true, selectable: true, attrs: { user: {}, name: {} } }
  },
  // Marks are listed in the order the Go side reads them back in.
  marks: {
    bold: {},
    italic: {},
    underline: {},
    strike: {},
    code: {},
    highlight: {},
    link: { attrs: { href: {} }, inclusive: false }
  }
})
