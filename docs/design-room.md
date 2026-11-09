# design 在 room 裡（`internal/designroom`）

## Working capacity

A Design room holds all frames as editable nodes in one CRDT document. Its encoded working log is limited to **8 MiB**, including the last compacted state and every accepted update since. This is separate from the static limits of 60 frames, 512 KiB per frame and a 256 KiB manifest: valid source files do not guarantee that their expanded room and edit history fit. Generic document rooms retain their 2 MiB limit; incoming peer updates retain their 1 MiB limit.

The Design budget is an intentional modest working capacity for multi-frame documents, rather than a promise to materialize every maximum-size static Design. Seed, persisted-log load, replacement and edits enforce the same encoded bound. This is not a heap-size guarantee or a global room-admission policy; concurrent large rooms and expensive merges still need deployment resource monitoring.

An over-budget publish names the attempted encoded bytes and cap through HTTP and MCP. Retain the draft and reduce the proposed change if the existing room fits. If an existing persisted room exceeds the limit, loading is refused before replay; its log and epoch are retained, never reset or reseeded from an older version. An operator must review capacity and recover the retained log under a compatible bounded configuration before editing resumes. Saved-version reads are not recovery of unsnapshotted live changes. Rooms retained above 2 MiB but within 8 MiB load normally without changing history.


設計依據：#91、#99（行內文字）、spike #59 的 R6、#60。
格式見 [design-format.md](design-format.md)，room 本身見 [realtime.md](realtime.md)，版本怎麼從 room 來見 [doc-room.md](doc-room.md)（`internal/roomkind` 對每個種類相同）。
這份文件描述現在的樣子；畫面上的編輯控制見 [design 的 AGENTS.md](../web/src/design/AGENTS.md)，live 顯示的範圍見最後一節。
人在 UI 做的每件事，agent 都用 `read` 看結構、改 frame 的 HTML 再 `publish`：複製是多寫一份元素，群組是多一個包住它們的 `div`，層序是檔案裡的先後，圖片是 `upload_asset` 加 `<img src="/files/…">`。

一個 design 的 version 是 `design.json` 加每個 frame 的檔案。room 把它們放成一棵扁平的樹，再無損地寫回同樣的檔案：frame 進 room 再出來，逐 byte 相同（正規形式的 frame）。

## room 裡的形狀

room 的一個共享 `Y.Map`（名稱 `design`，`designroom.Map`）。為什麼扁平：搬動一個元素只寫它的 `parent` 與 `order`，不碰它的文字與樣式，所以「一個人搬、另一個人改字」兩邊都留著；每個樣式屬性自己一個 entry，兩個人改同一元素的不同屬性都留著。
不用 `Y.XmlFragment`，因為 outline 的重排與搬動很頻繁（`docs/spikes/realtime.md`）。

