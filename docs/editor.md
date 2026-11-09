# Docs 編輯器（`web/src/editor`）

設計依據：#70、spec-docs E1、spike #59（`docs/spikes/realtime.md`）。
這份文件描述現在的樣子；還沒做的以 issue 標明。room 與 socket 見 [realtime.md](realtime.md)，doc 在 room 裡的形狀見 [doc-room.md](doc-room.md)。

## 它是什麼

viewer shell 裡的 ProseMirror 編輯器（不在 sandbox 裡：編輯器需要用登入者的 cookie 換 ticket，頁面本身不能）。同一個 doc 的所有人共用 server 的 room：
編輯器把 room 的 `Y.XmlFragment` 當作文件（`y-prosemirror`），改動經 `/ws/{id}` 即時送給別人，server 先存再廣播。

- 登入的人打開 `doc` 就直接進即時編輯器，沒有編輯與觀看的模式，也沒有 Edit／Done／Watch live 按鈕：能編輯的人（作者與編輯者，`artifact.canEdit`）直接打字；其他人進去的是**同一份 room 的唯讀即時畫面**：收得到所有編輯與游標，沒有插入列、不能打字（server 也會丟掉他們的寫入）。空的文件打開時游標已在裡面（新建的文件直接打字）。沒登入的人、舊版本（版本面板挑的）看的是頁面的 frame；載入編輯器失敗或失去權限時也退回 frame。
- 插入列：頂端列下方的細列，只有能編輯的人看得到：分頁 ▾（分頁的選單：選、新增、改名、搬動、刪除，讀者沒有列，多於一個分頁時分頁清單仍在頁面旁）、頁面寬度（窄／寬，每個人自己記在瀏覽器）、表格、圖片、待辦清單、＋（插入區塊）、更多 ▾。選單是按鈕展開的面板，按面板外或 Escape 就收。
- 「更多」裡的工具：刪除線（`Shift-Mod-x`）、highlight（`Shift-Mod-h`）、編號清單（`Shift-Mod-7`）、縮排與減少縮排（Tab、Shift-Tab）、引用（`Mod-Alt-q`）、程式碼區塊（`Mod-Alt-c`）、分隔線、Undo、Redo。套用只改文字的包法，不改文字；每一種都是作者自己的編輯，Undo 只復原自己的。待辦項目前面是真的 checkbox，點一下就存（唯讀的人點不動）。
- 區塊探索：＋和文字中的 `/` 共用 feature 的 insertion registry，提供項目／待辦／編號清單、程式碼區塊、日期、分隔線、下拉選單、標題 1–3、引用。搜尋名稱、縮寫或關鍵字，方向鍵循環選擇、Enter 套用、Escape 關閉；底部提示「在文件中輸入 / 查看更多」。斜線只在可編輯的普通文字、行首或空白後開啟，不攔程式碼、連結或唯讀畫面。搜尋字不進 room；取消時把 `/query` 當文字放回。命令套用到目前的 selection，別人同時插入的字會保留；一次 Undo 只復原該命令。圖片與表格使用工具列現有的流程。
- 連結：選字後 `Mod-k`（或工具列的 Link）開浮窗，填網址與文字，Save 後成連結；網址只收 `https:`、`http:`、`mailto:`、`#片段`，其他（`javascript:`、`data:`、沒有 scheme 的）留在欄位裡並在旁邊說原因，文件不變；Cancel 不改。游標在連結裡時工具列下面出現一條列：網址、Open（另開分頁）、Change、Remove（只拿掉連結、留下文字）。Undo 只復原自己的。
- 小工具列：選了字（不是游標）就浮在選取第一行的上方，只有一個：能編輯的人有粗體、斜體、底線、連結、區塊類型（段落、標題）、項目清單、code，能留言的人另有 Comment（兩者並存，同一個工具列；唯讀而能留言的人只有 Comment）。游標收合、正在打字（按了一個字元鍵、選取還沒變）時隱藏；上方沒有地方就移到選取下方，左右夾在頁面內。工具的鍵盤快捷鍵照舊。刪除線、highlight、數字與待辦清單、縮排、引用、程式碼區塊、分隔線、Undo、Redo 在插入列的「更多」。工具歸哪裡由 feature 的 `bubble`（工具列裡的順序）決定。
- 空白提示：只有一個空區塊的分頁（能編輯的人）在那個區塊顯示一行提示（`features/hint.tsx`，字是 `editor.emptyHint`），打第一個字就消失；有內容的文件、唯讀的人都不顯示。
- 連線狀態：只有連線中、重新連線與離線時顯示文件上方的提示；連線正常不另佔一列。離線時只有可編輯的人看到本機修改會在恢復連線後送出的說明。人員名單仍由 room awareness 送到共用頂端列；留言動作在選字後的小工具列或鍵盤快捷鍵。
- 儲存狀態：標題旁的 meta 顯示 Saving…／Saved／Offline／Not saved yet（離線而且有改動還沒存下），來自 room 的 `saved` 與連線狀態（`SaveState.tsx`），只有能編輯的人看得到。
- 表格：插入列的 Insert table 在游標所在的區塊後插入 3×3、有表頭的表（空段落會被取代；表在最後時後面留一個段落給你繼續打字）。游標在表裡時多一列工具：上／下加列、左／右加欄、刪列、刪欄、刪表、表頭開關、靠左／置中／靠右。對齊是整欄的（Markdown 只能這樣說），再按一次同一個對齊就取消；新列沿用各欄的對齊。Tab 到下一格、最後一格的 Tab 加一列並移到它的第一格、Shift-Tab 回上一格。最後一列、最後一欄不能刪（要刪就刪整張表）；欄上限 30、格上限 2000（doc 格式的限制），到了按鈕變灰。儲存格只有一個段落；自己的刪除範圍不能跨儲存格或從儲存格延伸到表外（否則表會被撕開），別人從 room 傳來的編輯不受這條限制。每一種操作都是作者自己的編輯，Undo 只復原自己的。
- 圖片：插入列的 Image 開檔案選擇器，貼上（剪貼簿只有圖片檔、沒有文字，例如截圖）與拖入（落在放開的位置）也一樣；每個檔案先上傳成這份 doc 的檔案（名字加 8 個字元，第二張截圖不會蓋掉第一張），上傳成功才把 `/files/{id}/{名稱}` 放進文件（選取該圖，工具列下面出現圖片工具）；server 拒絕（太大、太多檔、不是圖片）就把 server 的原因原文顯示在上方，文件不變，同一批的其他檔案照常加入。圖片工具：Alt text（按 Enter 或離開欄位才存）、Small／Medium／Large／Extra large（224、336、504、672 px，再按一次拿掉）、Delete image（Undo 復原，因為檔案仍屬於這份 doc）。檔案載入失敗（舊版本指向已刪的檔案，或位址不見了）畫「Picture deleted」（非本站的位址是「Picture not available」）加 alt 的佔位框。剪貼簿同時有文字（試算表複製的表格會附一張自己的圖）時走貼上規則，不當成圖片。
- 日期：從＋或 `/` 的 Date 開啟日期欄位，選好日期並 Save 才插入文字中的日期；Cancel 不改 room。點已有的日期可修改，Undo／Redo 只處理自己的變更，讀者看到相同日期而不能修改。Agent 使用同一份 inline date（[doc-format.md](doc-format.md)）。
- 下拉選單：從＋或 `/` 的 Dropdown 開啟欄位設定，填選項並選擇值，Save 才插入。點已有的欄位可改選擇或編輯選項；選項是 1–50 個不重複、無前後空白的單行文字，每個最多 100 字，最後一個不能刪除。取消不改文件，別人正在改的周圍文字保留；讀者看到所選的文字，不能開設定。Agent 經原有的 `read`／`doc_edit` 操作同一欄位。
- 提及人員：在文字中打 `@` 開啟人員清單，繼續打帳號或名稱縮小範圍；方向鍵選擇、Enter／Tab 或點擊插入，Escape 保留原文字。搜尋使用組織名錄（包含尚未登入 CodeXArtifacts 的同事）及 AD 給的名稱，至多八人；匿名與不能編輯文件的人不能搜尋。提及不改分享、不通知；選到無權開啟文件的人會顯示一則提示。輸入法組字的確認鍵不會選取人員。
- 留言：選字後按選取上方小工具列裡的 Comment 或 ⌘⌥M（見下面「留言」）。
- 舊版本以唯讀 frame 顯示；目前版本的登入讀者與編輯者維持 live room。Saved 表示 update 已持久化，room 安靜兩分鐘才切成 artifact 版本（見 [Docs room](doc-room.md)）。

