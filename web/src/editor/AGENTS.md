# web/src/editor（Docs 編輯器）

Docs 編輯器（`docs/editor.md`）：schema、id plugin、socket provider、view、畫面。

## 不變式

- **在 viewer shell 裡、是獨立的 chunk（登入的人打開 doc 時才載入，總覽與其他種類的 bundle 不含它）；schema 就是 Go 橋接用的那份（`internal/docroom/interop/schema.mjs`），不另外分岔；每個區塊都有唯一的 id（id plugin）；貼上只經 schema 的解析規則（script、事件屬性、style、`javascript:` 都無處可去；連結只在位址在清單上時才留下）；Undo 只復原自己的編輯；room 的內容到達之前不建 view；別人的游標沒有子節點、名字來自 server 的 roster。** 「Saved」只在 server 已存下才亮：每個 update 之後多送的 sync step 1 得到回應。Doc 沒有檢視／編輯模式：能編輯的人直接進編輯器，其他人看到同一份 room 的即時畫面，沒有插入列、打不了字；舊版本是頁面的 frame。「已儲存」在標題旁，來自 room（SaveState.tsx，測試在 Editing.test.tsx）。
  Tests: web/src/editor/schema.test.ts（與共用 schema 同形、連結與圖片只畫允許的位址）, web/src/editor/paste.test.ts（網頁、Word、Google Docs 的剪貼簿、不安全的連結）, web/src/editor/link.test.ts（位址檢查、存、改、移除、Undo、重新載入）, web/src/editor/LinkPopover.test.tsx（被拒的位址顯示原因、Cancel 不存）, web/src/editor/table.test.ts（每個操作只動該動的格、Undo／Redo／重新載入、Tab、上限、不跨格刪除、兩人同時、貼上的表）, web/src/editor/InsertRow.test.tsx（各功能的工具、按下的狀態、表格與圖片工具、小工具列的工具不在列裡）, web/src/editor/FormatBubble.test.tsx（選字出現、套用、收合隱藏、唯讀不出現、與 Comment 並存）, web/src/editor/bubblePlace.test.ts（在上方、沒地方就在下方、夾在頁面內）, web/src/editor/features.test.ts（id 不重複、按鍵不重複）, web/src/editor/image.test.ts（插入、alt 與大小、刪除與 Undo、載入失敗的佔位、貼上的 img 只認清單上的位址、截圖與拖入、不同名字）, web/src/viewer/EditingPictures.test.tsx（Image 按鈕上傳、server 的原因）, web/src/editor/view.test.ts（空白提示只在空文件、a hostile clipboard、undoes only the person's own typing…、gives the second half of a split block an id of its own…、全選貼上不換分頁）, web/src/editor/provider.test.ts（重連與 ticket、epoch、每個 close code、離線、Saved 要等 server 回應、唯讀不送）, web/src/lint.test.ts, web/src/viewer/Editing.test.tsx（編輯者直接進編輯器、讀者同一份內容沒有插入列、插入列的每個工具、舊版本唯讀、PDF 從畫面外的頁面印）；e2e/tests/editor-links.spec.ts（Cmd+K、改、移除）, e2e/tests/editor-tables.spec.ts（插入、Tab、對齊、兩人同時、貼表）, e2e/tests/editor-images.spec.ts（貼上、alt 與大小、第二個瀏覽器、拖入、佔位）, e2e/tests/editor.spec.ts（兩個瀏覽器即時共編、唯讀的第三個、離線重連、Undo、編輯器 chunk 不在總覽）。

- **分頁的順序是 room 裡的 id 陣列、每個分頁的內容在自己的 fragment（`tabs.ts`，docs/doc-room.md）：搬動只動陣列、不碰分頁內容（別人正在打的字留著），新分頁從一個空 H1 開始；標題只收文件存得下的（一行、非空、至多 200 字，否則 room 永遠存不成版本）；最後一個分頁不能刪；編輯器一次只對一個分頁開 view，正在看哪個只屬於自己，被別人刪掉時退回第一個，搬動的分頁仍是正在看的那個；每個分頁的 Undo 歷史在文件開著期間保留（`undoKeeper.ts`），換到別的分頁再回來 Undo 仍有效；分頁本身的新增、改名、搬動、刪除不在 Undo 裡。**
  Tests: web/src/editor/tabs.test.ts（搬動時有人在那個分頁打字、兩人同時搬動）, web/src/editor/TabList.test.tsx；e2e/tests/editor-tabs.spec.ts。

- **留言的錨點是 `{tab, block, quote}`（加上編輯器的 `start`、`end` relative position），標示跟著字走：別人在前面插字時跟著移、區塊搬動時（在 room 裡是刪除再插入）用區塊 id 與引文找回、字被刪掉時串與引文照舊並標「Original text changed」；標示對每個打開文件的人都畫（唯讀的人也是），點標示開那一串；錨點只由 `comments/anchor.ts` 建立與解讀，server 不看內容。**
  Tests: web/src/editor/comments/anchor.test.ts（moves with the words…、stays on the words when the block is moved、keeps its quote and is changed…、does not draw in the words typed right after it…、found from the block and the quote alone）, web/src/editor/comments/marks.test.ts, web/src/viewer/CommentsPanel.test.tsx；e2e/tests/editor-comments.spec.ts。

