# internal/docroom

doc 在 room 裡（`docs/doc-room.md`）：區塊 ↔ `Y.XmlFragment` 的對應（讀、寫、正規形式）、從最新版本開始的 room log（`SeededLog`）。`interop/` 是編輯器的 ProseMirror schema 與產生、檢查 fixture 的 Node 腳本（自己的 go.mod）。

## 不變式

- **doc 在 room 裡：分頁順序是共用陣列 `tabs`（分頁 id），每個分頁是自己的 `Y.XmlFragment` `tab:<id>`（唯一的子節點是 `tab` 元素），所以搬動分頁只動順序、不重建內容；舊的單一 fragment `content` 形式讀得懂、載入時搬成現在的形式。分頁內是 `Y.XmlFragment`（y-prosemirror 的形狀：區塊的 id 與 level 是元素屬性，mark 是文字的屬性），讀回來的區塊 JSON 與寫進去的相同（正規形式），id 全部保留；編輯器的 schema 寫出的文件 Go 讀得懂，Go 寫的文件編輯器的 schema 讀得懂。** 共用的文件在 `testdata/cases`，編輯器寫的 update 在 `testdata/yjs`、Go 寫的在 `testdata/go`。查共享型別與讀取只在 plan 階段：寫入時讀取立刻回 `ErrReadWhileWriting`，不會卡住。沒有 log 的 doc 的 room 從最新版本開始、保留區塊 id；已有 log 的不再重來。
  Tests: `TestBlocksToTheRoomAndBackGiveTheSameJSONForEveryCase`, `TestTheCasesHoldEveryBlockTypeAndMark`, `TestWhatTheRoomCannotTellApartIsReadBackAsOneForm`, `TestGoReadsWhatTheEditorSchemaWrote`, `TestTheDocumentsGoWritesAreTheCommittedFixturesTheEditorSchemaReads`, `TestReadingARoomThatHoldsSomethingTheSchemaHasNotGotNamesWhere`, `TestReadingWhileWritingFailsFastInsteadOfHanging`, `TestARoomForADocWithNoLogStartsFromTheLatestVersionAndKeepsItsIds`, `TestARoomWithALogIsNotSeededAgainWhenTheArtifactIsNewer`, `TestAnEditThroughTheHubIsReadBackAndSurvivesARestart`, `TestAnAgentChangesAnImagesAltAndSizeInPlaceAndTheRoomReadsThemBack`（圖片的 `size` 與 `alt` 一樣是元素屬性，沒有就不寫）；make verify-interop。
  搬動與遷移的 Tests: `TestAMoveWhileSomeoneTypesInTheMovingTabKeepsTheOrderAndEveryCharacter`, `TestTwoPeopleWhoMoveTabsAtOnceEndWithEveryTabOnceInTheSameOrder`, `TestARoomInTheOldOneFragmentFormIsReadAndMigratedWithoutChangingTheDocument`, `TestAMoveFromTheEditorMergedWithTypingFromAnotherEditorIsReadByGo`（interop：testdata/yjs/move-while-typing.json 與 testdata/go/move-while-typing.json 經 check.mjs）；roomkind 的 `TestARoomWhoseLogIsInTheOldFormIsMigratedWhenItLoadsAndStaysSoAfterARestart`。

- **Date 是帶字串 `date` 屬性的共用行內 atom，正規化不丟失、不與文字合併；只改日期更新既有元素，保留同段並行輸入，room log 重載後仍有欄位。**
  Tests: `TestChangingADatePreservesConcurrentTypingInTheSameParagraph`, `TestFieldsInTheRoomSurviveASQLiteLogRestart`, `TestBlocksToTheRoomAndBackGiveTheSameJSONForEveryCase`, `TestGoReadsWhatTheEditorSchemaWrote`, `TestTheDocumentsGoWritesAreTheCommittedFixturesTheEditorSchemaReads`.

- **Date 與 Dropdown 的欄位屬性更新在結構不變的清單（含巢狀項目）與表格保留既有 atom、段落和共用文字；結構變更使用容器替換，不執行未完成判定的欄位操作。**
  Tests: `TestFieldChangesInContainersPreserveConcurrentTypingAndSharedNodes`, `TestFieldContainerChangesFallBackForLaterStructuralEdits`.