## 程式怎麼分

| 檔案 | 責任 |
|---|---|
| `schema.ts` | 編輯器的 schema = `internal/docroom/interop/schema.mjs`（Go 與編輯器共用的那份）加上每個節點怎麼畫、剪貼簿的哪些標記看得懂。不另外定義節點、屬性或內容規則；`schema.test.ts` 要求兩邊的形狀相同 |
| `ids.ts` | id plugin：每個區塊都有 id（snapshot 拒絕沒有 id 的區塊）。編輯後沒有 id 或與別的區塊重複的（切開一段會複製屬性）拿到新的 `b_` 加 8 個字元。清單項目與表格儲存格裡的段落不給 id（模型沒有它們的 id） |
| `provider.ts` | socket 的瀏覽器端：y-websocket 的訊框、每次連線一張新 ticket、每個 close code 的處理、退避重連、離線／上線、roster 的 awareness、「已儲存」 |
| `view.ts` | 組成 view：schema、`ySyncPlugin`、`yCursorPlugin`、`yUndoPlugin`、id plugin，再加上每個 feature 的 plugin、鍵盤與 node view |
| `block.ts` | 區塊類型（段落、標題）的指令 |
| `feature.tsx`、`features.ts`、`features/` | 一個功能（格式、清單、連結、圖片、表格、分隔線、區塊類型、復原）是 `features/` 裡的一個模組，匯出它的按鈕（屬於插入列自己或「更多」）、插入列下的工具列、鍵盤、plugin 與 node view；`features.ts` 是唯一的清單，插入列與 view 都從它組出來，新功能是一個模組加那裡一行 |
| `linkEditing.tsx`、`imageAdding.tsx` | 連結浮窗與圖片上傳各自的 React 狀態（DocEditor 只呼叫它們） |
| `lists.ts` | 清單的指令：把選到的區塊讀成一行一個項目（有深度），改完再組回樹（巢狀清單只有一種型別，同一層取第一個被動到的項目的型別）；切換清單種類、Tab、Shift+Tab、Enter、Backspace |
| `format.ts` | mark 的切換、引用、程式碼區塊、分隔線、待辦的勾選；都保留文字與區塊 id |
| `link.ts` | 連結：網址檢查（與 server 同一份清單）、游標所在的連結範圍、設定與移除連結的指令；改文字時保留第一個字的其他 mark |
| `image.ts` | 圖片：插入、改 alt 與大小、刪除、貼上與拖入的檔案、上傳（唯一的名字）、載入失敗的佔位（`ImageView`） |
| `table.ts` | 表格：插入、列欄操作、刪除、表頭、整欄對齊、Tab 移動，與擋住跨儲存格刪除的 plugin；格子的段落沒有 id |
| `comments/` | 留言：`anchor.ts`（選取的字變成錨點、錨點在 room 裡找回字）、`marks.ts`（有留言的字畫成標示、點標示開串）、`useDocComments.tsx`（Comment 按鈕、標示與面板之間的接線）、`comments.css` |
| `LinkPopover.tsx` | Cmd+K 的浮窗（`LinkForm`）與游標在連結裡時的列（`LinkBar`） |
| `clipboards.ts` | 貼上測試用的剪貼簿：網頁、Word、Google Docs 各一份，照複製下來的樣子保留 |
| `DocEditor.tsx`、`InsertRow.tsx`（從 features 組出，不點名任何功能）、`RowPopover.tsx`、`editor.css`、`insert.css` | 畫面。`Viewer` 以動態 `import()` 載入 `DocEditor`，所以是獨立的 chunk |
| `fakeRoom.ts` | 測試用的 room 替身：照真的 server 的順序說同樣的訊框 |

