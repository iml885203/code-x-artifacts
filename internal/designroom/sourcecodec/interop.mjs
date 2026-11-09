// Uses the same pinned real yjs dependency as the existing designroom boundary.
// node interop.mjs <go fixture> <yjs output>; reads Go source slots and writes an
// actual Y.Text edit, leaving protected logic/schema/props and envelopes alone.
import { readFileSync, writeFileSync } from 'node:fs'
import assert from 'node:assert/strict'
import * as Y from '../interop/node_modules/yjs/dist/yjs.mjs'

const [input, output] = process.argv.slice(2)
if (!input || !output) throw new Error('usage: interop.mjs <go fixture> <yjs output>')
const fixture = JSON.parse(readFileSync(input, 'utf8'))
const doc = new Y.Doc()
Y.applyUpdate(doc, Buffer.from(fixture.update, 'hex'))
doc.clientID = 15002
const map = doc.getMap('design')
assert.equal(map.get('v2|manifest'), fixture.manifest)
for (const [key, value] of Object.entries(fixture.entries)) {
  const actual = map.get(key)
  assert.equal(actual instanceof Y.Text ? actual.toString() : actual, value, key)
}
const literal = map.get(fixture.literalKey)
assert.ok(literal instanceof Y.Text)
assert.equal(literal.toString(), 'Hello &amp; ')
literal.insert(0, '朋友😀 ')
const update = Buffer.from(Y.encodeStateAsUpdate(doc)).toString('hex')
writeFileSync(output, JSON.stringify({ update, after: '朋友😀 Hello &amp; ' }) + '\n')
console.log('yjs reads exact Go source units and writes Unicode literal text')
