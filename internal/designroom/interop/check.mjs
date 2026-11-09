// Applies, with the real yjs library, the rooms the Go side wrote for each case
// (../testdata/go) and requires the formatted text in them to be the delta the
// case says it is. This is the direction the Go tests cannot check by themselves.
//
//   node check.mjs <directory of Go fixtures>
import { readdirSync, readFileSync } from 'node:fs'
import { isDeepStrictEqual } from 'node:util'
import { join } from 'node:path'
import * as Y from 'yjs'
import { checkPen } from './pen.mjs'

const dir = process.argv[2]
if (!dir) {
  console.error('usage: node check.mjs <directory of Go fixtures>')
  process.exit(2)
}
const casesDir = new URL('../testdata/cases/', import.meta.url).pathname

let failed = 0
for (const file of readdirSync(casesDir).filter((f) => f.endsWith('.json')).sort()) {
  const c = JSON.parse(readFileSync(join(casesDir, file), 'utf8'))
  const fixture = JSON.parse(readFileSync(join(dir, file), 'utf8'))
  const doc = new Y.Doc()
  Y.applyUpdate(doc, Buffer.from(fixture.update, 'hex'))
  const text = doc.getMap('design').get(`frames/hero.html|n|${c.pid}|rich`)
  const got = text instanceof Y.Text ? text.toDelta() : null
  if (!isDeepStrictEqual(got, c.delta)) {
    failed++
    console.error(`${file}: yjs reads other formatted text than the case holds\n  case: ${JSON.stringify(c.delta)}\n  yjs reads: ${JSON.stringify(got)}`)
    continue
  }
  console.log(`${file}: yjs reads the formatted text Go wrote`)
}
// The pages and frames Go wrote are one entry per value, strings, that yjs reads as they are.
{
  const fixture = JSON.parse(readFileSync(join(dir, 'structure.json'), 'utf8'))
  const doc = new Y.Doc()
  Y.applyUpdate(doc, Buffer.from(fixture.update, 'hex'))
  const m = doc.getMap('design')
  const expected = {
    's|page|p|name': 'P',
    's|row|p.r1|page': 'p',
    's|frame|hero|file': 'frames/hero.html',
    's|frame|hero|w': '800',
    's|frame|hero|row': 'p.r1',
    's|frame|foot|h': '200',
  }
  for (const [key, value] of Object.entries(expected)) {
    if (m.get(key) !== value) {
      failed++
      console.error(`structure.json: yjs reads ${JSON.stringify(m.get(key))} at ${key}, want ${JSON.stringify(value)}`)
    }
  }
  if (m.has('manifest')) {
    failed++
    console.error('structure.json: the room still holds a single manifest entry')
  }
  console.log('structure.json: yjs reads the pages and frames Go wrote')
}
checkPen(dir)
process.exit(failed ? 1 : 0)