## 載入與大小

編輯器是 Vite 的獨立 chunk（`DocEditor-*.js` 與 `DocEditor-*.css`），只在登入的人打開 doc 時才取：總覽與其他種類的 viewer 的 bundle 不含它
（`e2e/tests/editor.spec.ts`：總覽不載入 `DocEditor`，doc 頁載入一次）。
`vite build` 量到的大小：編輯器 chunk 259 kB（gzip 81.5 kB）加 CSS 6.5 kB（gzip 1.9 kB）；主 bundle 488 kB（gzip 153 kB）。
`vite.config.ts` 的 `resolve.dedupe` 讓共用 schema（它在 `internal/docroom/interop`，有自己的 `node_modules`）與 `web/` 用同一份 ProseMirror 與 yjs，bundle 裡每個只有一份。
版本釘死：`prosemirror-model 1.25.12`、`prosemirror-state 1.4.4`、`prosemirror-view 1.41.8`、`y-prosemirror 1.3.7`、`y-protocols 1.0.7`、`yjs 13.6.33`、`lib0 0.2.119`，與 interop 套件相同；`prosemirror-keymap`、`-commands`、`-transform` 另外釘。

## 不變式

- **編輯器和 renderer 只從驗證過的 schema 產生 DOM。** `web/` 禁止 `innerHTML`、`outerHTML` 的賦值、`insertAdjacentHTML`、`createContextualFragment`、`document.write` 與 `dangerouslySetInnerHTML`（`eslint.config.js` 的 `no-restricted-syntax`），`web/src/lint.test.ts` 用真正的設定對故意違規的程式碼跑 lint，要求每一種都失敗。
- **貼上的東西只經 schema 的解析規則。** 規則認段落、標題 1–6、項目與編號清單（含巢狀與起始號碼）、換行與 mark（粗體、斜體、底線、刪除線、code、highlight，也認 Word 與 Google Docs 放在 style 裡的粗體與斜體，以及 Google Docs 包住整份內容、自稱不是粗體的 `<b>`）；連結只在位址是 `https:`、`http:`、`mailto:`、`#片段` 時成為 link，否則留下文字；其他元素丟掉標籤、留下文字，`<script>` 與 `<style>` 連內容一起丟，Word 在每段後面的 `<o:p>` 在解析前拿掉（只動這個字串，剩下的仍經 schema）；沒有任何屬性從剪貼簿複製（節點的屬性來自符合的規則），所以事件屬性、style、`javascript:` 位址無處可去；表格（`thead` 或第一列全是 `th` 就是有表頭、欄位對齊取自 `align` 或 `text-align`、`colspan` 展開、短的列補齊、每格的段落以空白接成一段並保留 mark、超過欄與格上限的部分丟掉）成為 table 區塊；`<img>` 在位址是本站的上傳檔（`/files/{id}/{名稱}`）、`https:` 或小的 `data:image` 時成為 image 區塊（帶 alt），其他位址整個丟掉；引用與程式碼區塊還沒有規則，變成段落。`style` 與事件屬性（`onclick` 等）在解析前改名（頁面的 CSP 不讓套用或執行它們，瀏覽器每遇到一個就記一筆錯誤），規則從改名後的屬性讀粗體、斜體與對齊。
  schema 的 `toDOM` 另外只畫自己寫的屬性：連結只在 `https:`、`http:`、`mailto:`、`#片段` 時畫成 `<a>`，圖片只畫本站的上傳檔、`https:` 或小的 `data:image`，room 裡有別的值也不會變成位址（畫佔位框）。
