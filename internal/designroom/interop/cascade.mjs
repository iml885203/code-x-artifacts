import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import * as Y from 'yjs'

export function generateCascade(goDir, out) {
 const base = new Y.Doc()
 Y.applyUpdate(base, Buffer.from(JSON.parse(readFileSync(join(goDir, 'structure.json'), 'utf8')).update, 'hex'))
 base.clientID = 8400
 const prefix = 'frames/hero.html|n|cascade|'
 base.transact(() => {
  const map = base.getMap('design')
  for (const [field, value] of Object.entries({kind:'el',tag:'div',ns:'',parent:'e1',order:'zz',attrs:JSON.stringify([['','\u0000style',''],['','\u0000pid','']]),rest:'border-top-color:border-top-color;border:2px solid red !important;background-color:background-color;background:red !important;transform:rotate(2deg)','s|border-top-color':'blue !important','s|background-color':'blue !important'})) map.set(prefix + field, value)
 })
 const save = (name, doc) => writeFileSync(join(out, `cascade.${name}.json`), JSON.stringify({name:`cascade.${name}`,update:Buffer.from(Y.encodeStateAsUpdate(doc)).toString('hex')},null,2)+'\n')
 const clone = (id) => { const doc=new Y.Doc();Y.applyUpdate(doc,Y.encodeStateAsUpdate(base));doc.clientID=id;return doc }
 save('base',base)
 const alice=clone(8401), bob=clone(8402)
 const map=alice.getMap('design'), undo=new Y.UndoManager(map,{captureTimeout:0,trackedOrigins:new Set(["properties"])})
 alice.transact(() => map.set(prefix+'override|border-top-color','green !important'),'properties')
 if (undo.undoStack.length!==1) throw new Error('one browser action must be one Undo step')
 bob.transact(() => bob.getMap('design').set(prefix+'override|background-color','transparent !important'))
 Y.applyUpdate(alice,Y.encodeStateAsUpdate(bob));Y.applyUpdate(bob,Y.encodeStateAsUpdate(alice))
 save('concurrent',alice)
 undo.undo();save('undo',alice)
 if (map.has(prefix+'override|border-top-color')) throw new Error('Undo retained the local override')
 if (map.get(prefix+'override|background-color')!=='transparent !important') throw new Error('Undo removed the collaborator fill')
 undo.redo();save('redo',alice)
 Y.applyUpdate(bob,Y.encodeStateAsUpdate(alice))
 bob.transact(() => bob.getMap('design').set(prefix+'override|border-top-color','yellow !important'))
 Y.applyUpdate(alice,Y.encodeStateAsUpdate(bob))
 undo.undo();save('later',alice)
 if (map.get(prefix+'override|border-top-color')!=='yellow !important') throw new Error('Undo removed a later collaborator value')
 const first=clone(8403), second=clone(8404)
 const firstMap=first.getMap('design'), firstUndo=new Y.UndoManager(firstMap,{captureTimeout:0,trackedOrigins:new Set(['properties'])})
 first.transact(() => firstMap.set(prefix+'override|border-top-color','green !important'),'properties')
 second.transact(() => second.getMap('design').set(prefix+'override|border-top-color','yellow !important'),'properties')
 Y.applyUpdate(first,Y.encodeStateAsUpdate(second));Y.applyUpdate(second,Y.encodeStateAsUpdate(first))
 firstUndo.undo();save('first-race',first)
 if (firstMap.get(prefix+'override|border-top-color')!=='yellow !important') throw new Error('concurrent first-edit Undo removed the winning peer value')
 console.log('cascade: direct Yjs edits, independent properties, one-step Undo/Redo and later collaborator edits agree')
}