| key | 值 |
|---|---|
| `s\|page\|…`、`s\|row\|…`、`s\|frame\|…` | 頁面與 frame 的結構，每個值一個 entry，見「頁面與 frame」；`design.json` 是它們的投影 |
| `<file>\|shell\|…` | 一個 frame 的外殼：`htmlAttrs`（html 元素的屬性）、`pre`（html 底下 body 之前的節點，head 在裡面，寫成它本來的 HTML）、`post`、`docBefore`、`docAfter`、`root`（body 的 id） |
| `<file>\|n\|<id>\|kind` | `el`、`text` 或 `comment` |
| `…\|parent`、`…\|order` | 節點的位置：父節點的 id（body 是空）與小數索引（base 62 的字串，`between` 在任兩個之間都產得出一個，插入不改別人的 key） |
| `…\|tag`、`…\|ns`、`…\|attrs` | 元素的標籤、命名空間（SVG）與屬性清單（JSON，沒有 `style` 與 `data-pid` 的值，只在原位置放標記，所以屬性回來是原來的順序） |
| `…\|rest` | 這個元素的 `style` 屬性，編輯器擁有的宣告的值換成 slot，其餘宣告、空白、`!important`、順序都照原樣 |
| `…\|s\|<屬性>` | 編輯器擁有的 style 屬性的值（拖曳寫的 `left`、`top`、`width`、`height`，加上屬性面板寫的位置、版面、間距、外觀與文字的屬性；清單在 `web/src/design/properties/owned.json`，`internal/designroom/properties.go` 與 frame 的 inspector 各有一份，由測試比對）；同一個屬性在一個元素的 style 裡寫兩次（fallback）就不歸編輯器，留在 `rest`。圖片填色與適應方式也是（`background-image`、`background-size`、`background-position`、`background-repeat`，`<img>` 的 `object-fit`）：`background-image` 只收 `none` 或一串 `url()`，位址只能是 frame 本來就能指的（`#片段`、寫在標記裡的小圖、artifact 自己的檔案 `/files/<id>/<名稱>`），其他寫法（漸層、別處的圖、超過一千位元組的 `data:`）在原檔裡不算編輯器擁有、留在 `rest` 原樣。`display` 不歸編輯器擁有（沒有 slot，附加在 style 最後；屬性面板用它做 flex 容器）。每個值都經 `ValidateStyleValue`：只收編輯器擁有的屬性，長度、顏色、關鍵字、字型名稱、少數函式，不能含 `; { } < > \ @ url(` 這些，不合格的由 `Check` 擋（hub 不存、不廣播，與環相同）；`SetStyle` 也一樣 |
| `…\|hidden` | outline 的「隱藏」：`1` 隱藏（檔案裡該元素寫 `display:none`，原有的 `display` entry 保留，顯示時回來）、`0` 顯示元素自己標記隱藏的（寫 `display:revert`，除非有自己的 `display` entry）、沒有就是沒說；舊版用 `s\|display` = `none` 隱藏的 room 照舊讀成隱藏 |
| `…\|text` | 文字節點的文字，是 `Y.Text`（兩個人改同一段字會合併，改的是最小差異） |
| `…\|rich` | 元素的格式化文字（內容有 `<b>`、`<br>`、連結之類時），是一個帶屬性與 embed 的 `Y.Text`，取代這個元素的子節點，見下一節 |
| `…\|data` | 註解節點的文字 |

節點的 id：元素用它的 `data-pid`，文字與註解節點是 `~t1`、`~c1`…（`~` 不會是合格的 pid）。frame 寫進 room 之前先經過 `designfmt.Normalize`，所以每個元素都有 pid。

讀回來（`Room.Read`）是把節點依 `order`（相同時依 id）排成樹，建成 `html.Node` 交給 `html.Render`，外殼的字串當 raw 節點放回去；因為正規形式是 `Normalize` 的固定點，沒改過的 frame 逐 byte 相同。
父節點不見了的節點（一個人刪掉它的父、另一個人剛搬進去）放到 body 的最後，不遺失。

Covered border/background longhands remain literal when a later shorthand covers them. A final override after that shorthand is its own style slot; ordinary duplicate/fallback declarations remain literal. In rooms containing a covered slot, backend `SetStyle` and source merge write `…|override|<property>` instead of changing the authored `…|s|<property>` entry or the shared `rest`. These override strings use the same owned-property/value validation as style entries. Serialization retains the authored declaration and appends the override after shorthand. Direct Yjs clients must select the same key when a covered slot or an override already exists, expose override values in frame state, and replay overrides after authored styles. Deleting or emptying the override reveals the authored cascade; an action writes one property value in its normal local Undo transaction. Concurrent first edits share that property key, and Undo leaves later peer values intact. Source read/publish uses the same overrides; snapshots retain their winning declaration order, and source reload identifies the final override slot. Priority remains part of the value (`!important`); no shorthand is expanded. Tests: `TestDirectYjsOverridesKeepCascadeOrderThroughConcurrentEditsUndoRedoAndReload`, `TestCoveredBorderAndFillSourceEditsBecomeWinningOverridesOverHTTPAndMCP`.

## 格式化的文字

agent 寫的 frame 常有 `<div>Hello <b>bold</b><br>line2</div>` 這樣的文字。內容是文字、裡面有行內標記的元素，在 room 裡是**一個 `Y.Text`**（元素的 `rich` entry），不是一串子節點：字是文字，換行與不編輯的標記是 embed（各佔一個位置），每個被編輯的行內元素是蓋在它包住的那段字上的一個 format。
兩個人改同一段的不同字，改的是同一個文字、兩邊都留著；把一個字變粗是對一個範圍的 format，不是在樹裡搬節點。