- **每個區塊都有唯一的 id**，由編輯器保證；全選貼上不會換掉分頁（全選只選目前分頁的文字，分頁 id 不變）。
- **在 room 的內容到達之前不建 view。** 在空的文件上建 view，編輯器會把一頁空白寫進文件，之後與 room 的內容合併成第二份。`LiveProvider.synced` 在第一個 step 2 到達後才為真，畫面此前只顯示「Connecting…」。
- **別人的游標是沒有子節點的元素**，名字由樣式表從 `data-name` 畫出：瀏覽器的游標可以進入游標裡的文字節點，那樣旁邊的人打的字會落進游標而不是文件。
  名字是 server 從連線身分寫進 roster 的（`realtime.md`），不是瀏覽器自報的；顏色是 awareness 裡的調色盤名稱（`c0`–`c7`，由帳號算出，只認這八個），顏色值在 `web/src/presence/presence.css`，因為 CSP 不允許 style 屬性。
- **別人的編輯不會把你的游標拉回去。** ProseMirror 從 `selectionchange` 事件得知點擊或按鍵，那是獨立的 task；遠端編輯若先到，y-prosemirror 會以點擊之前的選取範圍還原，接著打的字就落在舊位置（例如文件開頭）。
  `view.ts` 的 `adoptBrowserSelection` 在每個非 view 自己造成的 Yjs transaction 之前先採用瀏覽器目前的選取範圍（`view.test.ts` 的 `a person's caret when someone else's edit arrives`、`e2e/tests/editor.spec.ts` 的「clicks into a paragraph while someone else is typing」）。
