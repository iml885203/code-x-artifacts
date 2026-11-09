# internal/boardroom

白板在 room 裡（`docs/spikes/board.md`）：Excalidraw 的元素在共享 `Y.Map`（`board`）裡，每個元素的每個欄位一個 entry（`<元素 id>|<欄位>`，值是欄位的 JSON 文字）；hub 在保留每個更新之前問的 `Check`；`Read`、`Write`、`State` 讓 `roomkind` 的 board codec 在版本與 room 之間轉換。規則來自 `internal/boardfmt`。純函式加 ygo，不 log。

## 不變式

- **一個欄位一個 entry，所以兩個人改同一個元素的不同欄位（一個搬框、一個改標籤的字）都留著；`version`、`versionNonce`、`updated` 是每個 replica 自己的記帳，不進 room。不合 `boardfmt` 規則的更新被 hub 拒絕，房間保持原樣、別人收不到；別的種類的 room 不受影響。**
  Tests: `TestTheElementsTheEditorDrawsAreAccepted`, `TestWhatABoardMayNotHoldIsRefusedAndTheMessageNamesTheElementAndField`, `TestARoomWithoutABoardIsNotTheBoardsToRefuse`, `TestWhatABoardMayNotHoldIsRefusedOverTheSocketAndTheRoomKeepsWhatItHad`, `TestWhatABoardMayNotHoldIsRefusedInItsRoomAndTheRoomKeepsWhatItHad`；兩個瀏覽器的收斂、標籤跟著框、各自的 Undo 見 `web/AGENTS.md` 的白板一條與 e2e/tests/board.spec.ts。
- **寫一張白板進 room 只改不同的欄位（別人同時改的別處留著）；room 有而白板沒有的元素標成 `isDeleted`，不刪 entry（編輯器要被告知刪除才會拿掉）；回來的元素 `isDeleted` 寫回 false；room 裡白板不能有的內容被整個取代。從 room 種出的白板讀回是同一張。**
  Tests: `TestARoomSeededFromABoardGivesTheSameBoardBack`, `TestWritingABoardIntoARoomChangesOnlyWhatDiffersAndMarksWhatIsGoneDeleted`, `TestARoomHoldingWhatNoBoardCanIsReplacedByTheBoardWrittenIntoIt`, `TestAWholeBoardPublishWithTheRoomOpenGoesIntoTheRoomAndIsTheVersion`.