**哪些元素是格式化文字**（`richOf`）：元素的內容全是文字、`<br>`、註解與行內元素，其中至少有一個字不是空白，而且至少有一樣東西不是純文字。內容有區塊（`div`、`img`、`svg`…）的元素不是：它的子元素各自是樹的節點，文字照舊是文字節點。
只有純文字的元素也照舊是一個文字節點，因為編輯器現在寫的就是那樣；第一次加格式時由編輯器把它換成 `rich`（寫 `rich`、刪掉原本的子節點，同一個 transaction）。元素同時有 `rich` 與子節點時，子節點接在 `rich` 的內容後面（不遺失）。

**行內的內容分三種**：

| 行內元素 | 在文字裡 |
|---|---|
| `b`、`strong`、`i`、`em`、`u`、`a`、`span`、`code`，且 `style` 只講字的樣子（顏色、字型、字距、底線、`white-space`…） | 被編輯：一個 format，屬性名 `f:<data-pid>` |
| `br` | embed `{"br": <屬性清單的 JSON>}` |
| `s small mark sub sup abbr cite q kbd samp var time del ins data dfn bdi bdo wbr ruby rt rp`、註解、沒有內容的行內元素 | 原樣保留、不編輯：embed `{"raw": <標記的字串>}`，一個位置 |

行內元素有版面（`position`、`left`、`width`、`display`、`float`…）就是設計的一個盒子，不是文字：它所在的元素不是格式化文字，這個元素仍是樹的節點，編輯器仍能搬它（絕對定位的 `span` 是常見寫法）。
format 的值是 JSON 的字串，與元素的 `attrs` 同一種寫法：`{"t":"b","d":0,"a":[["","data-pid","e3"]]}`；`t` 是標籤、`d` 是它在行內元素之間的深度（所以 `<b><i>x</i></b>` 與 `<i><b>x</b></i>` 各自原樣回來）、`a` 是屬性清單（含 `data-pid`，順序與寫的一樣）。
讀回來時，一段字的 format 依 `d` 再依 pid 由外往內開；範圍交叉的 format 會關了再開，產生同樣意思的標記（存成版本時 `Normalize` 讓重複的 pid 各自唯一）。不認得的屬性與 embed 讀取時忽略。`https:` 與 `mailto:` 的 `<a href>` 讀回來時一律是 `target="_blank"`、`rel="noopener noreferrer"`（`designfmt.OpenLinkInANewTab`，與發佈時相同），所以經 room 寫進來的連結與發佈的連結一樣在新分頁開、不帶 opener。

**編碼與 yjs 互通**：`internal/designroom/testdata/cases` 的每個檔案是一個元素的標記和它的 delta，兩邊共用：Go 寫的 room 在 yjs 裡讀出的 delta 要等於它（`interop/check.mjs`），yjs 用 `applyDelta` 寫的文字 Go 讀出的標記要等於它的標記（`interop/generate.mjs` 寫 `testdata/yjs`），還有兩個 yjs client 同時改不同的字、由 yjs 合併後 Go 讀到兩者都在。`make verify-interop` 檢查。

**每個人寫的都受同一份規則限制**：`Check`（hub 在保留更新之前跑）拒絕 format 不是上面那幾個標籤、屬性是事件或外部位址或 `javascript:`、`style` 有版面或外部位址，以及 embed 的標記有 frame 不能有的東西；被拒絕的更新不存、不廣播，與環一樣（`*realtime.RefusedError`）。發佈時同樣的規則由 `Validate` 擋，這兩處共用 `internal/designfmt` 的 `EditedInline`、`OpaqueInline`、`CheckAttribute`、`CheckFragment`。