- **Undo／Redo 只復原自己的編輯**（`yUndoPlugin` 只追蹤本機的來源），別人打的字不動。

## 連線與「已儲存」

- 每次連線（含每次重連）先 `POST /api/artifacts/{id}/live` 取新 ticket，再開 `/ws/{id}?ticket=…&epoch=…`（`epoch` 是這份本機文件記得的）。ticket 被拒（登出、403、404）就停下、顯示 Offline 並通知 viewer；其他失敗退避重試（0.5 秒起、每次加倍、最長 15 秒）。
- close code 的處理照 `realtime.md` 的表：4001 與 4002 丟掉本機的 `Y.Doc`、從 room 的重來（view 重建）；4403 請 viewer 重讀 artifact（`canEdit` 可能變了，編輯器因此變成唯讀或不再顯示工具）並立刻換新 ticket；其他退避重連。
- 瀏覽器離線（`offline` 事件）就關掉 socket、停止重試、顯示 Offline；回到線上立刻重連。離線時打的字留在本機的 `Y.Doc`，重連的同步握手把它送出去，不會遺失。
- 狀態：Connecting…（還沒連上過）、Live、Reconnecting…、Offline。
- **Saved** 的意思是 server 已經存下這個瀏覽器的所有改動。協定沒有 ack，所以每個本機的 update 之後多送一個 sync step 1：server 依收到的順序處理，回的 step 2 到達就表示前面的 update 已寫進 `rt_updates`（`realtime.md`「已確認」）。
  server 在有人加入時會不請自來送一份整個狀態的 step 2，provider 略過每條連線的第一個（`room.go` 的 `addPeerLocked`）。重連時對 server 的 step 1 的回覆也同樣多送一個 step 1 當探針。唯讀的人沒有「已儲存」。

## 瀏覽器測試的 harness

