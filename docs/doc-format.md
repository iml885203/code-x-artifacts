# doc 內容格式 2（區塊）

設計依據：#64。`doc` 的內容格式是 `2`：帶穩定 id 的區塊 JSON，本文件定義它。格式 1（Markdown）已移除：啟動時的遷移把每個 doc 的每個版本轉成區塊（見「遷移」），Markdown 只是 agent 寫入、讀回與匯出的形式。
實作在 `internal/docfmt`（模型、驗證、renderer），`internal/kind/doc.go` 只負責把它接到 `Kind` 介面。

格式 2 的形狀是 CodeXArtifacts 自己的 schema，可以和 ProseMirror 的 schema 互相對應，但儲存的永遠不是編輯器的內部 JSON。
區塊有穩定的 id，是為了之後只改一段而不重寫整份、留言能錨定、兩個人改不同段落不會互相蓋掉。

## 形狀

```json
{"tabs": [
  {"id": "t_x1", "title": "Overview", "blocks": [
    {"id": "b_k3f9", "type": "heading", "level": 2, "content": [{"text": "Goals"}]},
    {"id": "b_m2a1", "type": "paragraph", "content": [{"text": "Ship "}, {"text": "beta", "marks": ["bold"]}]},
    {"id": "b_p0w6", "type": "pending", "intent": "Goals: the three outcomes this quarter commits to"}
  ]}
]}
```

- **分頁（tab）**：`tabs` 是扁平的清單，至少一個、至多 50 個；每個有 `id`、`title`（非空、一行、不含 `-->`、至多 200 字）、`blocks`。
  `children`（巢狀分頁）保留但必須是空的，目前送了會被拒絕。
- **id**：分頁與區塊的 `id` 是 1 到 64 個字母、數字、`-` 或 `_`，整份文件內不重複。送出時沒帶 id 的分頁和區塊，由 server 補上（`t_`、`b_` 加隨機字元）；
  帶了的照用。要保住留言錨點與合併，重新發佈時請帶原來的 id。
- **行內文字（`content`）**：一串 run，`{"text", "marks"?, "href"?}`。`text` 不可為空、不可含 NUL；文字裡的換行（`\n`）是換行。
  `marks` 是 `bold`、`italic`、`underline`、`strike`、`code`、`highlight`、`link`，同一個 run 不重複。`link` 一定要有 `href`，沒有 `link` 就不能有 `href`。

## 日期欄位

`content` 接受 `{"date":"2026-10-24"}`，可放在段落、標題、清單項目、引用與表格儲存格的文字之間。
日期必須是有效的 `YYYY-MM-DD`（年份 0001–9999，依日曆檢查閏年），不接受時間、時區、空值或其他 run 欄位。
編輯器以日期選擇器插入與修改；讀者看到日期文字。欄位與周圍文字共用既有 room、版本與權限，日期操作有自己的 Undo 步驟。
Agent 使用 `read` 的 `view: "blocks"` 讀回欄位，以 `doc_edit` 的 blocks 與 `ifHash` 修改所在區塊並保留區塊 id。
Markdown、Word 與 PDF 只保留目前可見的日期文字；重新發佈 Markdown 會得到普通文字，保留可編輯欄位請使用 blocks。
範圍不包含提醒或進階日期格式（#195）。

## 提及人員（mention）

`content` 也接受 `{"user":"anna.lee","name":"Anna Lee"}`，可放在段落、標題、清單項目與表格儲存格，與文字 run 相鄰。`user` 是必填的登入帳號（1–64 個英文字母、數字、`.`、`_`、`-`）；`name` 是提及時的名稱（非空白、單行、至多 100 個 Unicode 字元，不含 NUL 或控制字元）。mention 不接受 `text`、`marks`、`href` 或未知欄位；拒絕訊息包含位置，例如 `tabs[0].blocks[2].content[1].user`。

Markdown 寫成 `[@Anna Lee](mention:anna.lee)`，讀回保留帳號與名稱；一般 Markdown 閱讀器仍能顯示 `@Anna Lee`。名稱裡的 Markdown 字元由匯出器跳脫。唯讀頁面、PDF 與 Word 顯示 `@` 加儲存的名稱，沒有連結；名稱始終是文字；唯讀頁面與列印以低彩度文字保留提及時的名稱，離開名錄也不改寫文件。提及不授予權限，也不發送通知（#144）。

人員搜尋使用登入時在同一條已綁定 AD 連線上讀取的組織名錄，包含尚未登入 CodeXArtifacts 的同事，且遵守既有允許群組範圍。服務不保存 AD 密碼或連線；名錄資料不參與登入或 artifact 權限判斷。讀取上限為 20 頁、每頁 500 人、共 10,000 人與三秒；只有最後分頁確認結束且所有資料有效的快照才會原子地更新名單。逾時、範圍不明、轉介、分頁不完整或無效資料只會讓人員清單標示不完整，不阻擋原本成功的登入，也不把缺少的人誤判為離職。既有名單可繼續搜尋；名錄範圍已變更的讀取若失敗，則不顯示舊範圍的名單。

