// Applies the updates the Go room produced (committed under testdata/go) with
// the real yjs library and requires them to build the content Go says they
// build. This is the direction the Go tests cannot check by themselves.
//
//   node check.mjs <directory of Go fixtures>
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import * as Y from 'yjs'

const dir = process.argv[2]
if (!dir) {
  console.error('usage: node check.mjs <directory of Go fixtures>')
  process.exit(2)
}

let failed = 0
for (const file of readdirSync(dir).filter((f) => f.endsWith('.json')).sort()) {
  const fixture = JSON.parse(readFileSync(join(dir, file), 'utf8'))
  const doc = new Y.Doc()
  for (const u of fixture.updates) Y.applyUpdate(doc, Buffer.from(u, 'hex'))
  const got = {}
  if ('text' in fixture.expect) got.text = doc.getText('t').toString()
  if ('xml' in fixture.expect) got.xml = doc.getXmlFragment('content').toString()
  for (const [key, want] of Object.entries(fixture.expect)) {
    if (got[key] !== want) {
      failed++
      console.error(`${file}: ${key}\n  Go says: ${JSON.stringify(want)}\n  yjs says: ${JSON.stringify(got[key])}`)
    }
  }
  console.log(`${file}: ${fixture.updates.length} updates apply in yjs`)
}
process.exit(failed ? 1 : 0)
