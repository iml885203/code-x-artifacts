# internal/designroom/sourcecodec

Inert format 2 source codec; not registered in roomkind. No runtime admission or
production socket/service authority is implied by encoding a source template.

- One source envelope and typed source slots retain exact normalized source,
  including directives, logic, declarations, rich marks, SVG and token bytes;
  instances are never persisted as duplicate source nodes. Literal slots are
  Y.Text, protected slots and envelopes cannot be changed by raw-client policy.
  Tests: `TestSourceCodecRetainsOneTemplateAndExactBytesWhileLiteralTextCoedits`,
  `TestRawSourceUpdatesCannotChangeProtectedUnitsOrConvertLiteralRunsToBindings`.

- Real yjs reads the Go source units and writes Unicode literal text that Go
  projects without changing bindings or other source bytes. Test:
  `TestGoAndRealYjsAgreeOnTypedSourceUnitsAndUnicodeLiteralEdits`.

- Head declarations have protected source slots and omitted body blocks/comments
  never alias separate literal runs. Tests:
  `TestDeclarationsInTheHeadStayProtectedWhileInertBodyBlocksKeepDistinctLiteralGaps`,
  `TestOpaqueCommentsNeverAliasDistinctLiteralSourceSlots`.

- Validated source writes preserve existing literal CRDT objects so pending
  coedits survive protected prop changes; invalid planning changes no room byte.
  Tests: `TestValidatedSourceWritesKeepPendingLiteralCoedits`,
  `TestInvalidSourceWritePlanningLeavesEveryRoomByteUnchanged`.