## 區塊

| `type` | 欄位 | 備註 |
|---|---|---|
| `paragraph` | `content` | |
| `heading` | `level`（1 到 6）、`content` | 頁面的標題取自第一個 heading；每個 heading 在頁面上有以文字產生的錨點（`#goals`，重複時 `-1`、`-2`） |
| `bullet_list`、`numbered_list`、`task_list` | `items`（至少一項）；numbered 另有 `start` | 每一項 `{content?, checked?, items?}`，`items` 是巢狀清單（同一種）；`checked`（true 或 false）只有 task list 有，而且一定要有 |
| `quote` | `blocks`（至少一個） | 裡面可放除了 `pending` 以外的區塊 |
| `code` | `lang?`、`text` | `lang` 是 `go`、`c++` 這類名稱 |
| `rule` | | |
| `table` | `header?`、`rows` | `rows[].cells[]` 是 `{align?, content?}`，`align` 是 `left`、`center`、`right`；每列的格數要和第一列相同；`header` 為 true 時第一列是表頭 |
| `image` | `src`、`alt?`、`size?` | `src` 見下面；`size` 是 `small`、`medium`、`large`、`xlarge`（顯示寬度 224、336、504、672 px），沒有就是圖片本身的寬度（最寬到頁面） |
| `pending` | `intent` | 骨架的佔位：之後會填什麼。頁面顯示灰色的虛線框和 `intent`，不是錯誤。`intent` 是一行、不含 `-->` |

未知的欄位一律拒絕，訊息列出這個位置允許的欄位。

## 連結與圖片

- 連結只接受 `https:`、`http:`、`mailto:` 和 `#片段`。`javascript:`、`data:` 等其他 scheme 與相對位址都被拒絕；位址不可含空白或控制字元、至多 2048 位元組、`https:` 與 `http:` 要有主機。
  文件外的連結在新分頁開啟。
- 圖片 `src` 只接受三種：`https:` 位址、上傳到這份文件的檔案（以 `/files/` 開頭，或上傳工具回傳的完整位址）、以及不超過 256 KB 的 `data:image/png|jpeg|gif|webp;base64,…`。
  圖片屬於 assets，不屬於版本（見 [multi-file.md](multi-file.md)）。
  所以舊版本（或檔案被刪掉之後的目前版本）可能指向已經不在的上傳檔：頁面（`/raw`）在那個位置畫「Picture deleted」（中文頁畫「圖片已刪除」）加 alt 的佔位框，不畫壞掉的圖（`docfmt.MissingPictures`，由 server 依這份文件現有的檔案決定）；編輯器的圖片載入失敗時也畫同樣的佔位。別的網站的 `https:` 圖片不查，交給瀏覽器。

## 限制

整份 JSON 至多 2 MB（`docfmt.MaxBytes`）、50 個分頁、5000 個區塊、5000 個清單項目、巢狀 8 層（quote 與清單的巢狀都算）、每張表 30 欄與 2000 格。
超過時的訊息說出是哪個限制和現在的值：`this doc has 5001 blocks; at most 5000 are allowed`。
每一則驗證錯誤都以路徑開頭，如 `tabs[0].blocks[2].content[1].href: …`。

## 發佈

寫入的方式有兩種，結果是同一種儲存內容。

- **Markdown**：`content` 放 Markdown，格式 2 的文件存的是轉換後的區塊。`format` 不用帶；帶 `"1"` 會被拒絕，訊息指向格式 2。
- **區塊**：HTTP `POST /api/artifacts`、`PUT /api/artifacts/{id}` 與 MCP `publish` 用 `blocks` 取代 `content`：
  一個區塊清單（成為唯一的分頁，標題 `Document`），或完整的 `{"tabs": [...]}`。沒帶 `kind` 時 `blocks` 表示 `doc`，沒帶 `format` 時表示格式 2。

`content` 與 `blocks` 不能同時送。
存下來的內容是正規形式（固定的欄位順序、沒有多餘空白、`&` 不跳脫），所以內容相同的文件位元組也相同。

## Markdown

`docfmt.FromMarkdown` 與 `docfmt.ToMarkdown` 互為反函數：`read`、`GET /api/artifacts/{id}/versions/{n}`、衝突時回的 `current` 與匯出 `md` 對格式 2 的文件都給 Markdown（`service.Written`），
所以 agent 讀、改、再發佈不需要懂區塊。Markdown 轉區塊再轉回 Markdown 再轉區塊，得到相同的區塊。

