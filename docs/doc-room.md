# doc 在 room 裡（`internal/docroom`）

設計依據：#66（內容對應）、#67（版本）、spike #59（`docs/spikes/realtime.md`）。
區塊格式見 [doc-format.md](doc-format.md)，room 本身見 [realtime.md](realtime.md)。這份文件描述現在的樣子；還沒做的以 issue 標明。

編輯器寫 room，server 讀和寫 room，兩邊必須對「一份文件是什麼」有同一個答案。`internal/docroom` 是 server 的那一半：把格式 2 的 tabs 與 blocks 放進 room 的 `Y.XmlFragment`（y-prosemirror 的儲存形狀），再無損地讀回來，block id 全部保留。
編輯器的那一半是 `internal/docroom/interop/schema.mjs`（編輯器的 ProseMirror schema，R4 的編輯器直接 import 它）與 `model.mjs`（blocks 與 schema 之間薄薄的一層轉換）；兩邊用共用的 fixture 綁在一起。

## room 裡的形狀

分頁的順序與內容分開存，所以搬動分頁只動順序、不重建內容（別人正在那個分頁打的字不會掉）：

- 共用陣列 `tabs`（`docroom.OrderName`）：分頁 id 依順序。兩個人同時搬動可能讓一個 id 出現兩次，讀的時候每個 id 只算第一次。
- 每個分頁一個 fragment，名稱是 `tab:<id>`（`docroom.TabPrefix`）：它唯一的子節點是 `tab` 元素（下表）。編輯器一次只對一個分頁開 ProseMirror view，它的 doc 就是這個 fragment。
- 舊的形式（所有分頁都在一個 fragment `content`，`docroom.Fragment`）在 room 載入時由 server 搬成現在的形式（`roomkind.Migrator`），遷移那一筆寫進 log；Go 讀得懂舊形式。

| 位置 | 元素 | 屬性 |
|---|---|---|
| 分頁 fragment 的子節點 | `tab` | `id`、`title` |
| tab 的子節點（區塊） | `paragraph`、`heading`、`bullet_list`、`numbered_list`、`task_list`、`quote`、`code_block`、`rule`、`table`、`image`、`pending` | 每個都有 `id`；`heading` 有 `level`（數字）；`numbered_list` 有 `start`；`code_block` 有 `lang`；`table` 有 `header`（布林）；`image` 有 `src`、`alt`；`pending` 有 `intent` |
| list 的子節點 | `list_item` | task list 的項目有 `checked`（布林） |
| list_item 的子節點 | 第一個是 `paragraph`（項目的文字，沒有 id），之後可有一個同種類的 list（巢狀項目，沒有 id） | |
| table 的子節點 | `table_row` → `table_cell`（`align`）→ 一個 `paragraph`（沒有 id） | |
| 文字容器（paragraph、heading、儲存格的 paragraph）的子節點 | `XmlText`，以及夾在中間的 `hard_break` 元素 | |

- 程式碼區塊叫 `code_block`，不是 `code`：ProseMirror 的 schema 裡 node 與 mark 不能同名，而 `code` 是 mark。
- 空值不寫：空的 `lang`、`alt`、`align`、false 的 `header`、`start` 為 0，都是不存在的屬性，和編輯器對 null 的做法一致。
- 分頁（tab）是它自己 fragment 的唯一子節點，所以編輯器的 schema 照舊（`doc` 的內容是 `tab+`，一個 view 裡只有一個）；一份文件的所有分頁在同一個 room。
- **mark 是文字的屬性**：`XmlText` 裡每一段文字帶 `bold`、`italic`、`underline`、`strike`、`code`、`highlight`、`link` 這些屬性，值是 mark 的屬性物件（沒有屬性的是 `{}`，`link` 是 `{"href": …}`）。這是 y-prosemirror 的寫法，編輯器直接讀得懂。
- 文字裡的換行（`\n`）寫成 `hard_break` 元素，夾在兩段 `XmlText` 之間，也就是編輯器 Shift+Enter 的結果。

## 讀回來的樣子（正規形式）

room 分不出來的差別，讀回來就是同一個樣子（`docroom.Canon` 算出這個形式）：

- 一段文字的 mark 是集合，讀回來依固定順序（bold、italic、underline、strike、code、highlight、link）；
- 相鄰而 mark 相同的段落合成一段；
- 換行沒有字形，room 也不存它的 mark：換行帶著它前面文字的 mark（換行在行首時用後面的）；
- `numbered_list` 的 `start` 是 1 就當作沒有（編輯器的預設值）；
- 空陣列與沒有，是同一件事。

所以「區塊寫進 room 再讀回來，JSON 相同」的意思是：和 `Canon(原來的)` 相同；已經是正規形式的文件（Markdown 轉出來的、編輯器產生的）逐字相同。
讀的時候遇到 schema 沒有的東西（未知的元素或 mark、沒有 id 的區塊、不在區塊裡的文字），回傳以路徑開頭的錯誤（`tabs[0].blocks[2]: <video> is not a block of a doc`），不猜。

