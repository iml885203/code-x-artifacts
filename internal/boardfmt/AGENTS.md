# internal/boardfmt

白板（`board` kind）的內容：格式 `1` 的 JSON `{version, elements}`，元素是 Excalidraw 的元素；以及白板能有什麼的唯一一套規則。發佈（`kind.Board`）與 room 的每個更新（`internal/boardroom`）都用這裡的 `CheckID`／`CheckField`，所以從 room 進不了發佈不收的東西，反過來也一樣。純函式，不 log。

## 不變式

- **白板只收畫得出來、而且不會載入別的東西的內容：元素種類與欄位都是 allow-list（沒有 `embeddable`、`iframe`；圖片的 `fileId` 只能是上傳檔的名稱，不能是路徑、網址或資料），連結只能是 `https:` 或 `mailto:`，任何欄位裡都不能有 `data:` 位址，數字有限而且有界，顏色只能是 `transparent` 或 `#hex`，整份最多 1 MB、10000 個元素；頂層只有 `version` 與 `elements`。拒絕的訊息點名元素與欄位。`version`、`versionNonce`、`updated` 是每個編輯器自己的記帳，收下後丟掉。**
  Tests: `TestWhatABoardMayNotHoldIsRefusedAtPublishAndTheMessageSaysWhereAndWhat`, `TestABoardJustUnderTheSizeLimitIsAcceptedAndAnotherFormatIsRefused`, `TestABoardAnAgentWritesIsReadBackWithEveryElementAndItsBindings`, `TestPublishingABoardOverHTTPAndMCPGivesTheSameResultAndTheSameRefusalNamingTheElementAndField`.
- **白板存成一種正規形式（`Canonical`）：不含已刪除的元素、依 `index` 再依 id 排序、每個標籤（有 `containerId` 的 text）的 `x`、`y` 依它的框與對齊重算（照 Excalidraw 的內縮）。發佈與從 room 切出的版本是同一個函式，所以同一張白板是同一段文字；標籤的位置不信任 room 裡的值，因為搬框與改字的兩個人會各寫一次。**
  Tests: `TestTheStoredFormIsInDrawingOrderWithoutDeletedElementsAndWithLabelsOnTheirShapes`, `TestALabelIsLaidOutInsideItsShapeTheWayTheEditorDoes`, `TestABoardIsStoredInOneFormWhateverOrderItWasWrittenIn`, `TestABoardsRoomStartsFromItsVersionAndAQuietRoomBecomesAVersionWithTheLabelOnItsShape`.
- **`SVG` 把白板畫成一張獨立的 SVG（唯讀檢視、圖庫卡片、agent 的匯出）：每種元素都畫得出來（框、橢圓、菱形、線與箭頭和它的頭、筆畫、多行文字、frame 與名稱、透明度、旋轉），範圍包住每個元素再留邊；白板裡的字一律跳脫，不會變成標記；顏色只會是規則放行的 `#hex`。線條是整齊的，不是編輯器的手繪風格。**
  Tests: `TestEveryElementABoardHoldsIsDrawnInItsSVG`, `TestABoardsTextIsTextInItsSVGAndNeverMarkup`, `TestABoardsSVGFramesEveryElementAndAnEmptyBoardIsABlankOne`.
- **骨架（`Expand`）讓 agent 只寫圖的意思：框、橢圓、菱形可以有 `label`（server 補上綁定的文字元素 `<id>-label`、估出大小並置中；沒給大小的框依字給大小），箭頭與線可以有 `from`／`to`（server 算出在兩個形狀外框上的起點與終點、雙向綁定，箭頭預設有頭）；都沒有 `index` 時依寫的順序畫、標籤緊跟在它的框後面；沒有這些欄位的白板原樣不動。不畫出來的骨架（指到不存在或不是形狀的元素、只有一端、指到自己、label 放在不是形狀的元素上、標籤 id 已被用、有 label 卻沒有位置）以點名元素的訊息拒絕。不做自動排版。**
  Tests: `TestASkeletonOfLabelledBoxesAndArrowsBecomesBoundElementsWithCentredLabels`, `TestASkeletonIsDrawnInTheOrderItIsWrittenWithEachLabelAfterItsShape`, `TestABoardWithoutSkeletonFieldsIsLeftAsItWas`, `TestASkeletonThatCannotBeDrawnIsRefusedNamingTheElement`, `TestTheIndexesASkeletonGetsKeepTheirOrderAsPlainStringsAcrossEveryLength`, `TestASkeletonPublishedOverHTTPAndMCPBecomesTheSameBoundBoardAndIsRefusedTheSameWay`.
- **圖片是白板 artifact 的上傳檔：`FileIDs` 列出白板的圖片用到的檔案；發佈（新增與更新）與 `board_edit` 拒絕 `fileId` 不是這個 artifact 檔案的圖片，訊息列出白板有哪些檔案；新的白板沒有檔案，所以要先發佈、上傳、再加圖。瀏覽器只從這個 artifact 的 `/files/` 載入圖片，別的 artifact 的檔案與別的網站都到不了。**
  Tests: `TestAnImageOfAnUploadedFileIsAcceptedAndItsFileIsListed`, `TestABoardShowsOnlyImagesOfItsOwnFilesWhetherPublishedOrAddedByBoardEdit`；前端 web/src/board/files.test.ts；e2e/tests/board.spec.ts（貼上的圖片成為白板的檔案、另一個人即時看到、重新載入還在；檔案刪掉後白板照樣打開、不去要不存在的檔案）。
