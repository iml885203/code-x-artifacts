// Writes the fixtures the Go tests apply: updates produced by the real yjs
// library, each with the content yjs says they build. `make verify-interop`
// runs this into a scratch directory and requires the committed fixtures to
// be byte-identical, so a yjs upgrade or a hand edit shows up as a failure
// instead of a silent change of what the Go room is tested against.
//
//   node generate.mjs <output directory>
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import * as Y from 'yjs'

const out = process.argv[2]
if (!out) {
  console.error('usage: node generate.mjs <output directory>')
  process.exit(2)
}
mkdirSync(out, { recursive: true })

const hex = (u) => Buffer.from(u).toString('hex')

// Records every update a doc emits, in order.
function recorder(doc) {
  const updates = []
  doc.on('update', (u) => updates.push(u))
  return updates
}

function write(name, updates, expect) {
  const body = { name, updates: updates.map(hex), expect }
  writeFileSync(join(out, `${name}.json`), JSON.stringify(body, null, 2) + '\n')
}

// Text with an emoji and CJK: Yjs counts UTF-16 code units, so an astral
// character takes two and an index after it is easy to get wrong.
{
  const doc = new Y.Doc()
  doc.clientID = 101
  const updates = recorder(doc)
  const text = doc.getText('t')
  text.insert(0, 'Hello ')
  text.insert(6, '😀 世界 ')
  text.insert(text.length, '𠮷野家')
  text.delete(2, 2) // "Heo ..."
  text.insert(text.length, '!')
  write('text', updates, { text: text.toString() })
}

// Two clients edit the same text at the same time from a shared base.
{
  const base = new Y.Doc()
  base.clientID = 201
  const baseUpdates = recorder(base)
  base.getText('t').insert(0, 'shared base')

  const a = new Y.Doc()
  a.clientID = 202
  const b = new Y.Doc()
  b.clientID = 203
  Y.applyUpdate(a, baseUpdates[0])
  Y.applyUpdate(b, baseUpdates[0])
  const aUpdates = recorder(a)
  const bUpdates = recorder(b)
  a.getText('t').insert(0, '世界 ')
  b.getText('t').insert(b.getText('t').length, ' 😀')
  b.getText('t').insert(7, '[B]')

  const merged = new Y.Doc()
  for (const u of [...baseUpdates, ...aUpdates, ...bUpdates]) Y.applyUpdate(merged, u)
  write('concurrent', [...baseUpdates, ...aUpdates, ...bUpdates], { text: merged.getText('t').toString() })
}

// The shape y-prosemirror stores: block elements with attributes, text inside.
{
  const doc = new Y.Doc()
  doc.clientID = 301
  const updates = recorder(doc)
  const fragment = doc.getXmlFragment('content')
  const heading = new Y.XmlElement('heading')
  heading.setAttribute('id', 'b1')
  heading.setAttribute('level', 2)
  const headingText = new Y.XmlText()
  heading.insert(0, [headingText])
  const paragraph = new Y.XmlElement('paragraph')
  paragraph.setAttribute('id', 'b2')
  const paragraphText = new Y.XmlText()
  paragraph.insert(0, [paragraphText])
  fragment.insert(0, [heading, paragraph])
  headingText.insert(0, '標題 Title')
  paragraphText.insert(0, 'first line 😀')
  paragraphText.insert(paragraphText.length, ' and more')
  write('xml', updates, { xml: fragment.toString() })
}