## 讀與寫（ygo 的坑，包一次）

- **在 transaction 裡查共享型別（`doc.GetXmlFragment`）或讀文字，永遠不會回來**（`realtime.md`）。`docroom.Open(doc)` 在 plan 階段查好 fragment；寫入是 `Content.Change`／`Content.Write` 回傳的函式，只持有已查好的 fragment，沒有任何查找或讀取的入口；
  寫入進行中對同一個 `Content` 呼叫 `Read`，立刻回 `ErrReadWhileWriting`，不會卡住。用法：`hub.Edit(ctx, room, who, func(doc) { c := docroom.Open(doc); … return c.Write(next), nil })`。
- 屬性有型別：`level` 是數字、`checked` 是布林。從 room 讀出來的數字可能是 `int64`（Go 寫的）或 `float64`（JavaScript 寫的），`GetAttribute` 還會把數字變成字串；`attrs.go` 讀它本來的值，型別不對就當作沒有。
- `Content.Write` 是整份取代：先刪掉 fragment 現有的子節點再寫入，所有區塊對在場的人都是新的。只改一個區塊的最小變更（人正在同一段打字時不丟字）是 `doc_edit` 的工作（#68）。

## 從最新版本開始

`roomkind.SeededLog` 包住 store 的 room log：一個 doc 的 room 第一次載入時若 log 沒有任何一列，就讀 artifact 的最新版本，轉成 room 的狀態（`Codec.Seed`），存成 log 的第一列（block id 保留），然後從那裡開始。
已經有 log 的 room 不再重來：log 是工作中的那份。沒有 codec 的種類（html、slides…），room 從空的開始。

## 版本從 room 來（`internal/roomkind`）

room 是工作中的那份，版本是它的快照；版本仍然 append-only、屬於 artifact（#27），從不被覆寫或間引。

- **什麼時候切版本：只有自動的兩種。** room 安靜 2 分鐘（`roomkind.Quiet`）之後，以及還原之前的保護版本。沒有「現在儲存」。2 分鐘是 `roomkind.Quiet`，`cmd/pictor` 照用；只有 smoke server（`internal/web/smoke_test.go`）讀環境變數 `PICTOR_SMOKE_ROOM_QUIET`（`web.Config.RoomQuiet`）縮短它，`e2e/playwright.config.ts` 設成 2 秒，所以瀏覽器測試裡的版本比真實部署早出現；staging 或本機以 `cmd/pictor` 跑時沒有這個覆寫。
- **誰在等：** hub 的 `OnUpdate` 通知 `Snapshotter.Updated`，它把「這個 room 有還沒進版本的編輯」和編輯者的帳號寫進 store（`rt_pending`、`rt_pending_authors`），並在記憶體記下最後一次編輯的時間。
  `Snapshotter.Sweep`（每 30 秒，`Server.Run` 呼叫）對每個等著的 room：安靜夠久就讀 room（`Codec.Content`，經過和發佈同一套驗證）、和最新版本比（`Codec.Same`），不同才存成新版本。和上一版相同的快照不寫，等待記錄照樣清掉。
- **作者：** 版本的作者是這段期間編輯過的、被認證的身分（連線被授權的帳號，或 `Hub.Edit` 的呼叫者：agent 的工具呼叫、還原），依第一次編輯的順序、每人一次；**不是 awareness 裡自報的名稱**，唯讀者被丟掉的寫入不算。
  `Version.author` 是第一位，`Version.authors` 是全部（發佈與還原的版本只有操作的人；舊版本遷移時補上它原本的作者）。
- **不遺失待存的編輯：** 編輯在 `rt_updates` 裡（回應之前就寫了），等待記錄在 store，所以最後一個編輯者離開、或 server 重啟，都不會遺失。重啟後記憶體裡沒有「多久沒編輯」，snapshotter 第一次看到這個等著的 room 的那一刻起算，再安靜 2 分鐘就成為版本。
  一次編輯被確認、到 `OnUpdate` 記下等待之間 process 當掉的話，內容仍在 log 裡，只是沒有等待記錄；那個 room 下一次被編輯時整份內容會進版本。
- **還原是寫進 room：** `Service.Restore` 在 room 有 log（開過）時：先 `Flush`（room 裡還沒進版本的內容成為保護版本，作者是那些編輯者），再把要還原的內容寫進 room（`Codec.Write` 經 `Hub.Edit`，作者是還原的人；在場的人即時看到，是一般的 Yjs 交易，可以復原），最後存一個還原的版本（`restoredFrom`）。
  舊版本一個都不動。room 的內容和最新版本相同，所以之後 room 安靜時不會再多一版。從沒開過 room 的 doc，還原只存版本，room 以後從最新版本開始。room 裡是 doc 不該有的內容（例如 `javascript:` 連結）時，保護版本不存、不擋還原，還原會蓋掉它。
