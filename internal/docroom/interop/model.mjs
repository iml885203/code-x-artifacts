// The thin layer between the stored blocks (docs/doc-format.md) and the
// editor's schema, in both directions. It is the editor's half of what the Go
// bridge does in internal/docroom, and the fixtures make the two agree.
import { schema } from './schema.mjs'

const text = (run) => {
  const marks = (run.marks ?? []).map((name) => (name === 'link' ? schema.marks.link.create({ href: run.href }) : schema.marks[name].create()))
  return schema.text(run.text, marks)
}

// A run with a newline is text, a hard_break, text; a mention is its own node.
const inlines = (runs = []) =>
  runs.flatMap((run) =>
    run.dropdown !== undefined
      ? [schema.nodes.dropdown.create({ config: JSON.stringify(run.dropdown) })]
      : run.date !== undefined
      ? [schema.nodes.date.create({ date: run.date })]
      : run.user !== undefined
      ? [schema.nodes.mention.create({ user: run.user, name: run.name })]
      : run.text.split('\n').flatMap((part, i) => [...(i > 0 ? [schema.nodes.hard_break.create()] : []), ...(part === '' ? [] : [text({ ...run, text: part })])])
  )

const paragraph = (runs) => schema.nodes.paragraph.create(null, inlines(runs))

const items = (type, list) =>
  list.map((item) => {
    const attrs = type === 'task_list' ? { checked: item.checked ?? false } : null
    const nested = item.items?.length ? [schema.nodes[type].create(null, items(type, item.items))] : []
    return schema.nodes.list_item.create(attrs, [paragraph(item.content), ...nested])
  })

const block = (b) => {
  const attrs = { id: b.id }
  switch (b.type) {
    case 'paragraph':
      return schema.nodes.paragraph.create(attrs, inlines(b.content))
    case 'heading':
      return schema.nodes.heading.create({ ...attrs, level: b.level }, inlines(b.content))
    case 'bullet_list':
    case 'task_list':
      return schema.nodes[b.type].create(attrs, items(b.type, b.items))
    case 'numbered_list':
      return schema.nodes.numbered_list.create({ ...attrs, start: b.start ?? 1 }, items(b.type, b.items))
    case 'quote':
      return schema.nodes.quote.create(attrs, b.blocks.map(block))
    case 'code':
      return schema.nodes.code_block.create({ ...attrs, lang: b.lang ?? null }, b.text ? [schema.text(b.text)] : [])
    case 'rule':
      return schema.nodes.rule.create(attrs)
    case 'table':
      return schema.nodes.table.create(
        { ...attrs, header: b.header ?? false },
        b.rows.map((row) =>
          schema.nodes.table_row.create(
            null,
            row.cells.map((cell) => schema.nodes.table_cell.create({ align: cell.align ?? null }, [paragraph(cell.content)]))
          )
        )
      )
    case 'image':
      return schema.nodes.image.create({ ...attrs, src: b.src, alt: b.alt ?? null, size: b.size ?? null })
    case 'pending':
      return schema.nodes.pending.create({ ...attrs, intent: b.intent })
    default:
      throw new Error(`unknown block type ${b.type}`)
  }
}

// One tab as the editor holds it: a document of that tab alone, which is what
// each tab's fragment in a room is (docs/doc-room.md).
export const tabToProseMirror = (tab) => schema.nodes.doc.create(null, [schema.nodes.tab.create({ id: tab.id, title: tab.title }, tab.blocks.map(block))])

const sameRun = (a, b) => JSON.stringify([a.marks, a.href]) === JSON.stringify([b.marks, b.href])

// A break takes the marks of the text before it, or of the text after it when
// it starts the line: it has no glyph, and the room stores no marks for it.
const runsOf = (node) => {
  const runs = []
  const breaks = []
  node.forEach((child) => {
    if (child.type.name === 'hard_break') {
      breaks.push(runs.length)
      runs.push({ text: '\n' })
      return
    }
    if (child.type.name === 'dropdown') {
      runs.push({ dropdown: JSON.parse(child.attrs.config) })
      return
    }
    if (child.type.name === 'date') {
      runs.push({ date: child.attrs.date })
      return
    }
    if (child.type.name === 'mention') {
      runs.push({ user: child.attrs.user, name: child.attrs.name })
      return
    }
    const run = { text: child.text }
    const names = child.marks.map((m) => m.type.name)
    if (names.length) run.marks = names
    const link = child.marks.find((m) => m.type.name === 'link')
    if (link) run.href = link.attrs.href
    runs.push(run)
  })
  for (const i of breaks) {
    const from = i > 0 ? runs[i - 1] : runs[i + 1] && !breaks.includes(i + 1) ? runs[i + 1] : null
    if (from && from.user === undefined) {
      if (from.marks) runs[i].marks = from.marks
      if (from.href) runs[i].href = from.href
    }
  }
  const merged = []
  for (const run of runs) {
    const last = merged[merged.length - 1]
    if (last && last.user === undefined && run.user === undefined && last.date === undefined && run.date === undefined && last.dropdown === undefined && run.dropdown === undefined && sameRun(last, run)) last.text += run.text
    else merged.push(run)
  }
  return merged
}

const withContent = (out, node) => {
  const content = runsOf(node)
  if (content.length) out.content = content
  return out
}

const itemsOf = (type, list) => {
  const out = []
  list.forEach((li) => {
    const item = {}
    const [head, nested] = [li.child(0), li.childCount > 1 ? li.child(1) : null]
    withContent(item, head)
    if (type === 'task_list') item.checked = li.attrs.checked === true
    if (nested) item.items = itemsOf(type, nested)
    out.push(item)
  })
  return out
}

const blockOf = (node) => {
  const type = node.type.name
  const out = { id: node.attrs.id, type }
  switch (type) {
    case 'paragraph':
      return withContent(out, node)
    case 'heading':
      out.level = node.attrs.level
      return withContent(out, node)
    case 'bullet_list':
    case 'task_list':
      out.items = itemsOf(type, node)
      return out
    case 'numbered_list':
      if (node.attrs.start !== 1) out.start = node.attrs.start
      out.items = itemsOf(type, node)
      return out
    case 'quote': {
      out.blocks = []
      node.forEach((child) => out.blocks.push(blockOf(child)))
      return out
    }
    case 'code_block':
      out.type = 'code'
      if (node.attrs.lang) out.lang = node.attrs.lang
      out.text = node.textContent
      return out
    case 'rule':
      return out
    case 'table':
      if (node.attrs.header) out.header = true
      out.rows = []
      node.forEach((tr) => {
        const cells = []
        tr.forEach((td) => {
          const cell = {}
          if (td.attrs.align) cell.align = td.attrs.align
          cells.push(withContent(cell, td.child(0)))
        })
        out.rows.push({ cells })
      })
      return out
    case 'image':
      out.src = node.attrs.src
      if (node.attrs.alt) out.alt = node.attrs.alt
      if (node.attrs.size) out.size = node.attrs.size
      return out
    case 'pending':
      out.intent = node.attrs.intent
      return out
    default:
      throw new Error(`unknown node ${type}`)
  }
}

// The blocks of a document of one or more tabs.
export const proseMirrorToBlocks = (root) => {
  const tabs = []
  root.forEach((tab) => {
    const blocks = []
    tab.forEach((b) => blocks.push(blockOf(b)))
    tabs.push({ id: tab.attrs.id, title: tab.attrs.title, blocks })
  })
  return { tabs }
}
