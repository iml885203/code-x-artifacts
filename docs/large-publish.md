# 從檔案發佈大頁面（publish session）

設計依據：#115。

## 要解決的問題

MCP `publish` 要把整個頁面放在工具參數裡，agent 得把它一個字一個字寫出來。幾百 KB 的報告（十幾萬 token）沒有模型能一次可靠寫完；
`files` 一樣要寫出來，`upload_asset` 只收圖片、字型、PDF、CSV。頁面明明已經是磁碟上的一個檔案，卻只能重打一遍。

MCP 目前沒有「client 送檔案給 server」的機制（規格草案還在討論），所以位元組走帶外：agent 在 shell 裡用 `curl` 把檔案送到一個一次性的網址，
工具呼叫只傳「這些檔案的雜湊與大小」和「好了，發佈」。

## 流程

```
begin_publish   宣告每個檔案的 path、sha256、size   →  session、還沒有的 blob 各一個上傳 URL、到期時間、限制
curl -X PUT     把位元組送到 URL（不帶 token）      →  server 重新雜湊，不符就拒絕
commit_publish  session、sourceBases（live Design）                              →  經 Service.Create / Update 建版本，回傳與 publish 相同的結果
```

HTTP 是同一件事：`POST /api/publish-sessions`、`PUT /api/uploads/{secret}`、`POST /api/publish-sessions/{session}/commit`
（begin 與 commit 要 token 或 cookie；上傳不要）。MCP 與 HTTP 共用 `Service.BeginPublish`、`ReceiveUpload`、`CommitPublish`，
回應由 `service.BegunView` 一處產生。

### begin

```json
{ "id": "…", "title": "…", "kind": "…", "format": "…", "baseVersion": 3, "visibility": "…",
  "files": [ {"path": "content", "sha256": "…", "size": 123456}, {"path": "data/rows.json", "sha256": "…", "size": 99} ],
  "remove": ["old.js"] }
```

- 欄位和 `publish` 相同，只是內容換成 `files`。頁面本身的路徑是 `content`（每次都要送）；其他檔案是它帶的檔案（`html`、`react`、`design`，規則同 [multi-file.md](multi-file.md)）。
  `content` 沒有副檔名，所以不可能與任何檔案路徑相撞。`remove` 對應 `publish` 的 `files` 裡值為 `null` 的路徑。
- begin 在收到任何位元組之前就拒絕 inline publish 也會拒絕的事：誰能發佈、過期的 `baseVersion`（409，附最新版）、kind 與 format、
  路徑與型別、單檔大小（16 MB）、一次的檔案數（255）與總大小（256 MB）。頁面自己的大小上限與內容檢查在 commit 做，訊息與 `publish` 相同。
- 回應 `missing`：每個 server 還沒有的 blob 一項（`paths`、`sha256`、`size`、`url`）；內容相同的路徑共用一個上傳。
  `stored` 列出 server 已經有位元組的路徑：artifact 最新版的檔案裡已有同一個 sha256 的，不必重送（頁面本身不算，它不是 blob）。
  所以大型資料檔不變、只改頁面的更新，只傳頁面。
- 回應也帶 `session`、`expiresAt`、`expiresInSeconds`（600）與 `limits`。

### 上傳

`PUT /api/uploads/{secret}`，body 就是檔案本身：

```
curl -sS --fail-with-body -X PUT --data-binary @report.html "<url>"
```

URL 是唯一的憑證，所以它是秘密：不要印出、不要放進給人看的訊息（工具說明與 skill 都這麼教）。

- 綁定一個 session、一個 sha256、一個大小；body 比宣告的長、短，或雜湊不符都拒絕（400），什麼都不存，URL 仍可用。
- 成功之後作廢；存位元組之後才算用過，同一個 URL 的兩個同時請求只有一個成功。到期（十分鐘）、用過、不存在回同一個 404。
- 不讀 cookie 也不讀 Authorization，所以沒有任何環境權限可以被跨站請求借用；只收 `PUT`，其他方法 405 並說明怎麼送。
- 位元組先放在 `{assets dir}/~{session}/blobs/{sha256}`（artifact 還沒有的時候的暫存處；沒有 artifact id 會以 `~` 開頭）。

### commit

`commit_publish(session)` 把宣告的檔案組成一個 inline publish：頁面是 `content` 的位元組，其他檔案是 `files` 的差異（`remove` 是 `null`），
再交給 `Service.Create`（沒有 `id`）或 `Service.Update`。因此驗證、限制、`baseVersion` 衝突、正規化（design 的 `rewritten`）、review
與 inline publish 一樣，因為走的是同一段程式。

- session 只屬於建立者；別人的、過期的、已 commit 的、不存在的都回同一句 404。
- 一個 session 只能 commit 一次（commit 開始時先 claim）。被拒絕時 session 留到過期，暫時性的失敗可以重試；要改內容就重新 begin。
- 還有位元組沒到：拒絕、什麼都不存，列出缺的路徑。
- 位元組要是 UTF-8 文字（頁面與檔案都是文字）。
- commit 時才重新檢查編輯權：begin 之後被移除編輯者的人 403。

Live Design 的 commit 另帶 `sourceBases: {"design.json": "…", "frames/hero.html": "…"}`，
使用準備草稿時 `read` 回傳的逐檔 proof；HTTP commit body 是 `{ "sourceBases": {…} }`。
不要重送 baseline 的完整原始碼，session 只保留上傳草稿。過期、遺失或已失效的 proof 和 inline publish 一樣拒絕，
保留草稿、讀取現在的 source 並合併後再送。唯讀與指定歷史版本的 read 不發 proof；詳見 [design-source-edit.md](design-source-edit.md)。

### 清理

session、上傳 slot 與暫存位元組到期後刪除：每次 `begin` 開頭、`Server.Maintain`（啟動與每天）都會掃，也會刪掉沒有 session 的暫存目錄（超過一小時）。
commit 成功立刻刪。一個人最多同時有 10 個未過期的 session，限制沒人 commit 時佔住的磁碟。

## Inline publish

小頁面與沒有 shell 的 agent 繼續用 `publish`。內容超過 kind 的大小上限時，錯誤訊息最後指向這個流程（`begin_publish` 與 `commit_publish`、`POST /api/publish-sessions`）；
經 session 來的內容超過上限時不重複指向。

## 還沒有的

- 以 `blocks` 寫成的 doc 走 `publish`；session 的 doc 是 Markdown 檔案。
- MCP 的檔案輸入機制定案後，`begin_publish` 對應過去、由 host 上傳；流程的其餘部分不變。

## 規則與測試

見 `CLAUDE.md` 的不變式「大檔案從檔案發佈」，測試在 `internal/web/publish_sessions_test.go`、`internal/web/mcp_publish_sessions_test.go`、`internal/store/publish_sessions_test.go`，
瀏覽器之外的端對端是 `e2e/` 的 `publish-from-file.spec.ts`。