`e2e/tests/editor-support.ts` 是所有編輯器 issue 共用的：`browse`（一個人在自己的瀏覽器 context 裡登入，頁面受 console error 規則管）、`publishDoc`（作者以 Markdown 發佈並指名編輯者）、`openEditor`（進編輯器並等到連上）、`typeAtEnd`、`expectBlock`、`caretOf`、`blocksOf`、`agentOf`（同一個人的 token 與 MCP 工具）。
`e2e/tests/editor-images.spec.ts`：貼上圖片、設 alt 與大小、重新載入、第二個瀏覽器即時看到、連貼兩張是兩個檔案、檔案選擇器與 Delete／Undo、拖入、server 拒絕的原因、agent 上傳的圖片在編輯器與 viewer 的 sandbox 都畫得出來、檔案被刪之後兩邊都是佔位。
`e2e/tests/editor-tables.spec.ts`：插入、Tab 填格與加列、整欄對齊、agent 讀到的 pipe table、兩個人同時改不同格與一人加列一人打字、從網頁貼表、agent 用 doc_edit 加的表。
`e2e/tests/editor-links.spec.ts`：Cmd+K 存連結、被拒的網址、點連結後改與移除、Cancel、agent 的 script 網址被拒。
`e2e/tests/editor.spec.ts` 用它做：兩個瀏覽器在不同區塊打字且互看編輯、游標與名字、Saved；第三個唯讀的瀏覽器即時看到、打不了字；一個離線、兩邊都編輯、重連後都在；Undo 不動別人的字；總覽不載入編輯器 chunk；agent 發佈的文件開在編輯器裡。
smoke server 像 `cmd/pictor` 一樣跑 `Server.Run`（房間會被收拾、安靜的房間會被切成版本），`PICTOR_SMOKE_ROOM_QUIET`（Go 的 duration，`e2e/playwright.config.ts` 設成 2 秒）縮短那 2 分鐘的等待，對應 `web.Config.RoomQuiet`；
「兩個人的編輯成為一個版本、名單裡有兩位作者、版本面板不重載就標出最新版、還原時兩邊即時看到」在 `editor.spec.ts`。版本面板的最新版取自 server 回的清單與 viewer 手上的 artifact 兩者較新的，所以房間在編輯器開著時切出新版本，面板也不會把舊版標成 Current。

## 還沒有的

- 留言的解決、編輯與篩選：#50。
- 圖片的裁切、替換（beta 之後）；游標在圖片前後的位置（圖片是整個區塊，選取它才能刪除）。
- 表格的儲存格顏色、合併、欄寬、排序、移動：之後（beta 之後）。
- 從 Word 桌面版貼上的清單（`mso-list` 的段落，沒有 `<ul>`）：變成段落；網頁與 Word 網頁版的 `<ul>`／`<ol>` 照常。
- 真正的 IME 驗證：beta 前的人工項目，腳本與記錄表在 [acceptance-ime.md](deployment.md)（#107）。

## 分頁（#76）

一份 doc 有一或多個分頁（扁平，沒有子分頁、複製分頁、已刪分頁還原）。編輯器一次只對一個分頁開 view（每個分頁是 room 裡自己的 fragment，`tabs.ts`）；其餘分頁仍在 room 裡，只是這個人沒有開。正在看哪一個是每個人自己的，不同步；被別人刪掉時退回第一個。順序是 room 裡的 id 陣列，標題在分頁元素上。
頁面旁的分頁列（`TabList.tsx`，唯讀的人只有選分頁，只有一個分頁的唯讀文件不顯示）：新增（加在目前分頁之後，從一個空的 H1 開始、游標在裡面）、改名（按鈕或連點兩下；標題要是文件存得下的：一行、非空、至多 200 字）、上移／下移、刪除（問一次，Cancel 保留）；每一項都是 room 裡的一次編輯，所以對所有人即時、重新載入後順序與內容不變（分頁操作本身不進 Undo）。搬動只動 id 的順序，搬動當下別人在那個分頁裡打的字都留著（agent 的 `tab_move` 同）。
Agent 用 `doc_edit` 的 `tab_add`、`tab_rename`、`tab_delete`、`tab_move`（[doc-edit.md](doc-edit.md)），`read` 兩種 view 都列出每個分頁的 id 與標題。
Tests：`web/src/editor/tabs.test.ts`、`web/src/editor/TabList.test.tsx`、`e2e/tests/editor-tabs.spec.ts`。

