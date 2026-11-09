import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import * as Y from 'yjs'
const file = 'frames/hero.html'
const key = (pid, field) => `${file}|n|${pid}|${field}`
const from = (dir) => {
 const doc = new Y.Doc()
 Y.applyUpdate(doc, Buffer.from(JSON.parse(readFileSync(join(dir, 'stroke.json'), 'utf8')).update, 'hex'))
 return doc
}
export function checkPen(dir) {
 const map = from(dir).getMap('design')
 const attrs = JSON.parse(map.get(key('pen-path', 'attrs')))
 for (const [name, value] of [['d', 'M2 2Q12 12 17 7L22 2'], ['stroke', 'var(--accent)'], ['stroke-width', '4']]) {
  if (!attrs.some(([ns, k, v]) => ns === '' && k === name && v === value)) throw new Error(`Go stroke lost ${name}=${value}`)
 }
 if (map.get(key('pen', 'ns')) !== 'svg' || map.get(key('pen-path', 'ns')) !== 'svg') throw new Error('stroke lost its SVG namespace')
 console.log('stroke.json: yjs reads the vector path, variable color and width Go wrote')
}
export function generatePen(goDir, out) {
 const doc = from(goDir);doc.clientID = 8300
 const map = doc.getMap('design')
 doc.transact(() => {
  const attrs = JSON.parse(map.get(key('pen-path', 'attrs')))
  for (const row of attrs) {
   if (row[1] === 'd') row[2] = 'M1 1L30 40'
   if (row[1] === 'stroke') row[2] = '#336699'
   if (row[1] === 'stroke-width') row[2] = '6'
  }
  map.set(key('pen-path', 'attrs'), JSON.stringify(attrs))
  map.set(key('pen', 's|left'), '100px')
  map.set(key('pen', 's|width'), '80px')
 })
 writeFileSync(join(out, 'stroke.json'), JSON.stringify({name:'stroke',update:Buffer.from(Y.encodeStateAsUpdate(doc)).toString('hex')},null,2)+'\n')
}