**就地改字**（`web/src/design/text`）：雙擊文字，frame 裡的 inspector 讓那個元素可編輯，打完（Enter 或失焦提交、Esc 放棄）把內容回報成「文字、換行、原樣保留的標記，各自帶著它們在哪些行內元素裡」，頁面把它與開始時 room 的內容比較，只寫最小的差異：中間被取代的一段、以及留下來的字上不同的格式（`Y.Text` 的 insert、delete、format），一次動作是一步 Undo。別人在打字期間對同一段文字做的修改，位置依他們的變更換算後留著，兩邊的字都在。內容是純文字的元素維持一個文字節點，第一次加上格式或換行時換成 `rich`（寫 `rich`、刪掉文字節點，同一個 transaction，Undo 一起回來）；沒有內容的元素第一次打字時得到一個文字節點。只有內容全是文字（`rich`，或只有一個文字節點，或什麼都沒有）的元素可以就地編輯，其他的（有子元素、SVG、`img` 這類）說明原因、不進入編輯。畫布點到行內元素時（它有 `data-pid` 卻不是樹的節點），雙擊用最近的、是樹的節點的祖先；單擊的選取仍選到那個行內元素。frame 顯示的文字也來自 room（frame 的檔案只是 room 開始時的樣子）：每個只有文字的元素的內容以標記送給 inspector，它已經顯示那樣就不動，編輯中的元素不動，編輯結束後再送一次。

## 寫

寫入只經 `Hub.Edit`（plan 先讀、write 只寫，見 realtime.md）：

- `Room.Write(design)`：整份 design。內容沒變的 frame 不動（人正在裡面做的事留著），不同的 frame 整個換掉，design 沒有的 frame 刪掉；`manifest` 不同才寫。
- `Room.SetStyle(frame, id, prop, value)`：設一個編輯器擁有的屬性；空值移除那個宣告。
- `Room.EditText(frame, id, text)`：文字節點，只改不同的範圍（UTF-16 的第一個不同到最後一個不同）。
- `Room.Delete(frame, id)`：拿掉元素與裡面每個節點的每個 entry；body 不能刪。別人同時對被刪節點寫的 entry 沒有 `kind`，讀取時忽略。
- `Room.Move(frame, id, parent, index)`：只寫 `parent` 與 `order`；會讓元素成為自己的後代就是 `ErrCycle`，什麼都不改。

## 頁面與 frame

結構不是一個字串，而是每個值一個 entry（與節點樹同一個想法：兩個人同時改不同的值，兩邊都留著）：

| key | 值 |
|---|---|
| `s\|page\|<id>\|order` | 頁面存在；它在頁面之間的位置（小數索引，與節點相同的 `between`） |
| `s\|page\|<id>\|name` | 頁面名稱 |
| `s\|row\|<id>\|page`、`…\|order` | 列存在、在哪個頁面、在頁面裡的位置 |
| `s\|frame\|<id>\|file` | frame 存在；它的檔案 |
| `s\|frame\|<id>\|name`、`w`、`h` | 名稱與寬高（字串） |
| `s\|frame\|<id>\|row`、`order` | 它在哪一列、在列裡的位置 |

`design.json` 是這些 entry 的投影（`projectStructure`，瀏覽器的 `pages/structure.ts` 做同一件事）：頁面、列、frame 依位置排，沒有 frame 的列不畫，沒有 frame 的頁面不算頁面；掛在已刪的列或頁面上的 frame、沒有內容的 frame 不屬於這份設計（它的檔案也不進版本）。輸出是發佈者寫的那種緊湊 JSON，所以 `Same` 比較的是頁面與 frame，不是文字。
編輯器（`web/src/design/pages`）新增、改名、刪除、調整大小、重排頁面與 frame 時，一個 transaction 只寫被改的值，加上新增的 frame 的 entries（空 frame 是 `emptyFrameEntries`，與 `designfmt.Normalize` 的空 frame 讀回來是同一份）、拿掉被刪除的 frame 的所有 entries（`<file>|` 開頭的每一個，含 `Y.Text`）。所以一個動作是自己的一步 Undo，刪掉頁面再 Undo 連每個 frame 的內容一起回來。
同時結構化的人互相合併：兩個人各新增一個 frame（id 是隨機的）兩個都在；一個改名一個調整大小都留著；一個刪頁面、另一個正把 frame 搬進去，結果是兩邊相同的設計、沒有那個 frame，沒有任何拒絕。
舊的房間只有一個 `manifest` entry（整份 design.json）：讀的時候照它，第一次改結構時瀏覽器把它換成上面的 entry。
新增的 frame 在下一個版本存下之前不是任何版本的檔案：能進 room 的人從 live 位址（下一節）拿到它，server 從房間讀出那個 frame（`internal/web/frames_live.go`，與版本的 frame 同一個 sandbox 與 CSP），所以 room 裡的每個人立刻看到；版本的 grant（不論新舊、不論誰拿著）只讀它的版本，從不讀房間。