- **語法**：CommonMark 加 GitHub 的表格、刪除線、任務清單、裸網址。原始 HTML 一律不讀（被略過），只有兩種註解行有意義：
  `<!-- pictor:pending 意圖 -->` 是 `pending` 區塊（`意圖` 要有內容；不能放在 quote 裡）；`<!-- pictor:tab 標題 -->` 開始一個分頁（只有一個叫 `Document` 的分頁時不寫）。
- **表格**：第一列是表頭（`header` 為 true）；對齊記在每個儲存格上，轉回 Markdown 時以每欄第一列的為準。
- **圖片**：段落裡的 `![alt](url)` 把段落切開，圖片成為自己的 `image` 區塊；大小寫在 Markdown 的標題位置，`![alt](url "large")`（只有四個大小名稱算，其他標題忽略），轉回 Markdown 時照寫；標題、清單項目與表格裡的圖片只留 `alt` 文字。
- **清單項目**只放文字：項目內的第二段以換行接在後面，巢狀清單成為項目的 `items`，其他區塊（程式碼、引用）變成文字。
- **文字裡的換行**：Markdown 的硬換行（行尾 `\` 或兩個空白）是 `\n`，軟換行是空白。
- **連結與圖片的規則**與 JSON 相同；不合格的位址使整個轉換失敗，訊息以 `line N:` 開頭並引出位址與原因。
- **沿用 id**（`docfmt.KeepIDs`）：重新發佈 Markdown 時，內容沒變的區塊沿用原 id。做法是把新舊區塊以內容比對（最長共同子序列，所以插入或刪除一個區塊不會讓其他區塊換 id），
  沒被比對到但移動過的區塊以內容找回原 id，夾在兩個比對之間、型別相同的區塊視為被修改的同一個區塊，保留 id。分頁先以標題、再以位置比對。引用裡的區塊遞迴比對。
- **Markdown 裝不下的**（底線與螢光標記、空段落、沒有表頭的表格、同一欄不同的對齊、表格與 code span 裡的換行）變成純文字或省略，文件最後以一行註解 `<!-- pictor: Markdown cannot hold … -->` 說明。
  Markdown 不會表達的字元（`*`、`_`、`[`、行首的 `#`、`-`、`1.`、會被當成連結的網址與 `www.`）都以反斜線跳脫，使文字回來還是同一段文字。

## 匯出

viewer 的 Export 選單的 Markdown 是 `ToMarkdown`，另外把上傳到文件的圖片（`/files/…`）寫成完整位址（`https://主機/files/…`），檔案離開 CodeXArtifacts 後仍然開得起來；
只在 server 以 https 連線時這麼做，因為文件不收 `http:` 的圖片，寫了就不能再發佈。匯出的 Markdown 再發佈，得到相同的區塊，除了檔尾註明的降級與圖片的完整位址。匯出不改文件。

## 畫面

`docfmt.RenderPage` 把區塊畫成唯讀頁面，沿用閱讀樣式（`internal/kind/doc.css`）再加上 `blocks.css`（pending 佔位、任務清單、表格對齊、分頁標題）。
每個字串都經 HTML 跳脫，標記的屬性只有 renderer 自己寫的（`data-block`、`id`、`class`、`href`、`src`、`alt`、`start`），沒有原始 HTML；
renderer 對已存的資料再檢查一次連結與圖片位址，不合格的連結只留文字、不合格的圖片留下說明文字。
多個分頁時每個分頁是一個 `<section class="tab">`，帶它的標題；單一分頁不加外框。頁面仍經 `/raw/{id}` 的 sandbox 與 bridge。

## 遷移

啟動時 `store` 的最後一個 migration（`migrateMarkdownDocs`）把每個 `doc` artifact 的每個格式 1 版本轉成格式 2，每個 artifact 一個 transaction，已經是格式 2 的版本不動，所以重跑不改變任何東西；
`artifacts` 那一列跟著最新版本。轉換前的 Markdown 原文存在 `artifact_versions.original_markdown`（不對外提供），轉換有錯時修好後可以從它重跑；這一欄在發佈前移除。

轉換用 `docfmt.FromLegacyMarkdown`：和 `FromMarkdown` 相同，但不因為連結或圖片拒絕（舊文件沒人能改了）：不合格位址的連結保留文字、圖片省略，所有註解行都只是原始 HTML（不會變成 `pending` 或分頁），
版本之間沿用沒變的區塊的 id。結果的文字必須和 Markdown 頁面顯示的文字相同（`TextOfDoc` 對 `TextOfMarkdown`：去掉標籤與註解、空白正規化）；不同或不合格時，整份 Markdown 以程式碼區塊保留，不遺失任何文字。
回到舊版 binary 讀不到遷移後的 doc（格式 2）；要回滾得先從 `original_markdown` 還原。
