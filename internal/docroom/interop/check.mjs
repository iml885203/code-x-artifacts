// Reads, with the editor's schema, the documents the Go bridge wrote
// (testdata/go) and requires each to be a valid document of the schema that
// holds the blocks of its case. This is the direction the Go tests cannot
// check by themselves.
//
//   node check.mjs <directory of Go fixtures>
import { readdirSync, readFileSync } from 'node:fs'
import { isDeepStrictEqual } from 'node:util'
import { join } from 'node:path'
import * as Y from 'yjs'
import { yXmlFragmentToProseMirrorRootNode } from 'y-prosemirror'
import { schema } from './schema.mjs'
import { proseMirrorToBlocks } from './model.mjs'

const dir = process.argv[2]
if (!dir) {
  console.error('usage: node check.mjs <directory of Go fixtures>')
  process.exit(2)
}
const casesDir = new URL('../testdata/cases/', import.meta.url).pathname

let failed = 0
const moveFile = 'move-while-typing.json'
for (const file of readdirSync(dir).filter((f) => f.endsWith('.json') && f !== moveFile).sort()) {
  const fixture = JSON.parse(readFileSync(join(dir, file), 'utf8'))
  const want = JSON.parse(readFileSync(join(casesDir, file), 'utf8'))
  try {
    const ydoc = new Y.Doc()
    Y.applyUpdate(ydoc, Buffer.from(fixture.update, 'hex'))
    // A room holds the order of the tab ids and a fragment per tab; each is a document of one tab.
    const got = { tabs: [] }
    for (const id of ydoc.getArray('tabs').toArray()) {
      const root = yXmlFragmentToProseMirrorRootNode(ydoc.getXmlFragment('tab:' + id), schema)
      root.check()
      got.tabs.push(...proseMirrorToBlocks(root).tabs)
    }
    if (!isDeepStrictEqual(got, want)) {
      failed++
      console.error(`${file}: the editor schema reads something else than Go wrote\n  Go wrote: ${JSON.stringify(want)}\n  schema read: ${JSON.stringify(got)}`)
      continue
    }
    console.log(`${file}: the editor schema reads what Go wrote`)
  } catch (e) {
    failed++
    console.error(`${file}: ${e.message}`)
  }
}

// A move Go made, merged with what a person typed in the moved tab from the same start:
// the order is Go's and the text is the person's, whichever update is applied first.
try {
  const fx = JSON.parse(readFileSync(join(dir, moveFile), 'utf8'))
  const from = (hex) => Buffer.from(hex, 'hex')
  const start = () => {
    const d = new Y.Doc()
    Y.applyUpdate(d, from(fx.base))
    return d
  }
  const person = start()
  const before = Y.encodeStateVector(person)
  const run = person.getXmlFragment('tab:t3').get(0).get(0).get(0)
  run.insert(run.length, ' + typed')
  const typing = Y.encodeStateAsUpdate(person, before)
  const server = start()
  Y.applyUpdate(server, from(fx.move))
  Y.applyUpdate(person, from(fx.move))
  Y.applyUpdate(server, typing)
  for (const [who, d] of [['server', server], ['person', person]]) {
    const order = d.getArray('tabs').toArray().join(',')
    const text = d.getXmlFragment('tab:t3').get(0).get(0).get(0).toString()
    if (order !== 't3,t1,t2' || !text.includes('text of t3 + typed')) throw new Error(`${who} ended with order ${order} and text ${text}`)
  }
  console.log(`${moveFile}: Go's move and a person's typing in the moved tab both stay`)
} catch (e) {
  failed++
  console.error(`${moveFile}: ${e.message}`)
}
process.exit(failed ? 1 : 0)