## 畫布上的 frame 從哪裡來

能進 room 的人（登入、看得到這個 design、看最新版本）的畫布從 **live 位址**載入 frame：`/raw/{id}/live.{密封的內容}/frames/<名稱>.html`，送的是 room 現在的 frame（沒有 room 時是最新版本的檔案；room 裡刪掉的 frame 就是沒有），與 MCP `read` 與 HTTP 讀檔案的 `file` 給同一個人的內容相同（同一個讀法）。帳號與到期以 AES-GCM 密封在位址裡（金鑰由 session 金鑰導出、artifact 是附加資料），因為 frame 自己的 script 讀得到自己的位址，不該知道是誰在看；它綁的是 artifact 與帳號，不是版本，所以 room 存成新版本時畫布不重新載入 frame：正在打的字、選取與 Undo 都留著。每次請求都以那個帳號重新判斷 `Viewer.CanSeeLive`，失去權限立刻 404；Play 也從 live 位址播放。匿名的 public 讀者、整頁的 `/raw/{id}` 與舊版本仍然用版本的 grant，看到的是存下的版本，不是 room 裡還沒存的內容。

frame 開著時，inspector 即時顯示 room 的樣式、隱藏、元素的順序、只有文字的元素的字、room 有而 frame 沒有的元素，以及圖片的 `alt`、`src`；其餘（frame 的外殼也就是 head、其他屬性、`rest`、標籤、命名空間、註解、混在元素之間的文字）只有重新載入才看得到。所以 room 改到這些、而且是 frame 開著時已經有的節點（`web/src/design/frameShape.ts`），畫布才重新讀一次 live 位址、重新載入那個 frame（讀不到 live 位址時，例如剛失去權限，frame 留著原樣）；frame 比 room 先載入的話，room 到了時再重新載入一次，因為兩者之間 room 可能改過，例如別人還原了不同 head 的版本，或 agent 改了 `class`；新增與刪除的節點 inspector 自己處理，outline 的名稱（`data-name`）frame 不顯示，都不重新載入。看不到 room 的人每個新版本照舊從那個版本的位址重新載入。

## 環與 hub 的檢查

兩個人各搬一次（A 放進 B、B 放進 A），單獨都合理，合起來是環。環要在 server 擋掉：`realtime.Config.Validate` 在每次對 document 的變更之後、保留之前被呼叫（`roomkind.Codecs.Check`，問每個有 `Check` 的 codec；design 的是 `designroom.Check`）。
變更被拒絕時它不存、不廣播，room 從記憶體丟掉並由 log 重載，document 回到變更前（`*realtime.RefusedError`，連線以 `ErrRoomReset` 結束，重連拿到存好的狀態）。
為此 `room.committed` 只記下更新，`settleLocked` 在操作的變更回來之後檢查、再存與廣播（在 transaction 裡讀 document 會永遠卡住）；回傳 nil 仍然代表那一列已在磁碟上。
`Check` 同時檢查結構的每一個值（`checkStructure`）：邊長要是 1 到 8192 的整數、名稱非空且至多 80 字、id 與檔名合格；不合格的更新同樣不存、不廣播，訊息列出違反的規則與限制。兩個人同時做的合法動作加起來的結果（超過 20 頁、一個頁面的 frame 都被刪光）不拒絕：那是一般的共同編輯，投影把不成立的東西略過，存成版本時才由 `Validate` 檢查。舊的單一 `manifest` entry 照整份格式規則檢查，沒有結構也沒有 manifest 的房間（還不是 design）不檢查。
`Read` 也不認環（回 `ErrCycle`），所以一個存不了的 room 不會變成版本。

每次變更都檢查整棵樹，成本與 room 的大小成正比；要更便宜得只在 `parent` 的 entry 改變時檢查。

## 插入、複製、群組與層序

這些動作都由編輯器（`web/src/design/ops`）直接寫 room 的 map，寫的 entry 與上面的表相同，一個動作是一個 transaction，所以是自己的一步 Undo、對所有人即時；server 不需要認得「這是複製」，它讀到的是節點。

