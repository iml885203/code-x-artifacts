# 使用量

部署的負責人用使用量回答「CodeXArtifacts 多常被用、被哪些部門用」：每週（或每天）各部門、各種類開啟與發佈 artifact 的次數。
結果只有數字，不列出任何帳號或 artifact。

## 記了什麼

`usage_daily` 每列是一個人一天對一種 kind 的一種動作的次數：

- **open**：viewer 開啟 artifact（`POST /api/artifacts/{id}/open`，每次開啟算一次，同一天重複開也算）。
  沒登入、開 `public` 連結的讀者也算，帳號記成空字串；不記 IP、瀏覽器或任何能認出人的資料。agent 的 `read` 不算開啟，帶 token 的 open 一律 401。
- **publish**：每個新版本（新建、更新、還原、複製、改名、room 切出的版本），分成 `web` 與 `agent`。
  帶個人 token 的請求（MCP 或 HTTP）、或版本記錄的 agent 寫進 room 的內容，算 `agent`；其餘算 `web`。

表不參照 `artifacts`，所以刪掉 artifact 後它過去的使用量還在。日期以 UTC 切。

部門是 AD 在這個人最近一次登入時給的 `department` 屬性（同一次 bind、同一個查詢），存在 `users.department`；
AD 沒給就是空的，摘要裡顯示 `unknown`。不從群組推斷部門。摘要用每個人目前的部門，所以換部門的人，舊的次數也算到新部門。

## 保留多久

每天的 housekeeping（`Server.Maintain`）刪掉超過 400 天的列（`store.UsageKeptDays`）。

## 誰能讀

部署設 `PICTOR_USAGE_READERS`（逗號分隔的帳號，和登入時一樣不分大小寫、不帶網域）。
規則只在 `service.Viewer.CanReadUsage`：只有列在裡面的已登入帳號能讀；沒設就沒有人能讀，作者、編輯者也不行。
其他人一律 403，HTTP 和 MCP 給同樣的拒絕。

## 怎麼讀

```
GET /api/usage?from=2026-08-01&to=2026-10-31&period=week&by=department,kind
```

MCP 工具 `usage_summary` 收同樣的 `from`、`to`、`period`、`by`（陣列），回同樣的 JSON。

- `from`／`to`：`YYYY-MM-DD`，兩端都含；預設是到今天為止的 12 週，最多 400 天。
- `period`：`week`（預設，週一開始）或 `day`。
- `by`：`department`、`kind`、兩個都要，或不給（每期一個總數）。

每一列：`period`（該期第一天）、分組時的 `department`／`kind`、`opens`（登入的人）、`anonymousOpens`、`publishes`（`webPublishes` + `agentPublishes`）、
`activePeople`（開過或發佈過的不同人數）、`publishingPeople`。依部門分組時，匿名的開啟歸在 `anonymous`。

沒有網頁介面；讀者的 agent 可以呼叫工具，再把結果發佈成一個 artifact。
