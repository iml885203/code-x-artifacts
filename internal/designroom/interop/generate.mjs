// Writes, with the real yjs library, what a browser editor writes into a design's
// room for each case in ../testdata/cases: the case's formatted text, as a
// Y.Text of its own, in place of the one Go wrote (../testdata/go). The Go
// tests read these as formatted text someone else wrote. A case with edits is
// also changed by two clients at once from the same start and merged by yjs.
// `make verify-interop` runs this into a scratch directory and requires the
// committed fixtures to be byte-identical.
//
//   node generate.mjs <output directory>
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import * as Y from 'yjs'
import { generatePen } from './pen.mjs'
import { generateCascade } from './cascade.mjs'

const out = process.argv[2]
if (!out) {
  console.error('usage: node generate.mjs <output directory>')
  process.exit(2)
}
mkdirSync(out, { recursive: true })

const casesDir = new URL('../testdata/cases/', import.meta.url).pathname
const goDir = new URL('../testdata/go/', import.meta.url).pathname
const hex = (u) => Buffer.from(u).toString('hex')

export const richKey = (c) => `frames/hero.html|n|${c.pid}|rich`

function fromGo(c, clientID) {
  const doc = new Y.Doc()
  Y.applyUpdate(doc, Buffer.from(JSON.parse(readFileSync(join(goDir, `${c.name}.json`), 'utf8')).update, 'hex'))
  doc.clientID = clientID
  return doc
}

function clone(doc, clientID) {
  const copy = new Y.Doc()
  Y.applyUpdate(copy, Y.encodeStateAsUpdate(doc))
  copy.clientID = clientID
  return copy
}

// One person's edit, by what it does to the words.
function edit(doc, c, step) {
  const text = doc.getMap('design').get(richKey(c))
  doc.transact(() => {
    if (step.replace) {
      const [from, to] = step.replace
      const i = text.toString().indexOf(from)
      text.delete(i, from.length)
      text.insert(i, to)
    }
    if (step.format) {
      const [word, key, value] = step.format
      text.format(text.toString().indexOf(word), word.length, { [key]: value })
    }
  })
}

let clientID = 8000
let n = 0
for (const file of readdirSync(casesDir).filter((f) => f.endsWith('.json')).sort()) {
  const c = JSON.parse(readFileSync(join(casesDir, file), 'utf8'))
  n++
  const doc = fromGo(c, clientID++)
  doc.transact(() => {
    const text = new Y.Text()
    doc.getMap('design').set(richKey(c), text)
    text.applyDelta(c.delta)
  })
  writeFileSync(join(out, file), JSON.stringify({ name: c.name, update: hex(Y.encodeStateAsUpdate(doc)) }, null, 2) + '\n')

  if (c.edits) {
    const alice = clone(doc, clientID++)
    const bob = clone(doc, clientID++)
    edit(alice, c, c.edits.alice)
    edit(bob, c, c.edits.bob)
    const merged = clone(doc, clientID++)
    Y.applyUpdate(merged, Y.encodeStateAsUpdate(alice))
    Y.applyUpdate(merged, Y.encodeStateAsUpdate(bob))
    writeFileSync(join(out, `${c.name}.merged.json`), JSON.stringify({ name: `${c.name}.merged`, update: hex(Y.encodeStateAsUpdate(merged)) }, null, 2) + '\n')
  }
}

// The pages and frames: two clients restructure the design at once, from the
// room Go wrote (../testdata/go/structure.json), and yjs merges them. One
// renames a frame and resizes it; the other puts it in a row of its own and
// renames the other page.
{
  const base = new Y.Doc()
  Y.applyUpdate(base, Buffer.from(JSON.parse(readFileSync(join(goDir, 'structure.json'), 'utf8')).update, 'hex'))
  const alice = clone(base, clientID++)
  const bob = clone(base, clientID++)
  alice.transact(() => {
    const m = alice.getMap('design')
    m.set('s|frame|hero|name', 'Banner')
    m.set('s|frame|hero|w', '390')
  })
  bob.transact(() => {
    const m = bob.getMap('design')
    m.set('s|row|r2|page', 'p')
    m.set('s|row|r2|order', 'V')
    m.set('s|frame|hero|row', 'r2')
    m.set('s|frame|hero|order', 'V')
    m.set('s|page|q|name', 'Footer')
  })
  const merged = clone(base, clientID++)
  Y.applyUpdate(merged, Y.encodeStateAsUpdate(alice))
  Y.applyUpdate(merged, Y.encodeStateAsUpdate(bob))
  writeFileSync(join(out, 'structure.merged.json'), JSON.stringify({ name: 'structure.merged', update: hex(Y.encodeStateAsUpdate(merged)) }, null, 2) + '\n')
}

generatePen(goDir, out)

generateCascade(goDir, out)