- **插入**：文字、矩形、橢圓在 body 的最後加一個絕對定位的 `div`（`position`、`left`、`top`、尺寸與外觀各是 `s|` entry，沒有 `rest`；文字是一個子文字節點，`Y.Text`，所以之後可以就地改字）；圖片加一個 `img`，`attrs` 裡有 `src`（artifact 自己的檔案，`/files/<id>/<名稱>`）與 `alt`。新元素的 id 是 `n` 加隨機字元，文字節點的是 `~t` 加隨機字元；存成版本時照舊經 `Normalize`。圖片先走和檔案面板同一個上傳（server 的限制與拒絕原因原樣顯示），frame 裡只有位址、沒有位元組。
- **複製與貼上**：把元素與裡面每個節點的每個 entry 抄一份，換上新的 id（格式化文字裡行內元素的 id 也換，所以一個 frame 裡沒有兩個元素共用 id），放在原來的後面（貼上是 body 的最後）；絕對定位的元素的 `left`、`top` 各加 16 像素。貼上的是複製當下的樣子。
- **群組**：新的 `div`（`data-name` 是 Group）取代第一個選取元素的位置，選取的元素只改 `parent`；選取的元素都是絕對定位時，群組框蓋在它們的外框上、元素的 `left`、`top` 改成框內的位置，所以畫面上沒有東西動；解組把元素放回框的位置並加回框的位置，框的其餘（本身的文字除外，有文字的框拒絕解組）與空白文字一起拿掉。選取的元素要在同一個父元素裡。
- **層序**：後面的元素蓋在前面的上面，所以上移、下移、移到最上、移到最下只寫 `order`（`between`），被移動的元素之間維持原來的相對順序。

frame 是 version 的檔案，room 才是現況，所以 frame 裡的 inspector（`internal/kind/design_inspector.js`，由 `internal/kind/inspector/` 的模組建出）除了顯示樣式、隱藏與順序，還會依頁面的指示建出 room 有、檔案沒有的元素（標籤、屬性、不歸編輯器擁有的宣告、裡面的內容依序；格式化文字用 DOMParser 解析）並改 `<img>` 的 `alt` 與 `src`；它說「ready」時一併回報自己有哪些 `data-pid`，頁面才知道哪些要建。建出的元素和其他元素一樣被選取、設樣式與排序；Undo 讓它從 room 消失，frame 把它藏起來（`alive`）。
`Check` 對 room 裡每個元素也用發佈同一份規則：標籤要在 allow-list、屬性不能是事件或外部位址（`<img src>` 只能指 `#片段`、小圖或 `/files/…`）、id 與標籤的字元合格；不合格的更新被 hub 拒絕。

## 版本

`roomkind.Codec` 的內容是 `kind.Bundle`（內容加檔案文字）；design 的 codec 把 room 讀成 `design.json` 與每個 frame（每個 frame 再經 `Normalize`，經過和發佈同一套 `Validate`）。
room 安靜 2 分鐘就和最新版本比，不同才存成新版本，作者是編輯過的人；檔案經 `Service.SaveFromRoom` 的差異（有變的、新的、room 沒有了的是 null）進版本，所以沒動的 frame 是同一個 blob。
還原是 `Service.toRoom`：先把 room 裡還沒進版本的內容存成保護版本，再把還原的版本（內容加檔案）寫進 room，最後存還原的版本。room 從最新版本開始（`SeededLog` 讀版本的檔案）。

## agent 發佈進開著的 room

room 開著（有人開過、有 log）時，對 design 的 `publish`（帶 `files`）不另存版本：room 是工作中的那份，旁邊多一個版本會被它的下一個快照蓋掉，所以內容只經 room 寫。
`Service.publishIntoRoom` 把發佈驗證過（與沒有 room 時同一套規則），再以 `Room.Merge(base, next)` 合併，`base` 由逐檔 source read proof 決定，`next` 是它送來的樣子。`baseVersion` 仍獨立檢查最新 saved version，過期的照舊以 Conflict 拒絕並附最新版。
`base` 是 agent 明確帶回的逐檔 `sourceBases` 所指的 read snapshot，不是帳號或 client 的最後一次讀取。編輯者讀取現在的 manifest 與 frame 時取得 proof；`publish` 和大檔案的 `commit_publish` 帶回 `design.json`／frame 路徑對應的 proof。每次 read 獨立，綁定帳號、artifact、檔案、format 與 live epoch；缺少、過期、被淘汰或已失效時拒絕，保留草稿、重讀並合併，不回退到 saved version。唯讀與指定歷史版本的 read 不發 write proof。metadata-only 與相同內容不需要猜測 baseline；完整契約見 [design-source-edit.md](design-source-edit.md)。
`Merge` 以 `data-pid` 對應元素，**只寫 agent 相對於 base 改變的地方**，所以沒有改的就不碰：