- **Docs keeps exactly one shared RoomPresence mounted using its existing provider: the host hides its normal live banner, while connecting, offline and reconnecting remain visible. Room awareness still publishes collaborators and save feedback stays beside the title; readers get the same connection exceptions without editing tools.**
  Tests: web/src/viewer/Editing.test.tsx（says it is connecting, shows no document until the room's state has arrived, and shows connection feedback only while connecting, offline or reconnecting、tells a read-only viewer when the connection is offline without adding editing tools）。

- **The plus menu and slash palette share Feature.insertions and the existing commands. Queries stay outside Y.Doc; Escape or dismissal inserts a cancelled slash query literally. Choosing a command creates its own undo step and uses the editor's current mapped selection, so peer edits during discovery remain intact. Slash guards use the actual input range and synchronize its selection before opening, even when browser selectionchange is pending. The anchored search receives focus once its visible positioning pass commits. Code, linked text and read-only views do not open slash discovery. More retains its formatting and history actions.**
  Tests: web/src/editor/InsertionMenu.test.tsx（keeps the selected words when a heading is chosen with the keyboard, persists it and undoes once、offers the same blocks from slash without saving the query, and restores literal slash text on Escape、maps the caret through peer edits while discovery is open and undoes only the chosen block change、wraps keyboard navigation and cancels an unmatched query as literal text after a peer edit、leaves slash as ordinary typing in %s、opens slash discovery after a hard line break without putting its query into the room、focuses the anchored slash search after its hidden positioning pass、uses the actual input range when the browser caret moves before the editor selection catches up、opens at ordinary input even when the stale selection is a code block node）、web/src/editor/features.test.ts、web/src/editor/InsertRow.test.tsx。

- **Date and Dropdown enter through the shared insertion registry and their real feature commands; opening a picker does not write a field. The Docs surface supplies Dropdown words from the current catalog through Hooks.dropdown.words. Saving uses the same room/model as other editing, so a reload sees the value.**
  Tests: web/src/viewer/Editing.test.tsx（chooses a real Date from plus, persists it through the room and undoes it once、uses slash and the surface's catalog words to insert a real Dropdown and reload its selected value）、web/src/editor/features/date/date.test.tsx、web/src/editor/features/dropdown/dropdown.test.tsx。

- **Date and Dropdown copy as validated visible values in text/plain for readers and editors; HTML retains field configuration and other leaves keep ProseMirror clipboard behavior.**
  Tests: web/src/editor/fieldClipboard.test.ts.

## 坑

- 新的編輯功能是 `features/` 裡的一個模組加 `features.ts` 的一行，不要把按鈕、按鍵或 plugin 直接加進 `InsertRow.tsx`、`FormatBubble.tsx`、`DocEditor.tsx` 或 `view.ts`；`features.test.ts` 擋重複的 id 與重複綁定的按鍵。

- 貼上的 HTML 在解析前先改名 `style` 屬性（`view.ts` 的 `readyForParsing`）；解析規則要讀 style 時用 `schema.ts` 的 `styleOf`，不要用 `element.style`，那在有 CSP 的頁面上是空的。表格的指令一律走 `table.ts`（儲存格沒有 id、對齊是整欄的）。

- 清單的指令一律走 `lists.ts` 的「讀成行、改行、組回樹」，不要直接改巢狀的節點樹；`list_item` 的 `checked` 只有待辦清單裡是布林，其他是 null（Go 與 room 都依賴這點）。新的區塊指令要保留區塊 id、文字，並在 `format.test.ts`／`lists.test.ts` 測套用、Undo、Redo、重新載入。

- `web/` 禁止 `innerHTML`／`outerHTML` 賦值、`insertAdjacentHTML`、`createContextualFragment`、`document.write` 與 `dangerouslySetInnerHTML`；這是 lint gate（`eslint.config.js`，`web/src/lint.test.ts` 測它），被擋時改寫成 DOM API，不要關掉規則。
- Docker 的 web stage 只複製 `internal/docroom/interop/schema.mjs`（編輯器 import 的共用 schema，套件從 `web/node_modules` 解析）；搬動或改名 schema 檔時一起改 `Dockerfile`。

- **Known OIDC people can be selected as mentions, persisted through the room and reloaded. Clipboard mentions retain only original directory-account identifiers or the exact assigned `oidc:[0-9a-f]{32}` grammar; arbitrary colon values become plain text.** Tests: web/src/editor/features/mention/mention.test.ts (selects a discovered OIDC account and preserves it across save, reload and clipboard parsing).