- **別的種類接上來：** 要即時編輯的種類給一個 `roomkind.Codec`（`Seed`、`Content`、`Same`、`Write`；內容是 `kind.Bundle`，內容加檔案文字）並登記在 `DefaultCodecs`；snapshotter、起點與還原對每個種類相同。Docs 是第一個（`docroom.Codec`），Design 是第二個（[design-room.md](design-room.md)）。
- **存不了的 room：** room 裡是 doc 不該有的內容時不成為版本；重試的間隔每次加倍（安靜時間、2 倍、4 倍…，最長 1 小時），只有第一次記 error，之後是 debug；有人再編輯就重新從安靜時間算起。

## room 開著時整份寫入（發佈與還原）

不變式：**room 有 log（開過）時，doc 的內容先寫進 room，版本的內容就是 room 的內容。** room 沒開過就照舊直接存版本。

- 整份 `publish`（HTTP 與 MCP 同一個 `Service.Update`）：新內容經 `Content.Apply` 寫進 room，只改不同的地方；沒變的區塊不動、id 保留，正在打字的人不丟字，在場的人即時看到，然後存版本（內容與 room 相同，所以 room 安靜後不會多一版）。
  `baseVersion` 的規則和以前一樣，加一條：room 裡有還沒存成版本的內容時，先存成保護版本；呼叫者若帶了 `baseVersion`，這等於它沒看過這些內容，以過期處理（Conflict，附最新版），內容不遺失；沒帶 `baseVersion` 就在保護版本之上取代。
- 還原是同一條路（`Service.toRoom`），之後存一個還原的版本。
- `Content.Apply` 怎麼改：分頁的順序是 `tabs` 陣列（以 id 比對、最長共同子序列，搬動的分頁只有 id 被刪掉再插入，內容不動）；區塊以 id 比對（最長共同子序列，搬動的視為刪掉再插入）；清單項目與表格的列以內容比對，沒變的留著、變的換掉；段落、標題、程式碼區塊的文字就地改，從第一個不同的字到最後一個不同的字（以 UTF-16 計），範圍外的字（別人剛打的）留著。種類或 id 變了的區塊換成新的。
- 「room 的版本」之外讀取：`Service.Current` 給 room 的內容（有 room 時），`read` 與 `doc_edit` 從這裡開始，讓 agent 看到的是人們看到的。

## 與編輯器 schema 的共用 fixture

- `testdata/cases/*.json` 是共用的文件（手寫，正規形式，必須通過 `docfmt.Validate`，涵蓋每種區塊與 mark、emoji 與 CJK、換行、巢狀清單、兩個分頁、空文件）。
- `testdata/yjs/` 是**編輯器的 schema 經 y-prosemirror 寫出來的** update（`interop/generate.mjs`）。Go 讀它們必須得到 case 的 blocks（`TestGoReadsWhatTheEditorSchemaWrote`）；`make verify-interop` 重新產生並要求逐 byte 相同。
- `testdata/go/` 是 **Go 寫的** update（固定 client id，可重現；`PICTOR_UPDATE_FIXTURES=1 go test ./internal/docroom` 重寫）。`interop/check.mjs` 用編輯器的 schema 讀它們、`check()` 通過 schema 的內容規則，並要求轉成的 blocks 等於 case（`TestTheDocumentsGoWritesAreTheCommittedFixturesTheEditorSchemaReads` 固定 Go 端的 bytes）。
- 依賴釘版在 `interop/package.json`（`y-prosemirror 1.3.7`、`yjs 13.6.33`、`prosemirror-model`），`package-lock.json` 提交；升版要讓 `make verify` 全過。

## 還沒有的

- 局部修改不在這裡：`doc_edit` 對 room 做最小變更，見 [doc-edit.md](doc-edit.md)；編輯器見 [editor.md](editor.md)。這份文件的 `Content.Write` 與 `Content.Apply` 是整份寫入。
- 區塊沒有 id 時由誰補：編輯器的 id plugin 保證每個區塊都有；server 讀到沒有 id 的區塊會回錯誤，不自己補（補了也只在讀出來的那一份，下次讀又不同）。

## 行內日期

格式 2 的 `{"date":"2026-10-24"}` 對應到沒有子節點的 `date` 元素，只有字串屬性 `date`。
它是共用 schema 的行內 atom，不能帶 mark。相鄰日期不合併成文字；Go 與編輯器的互轉 fixture 包含段落、標題、清單、引用、儲存格與換行旁的日期。
只修改日期值時更新既有元素屬性，不重建周圍 `XmlText`，因此同段的並行輸入仍保留。
日期選擇器開啟時不改 room；提交時以 Yjs relative position 找回插入位置，以共用元素 identity 找回既有日期，已被刪除的目標不會改到鄰居。