| agent 做的 | 寫進 room 的 |
|---|---|
| 改一個 inline style 的屬性、屬性清單、tag、`rest`（style 的骨架） | 那個元素的那一個 entry；同一個值兩邊都改，agent 的贏 |
| 改一段文字 | 只改 agent 改的範圍（版本的字到 agent 的字之間），別人在範圍之外打的字留著；範圍重疊是 Conflict |
| 改格式化的文字 | 最少的差異（中間被取代的一段加上格式的變更，與編輯器的 `text/diff.ts` 同一個做法，兩邊跑 `testdata/diff` 的同一組案例），寫在房間的字現在的位置；別人在別處打的字留著，重疊是 Conflict |
| 新增元素 | 新的 pid 避開房間裡已有的（`designfmt.NormalizeAvoiding`），放在 agent 排的位置（`between` 在沒動的鄰居之間） |
| 搬動、重排元素 | 只寫 `parent` 與 `order`，沒動的鄰居與別人搬動的元素照舊；與別人的搬動合起來成環是 Conflict |
| 拿掉元素 | 那個元素與裡面的東西；agent 搬到別處的子元素除外 |
| 改 design.json | 只寫 agent 改變的 `s\|…` entry，別人同時新增的頁面與 frame 留著 |

別人刪掉的元素維持刪掉（agent 對它的改動略過）；沒有 `data-pid` 的 frame（整份重寫）等於每個元素都是新的，房間裡沒有別人對它的改動時整份換成 agent 的，有的話以 Conflict 拒絕，訊息要 agent 讀 live 的 frame 並帶著 `data-pid` 發佈。
編輯器不收的 style 值（`values.go` 的文法）與重疊的改動以訊息拒絕，room 不變（Conflict 要 agent 重新讀 room、在其上重做）。
發佈的結果是 `live: true` 與 frame 在 room 裡的樣子（`rewritten`，含 agent 新元素的 pid）；沒有新版本，版本是 room 安靜之後的快照，agent 列在它的製作者裡。`read` 的 `file` 讀的是 room 現在的樣子，`read` 的內容（design.json）也是。標題與可見範圍的改變照舊另存。

agent 在 room 裡以「<帳號> 的 <client>」出現，帶它改的元素（`touched: {frame id: [pid…]}`），幾秒後淡出，見 [realtime.md](realtime.md) 與 `web/src/design/AGENTS.md`。

## 共用 Token

`design` map 的 `t|--name` entry 保存每個 token 的 CSS 字串，與 `design.json.tokens` 雙向投影；每個名稱獨立，所以兩個人改不同 token 或 agent 只改其中一個時，其餘共用值保留。省略 tokens 的設計沒有這些 entries。`Check` 用 designfmt 的同一條值規則拒絕惡意 socket 更新；保存／還原使用既有 room snapshot 路徑。frame 原始碼一直保留 `var()`，只有顯示副本注入共用值。

前端透過 inspector 的 `tokens` 訊息換掉平台產生的 stylesheet，所有 ready frame 都收到，重新載入也重送；不改元素的 style 宣告。properties 的共用值用 `DesignEdits.transact`，跟元素編輯共用自己的 Undo。

## 還沒有的

- 畫面上的編輯控制（移動、調整大小、屬性、插入、頁面與 frame 結構、浮動工具列、留言釘、畫筆與 token）各節與 [design-format.md](design-format.md) 已描述；每個動作寫 room 的 entry 與上面的表相同。沒有的：貼到另一個設計裡、線、箭頭、便利貼（#136 不在 beta），行內連結與 Cmd+B、Cmd+I 以外的格式化沒有 UI。
- frame 在畫面上即時顯示的 room 變更是樣式、元素的順序、新增、刪除、文字，以及圖片的 alt 與 src。