## 從網站新增與改名（#77）

總覽頂端的「New」選單：「Document」（私人、一個分頁、一個空的 H1，開在編輯器且游標在 H1）與「Design」（私人、一頁一個 1440×900 的空 frame）。`POST /api/new {kind, title?}`（`Service.CreateBlank`）；agent 的路徑是 `publish`。
作者與編輯者在 viewer 標題上改名（`PUT /api/artifacts/{id}/title`，`Service.Rename`）：是一個新版本，內容與檔案和最新版本相同，文件自己的 H1 不動；同一個標題不產生版本；讀者 403，看不到的人 404。Agent 用 `publish` 帶新 `title`。
Tests：`internal/web/create_test.go`、`web/src/create/Create.test.tsx`、`e2e/tests/create.spec.ts`。


## 留言（#81）

選字（編輯者與唯讀的人都可以，只要 `canComment`）後按選取上方小工具列裡的 Comment（與格式工具同在一個小工具列，唯讀的人只有 Comment；位置規則見「小工具列」）或 ⌘⌥M（編輯器有焦點時）：右側的留言面板開著，帶著「對這段文字留言」與引文，寫完 Send 就是一串新留言，錨在那段字上。有留言的字在編輯器裡畫成標示（所有人都看得到），點標示開啟面板、並把那一串標成目前的；面板裡點引文則反過來讓標示變強（字在別的分頁時切到那個分頁）。留言面板與 `comments` 是通用的，doc 只決定錨點長什麼樣。
- **錨點**是 `{tab, block, quote, start, end}`（`anchor.ts`）：`tab` 是分頁 id、`block` 是字開頭的區塊 id、`quote` 是那段字（最多 300 字元，錨點整體上限 2000 bytes）。agent 讀這三個；`start`、`end` 是編輯器加的兩個 Yjs relative position（base64），`end` 黏在最後一個字上，所以在引文之後或之前打的字不會被拉進標示。agent 寫的錨點只有前三個。
- **標示跟著字走：** 先用 `start`／`end` 在 room 裡解開，別人在前面插字、同一段改別處，標示都跟著。區塊被搬動時在 room 裡是刪除再插入（新的 Yjs 項目、同一個區塊 id），位置解不開了，就用區塊 id 與引文再找一次；兩個都找不到就是字已經不在了。字只被刪掉一部分時，標示縮成剩下的那部分。
- **字被刪掉：** 串與引文照舊在面板裡，標「Original text changed」（原文已被修改）。是否還在由 room 回答（`standing`，每個分頁都問，不只畫面上那個），編輯器每次 room 更新時重算並告訴 viewer；沒開編輯器時面板只有引文、沒有這個標記。
- **兩個來源：** 留言串由 viewer 讀（`useComments`），面板與編輯器畫同一份；新留言（別人的）使留言數改變，編輯器開著時就重讀、標示出現。
- Agent：`comments` 回 `anchor`（tab、block、quote），`comment` 帶同樣形狀的 `anchor` 就能對一段字留言，`changes_since` 列出新留言；doc 的 Guide 說明形狀。
- 還沒有：解決、編輯、刪除與篩選（#50）；選取跨區塊時 `block` 是開頭的區塊，區塊搬動後的比對只在那個區塊找引文；不在畫面上的分頁只顯示引文與標記、不畫標示。
Tests：`web/src/editor/comments/anchor.test.ts`、`web/src/editor/comments/marks.test.ts`、`web/src/editor/FormatBubble.test.tsx`、`web/src/viewer/CommentsPanel.test.tsx`、`e2e/tests/editor-comments.spec.ts`。
