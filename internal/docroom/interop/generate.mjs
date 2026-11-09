// Writes what the editor's schema makes of each case in ../testdata/cases: the
// Yjs update y-prosemirror produces for it. The Go tests read these as a
// document the editor wrote. `make verify-interop` runs this into a scratch
// directory and requires the committed fixtures to be byte-identical.
//
//   node generate.mjs <output directory>
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import * as Y from 'yjs'
import { prosemirrorToYXmlFragment } from 'y-prosemirror'
import { tabToProseMirror } from './model.mjs'

// A room the way the editor keeps it: the order of the tab ids, and a fragment per tab.
const fillRoom = (ydoc, doc) => {
  for (const tab of doc.tabs) prosemirrorToYXmlFragment(tabToProseMirror(tab), ydoc.getXmlFragment('tab:' + tab.id))
  ydoc.getArray('tabs').push(doc.tabs.map((tab) => tab.id))
}

const out = process.argv[2]
if (!out) {
  console.error('usage: node generate.mjs <output directory>')
  process.exit(2)
}
mkdirSync(out, { recursive: true })

const casesDir = new URL('../testdata/cases/', import.meta.url).pathname
let clientID = 5000
for (const file of readdirSync(casesDir).filter((f) => f.endsWith('.json')).sort()) {
  const doc = JSON.parse(readFileSync(join(casesDir, file), 'utf8'))
  const ydoc = new Y.Doc()
  ydoc.clientID = clientID++
  fillRoom(ydoc, doc)
  const update = Y.encodeStateAsUpdate(ydoc)
  writeFileSync(join(out, file), JSON.stringify({ name: file.replace(/\.json$/, ''), update: Buffer.from(update).toString('hex') }, null, 2) + '\n')
}

// One person moves a tab while another types in it: the two updates, made from the
// same start by two editors, which Go has to merge (move-while-typing.json).
const start = {
  tabs: ['t1', 't2', 't3'].map((id) => ({ id, title: id.toUpperCase(), blocks: [{ id: 'b_' + id, type: 'paragraph', content: [{ text: 'text of ' + id }] }] }))
}
const base = new Y.Doc()
base.clientID = 5100
fillRoom(base, start)
const baseUpdate = Y.encodeStateAsUpdate(base)
const replica = (clientID) => {
  const d = new Y.Doc()
  d.clientID = clientID
  Y.applyUpdate(d, baseUpdate)
  return d
}
const change = (d, fn) => {
  const before = Y.encodeStateVector(d)
  fn()
  return Y.encodeStateAsUpdate(d, before)
}
const mover = replica(5101)
const move = change(mover, () => {
  const order = mover.getArray('tabs')
  mover.transact(() => {
    order.delete(2, 1)
    order.insert(0, ['t3'])
  })
})
const typist = replica(5102)
const typing = change(typist, () => {
  const run = typist.getXmlFragment('tab:t3').get(0).get(0).get(0)
  run.insert(run.length, ' + typed while it moved')
})
const hex = (u) => Buffer.from(u).toString('hex')
writeFileSync(join(out, 'move-while-typing.json'), JSON.stringify({ name: 'move-while-typing', base: hex(baseUpdate), move: hex(move), typing: hex(typing) }, null, 2) + '\n')
