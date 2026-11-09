# internal/boardedit

agent 局部修改白板（`board_edit`，HTTP 是 `POST /api/artifacts/{id}/board-edit`、讀 hash 是 `GET /api/artifacts/{id}/board` 與 MCP `read` 的 `hashes`）：`add` 一個元素（可以是骨架）、`change` 一個元素的欄位（`label` 改框裡的字）、`delete` 一個元素。純函式，建在 `internal/boardfmt` 上；寫進 room 的是 `boardroom.Merge`。

## 不變式

- **一次呼叫的 op 依序做、全做或全不做；change 與 delete 要帶讀到的 `ifHash`，元素已被別人改過就整個拒絕，並附上那些元素現在的樣子與 hash；hash 對照的是呼叫前的白板。新增的元素畫在最上面；刪除帶走它的標籤、解開綁在它上面的箭頭；id 與 type 不能改。做完的白板要通過 `boardfmt` 的規則。**
  Tests: `TestAChangeSetsTheFieldsItNamesAndTheLabelOfAShapeAndLeavesTheRest`, `TestAnAddDrawsASkeletonElementBoundToShapesAlreadyThereAndAboveThem`, `TestADeleteRemovesTheElementWithItsLabelAndUnbindsWhatWasBoundToIt`, `TestAnOpOnAnElementSomeoneChangedSinceItWasReadIsRefusedWithTheElementAsItIsNow`, `TestAnOpThatCannotBeDoneRefusesTheWholeCallSayingWhich`.
- **白板開著時只寫 op 改到的欄位（`boardroom.Merge`），別人同時改的別的元素、同一個元素的別的欄位都留著；agent 以名字出現在 room 裡；沒有 room 時存成新版本。HTTP 與 MCP 給同樣的結果與同樣的拒絕。**
  Tests: `TestMergingAnAgentsChangeKeepsWhatAPersonChangedInTheRoomSince`, `TestAnAgentsBoardEditChangesOneBoxsWordsWhileAPersonMovesAnotherAndBothStay`, `TestABoardEditOverHTTPAndMCPGivesTheSameResultAndTheSameRefusalForAStaleHash`；e2e/tests/board.spec.ts（agent 的修改即時出現在兩個開著的瀏覽器，名字在右上角）。
