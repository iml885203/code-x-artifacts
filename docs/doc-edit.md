# Agent edits a doc (`internal/docedit`)

設計依據：#68。區塊格式見 [doc-format.md](doc-format.md)，room 見 [doc-room.md](doc-room.md)。
agent 局部修改一份 doc：先以區塊讀（id、hash），再送 `doc_edit`；不重寫整份、不蓋掉別人正在打的字。

## 讀

- MCP `read` 加 `view`：`markdown`（預設：內容仍是 Markdown，另加 `outline`：每個頂層區塊的 `tab`、`id`、`type`、`hash`、`preview`）或 `blocks`（`tabs`：每個分頁的每個區塊，帶 `id`、`type`、`hash` 與內容，不含 Markdown）。兩種 view 都有 `tabs`，每個分頁至少有 `id` 與 `title`（`markdown` view 沒有 `blocks`）。HTTP 是 `GET /api/artifacts/{id}/blocks`（`version`、`tabs`、`outline`）。
- doc 有 room（開過）時，讀到的是 room 的內容（人們看到的），`version` 仍是最新**存下來**的版本（`Service.Current`）。
- `hash` 是區塊現在的樣子（`docedit.Hash`：區塊 JSON 的 SHA-256 前 12 位，含 id 與巢狀區塊）；區塊裡任何東西變了它就變。

## 改：`doc_edit {id, ops[], baseVersion?}`

HTTP：`POST /api/artifacts/{id}/edit`，同一個 `Service.DocEdit`，同樣的結果與拒絕。

| op | 欄位 |
|---|---|
| `replace` | `id`、新內容（`markdown` 或 `blocks`）；第一個新區塊沿用被取代區塊的 id，除非它自帶 id |
| `insert` | 位置恰好一種：`before`／`after`（區塊 id）或 `tab` 加 `at`（`start`／`end`）；新內容 |
| `delete` | `id` |
| `tab_add` | `title`（一行）；位置至多一種：`at`（`start`／`end`，預設 `end`）或 `before`／`after`（分頁 id）；內容可選（`markdown` 或 `blocks`），沒有內容時新分頁從一個空的 H1 開始 |
| `tab_rename` | `tab`（分頁 id）、`title` |
| `tab_delete` | `tab`；分頁連同它的區塊一起刪（`deleted` 列出那些區塊的 id），文件的最後一個分頁不能刪 |
| `tab_move` | `tab`，加上恰好一種位置：`before`／`after`（別的分頁 id）或 `at`（`start`／`end`） |

分頁 op 用 `tab` 指分頁、不用 `id`（區塊 id 不是分頁 id），也不帶 `ifHash`（那是給區塊的）。做過分頁 op 的呼叫，結果多一個 `tabs`（做完之後每個分頁的 `id` 與 `title`，新分頁的 id 在這裡）。一次最多 50 個分頁（`docfmt.MaxTabs`）。
分頁在 room 裡的順序是一個 id 陣列、每個分頁的內容在自己的 fragment（[doc-room.md](doc-room.md)）：改名只改標題，搬動只動順序，所以搬動的當下有人在那個分頁裡打字，字都留著。

- **全部成功或全部不存。** 依序在一份副本上做；任何一個 op 做不到（未知的 id、位置寫錯）、或結果不是合法的 doc（連結不合格、id 重複…），整個呼叫被拒絕，訊息以 `ops[i]:` 開頭；什麼都沒存、room 不動。
- **`ifHash`**（每個 op 可帶）：replace／delete 是它指的區塊，insert 是它旁邊的區塊。比對的是呼叫開始前的 doc；有任何一個不符，整個呼叫被拒絕（HTTP 409、MCP 錯誤），附上所有過期區塊現在的樣子（`stale`：`tab`、`id`、`type`、`hash`、內容），agent 合併後以新 hash 重送。
- `baseVersion`（可選）照一般發佈的規則比對最新存下來的版本，過期回 Conflict。room 開著時版本每隔一陣子才動，所以真正保護人們輸入的是 `ifHash`。
- 可定位的區塊包括 quote 裡面的（有 id）；清單項目與表格沒有 id，換掉整個清單或表格區塊。
- **怎麼寫：** doc 有 room 時，結果經 `Content.Apply` 寫進 room（作者是呼叫者的帳號），只改不同的地方，所以另一個區塊、同一個 quote 的另一段、清單沒改的項目裡正在打的字都留著；回應 `saved: false`，版本在 room 安靜之後出現。沒有 room 時直接存成新版本（`saved: true`），不為它開 room。內容和現在相同時什麼都不做。
- **結果：** `version`（最新存下來的）、`saved`、`blocks`（寫了的區塊：`id`、`type`、新 `hash`）、`deleted`（刪掉的 id）、`review`（要再看一次的事：文件裡每一張沒有 alt 的圖片，不只這次寫的，所以人在編輯器放進去的圖片在 agent 下一次編輯時也會被提醒）。
- **刪掉有留言的區塊（#140）：** 呼叫拿掉了某個留言串錨點（#81）的 `block` 時——`delete`、`replace` 成自帶 id 的區塊、刪掉包住它的 quote 或分頁——結果多一個 `commentsOnDeletedBlocks`（每串的 `id`、`tab`、`block`、`quote`），`review` 多一則指向它的提醒。編輯照樣做、不擋；留言串照樣在 `comments` 裡、帶著原本的引文（編輯器標「Original text changed」）。只改區塊裡的字、區塊還在時不提醒。比對的是呼叫前後的區塊 id（`docedit.Removed`），在寫入之前做，所以兩個入口、有沒有 room 都一樣；呼叫者能讀留言（`CanComment`）時才列。
- 上限：一次最多 200 個 op。

## 在線上的 agent（presence）

#78 的 server 端；編輯器（`web/src/editor/agents.ts`）與共用的頭像堆疊（`web/src/presence/`，[viewers.md](viewers.md)）畫出來：堆疊裡 agent 有「AI」小圖示，agent 改的區塊以它的名字標示、約 4 秒後淡出（從這個瀏覽器收到的時刻起算，不用 server 的時鐘）；`e2e/tests/editor.spec.ts` 有兩個瀏覽器加一個 agent 的測試。

- agent 透過 room 寫 doc 時（`doc_edit`，以及 room 開著時的整份 `publish` 與還原），它出現在 room 的 roster：awareness 的一筆 `{user: {name, agent: true}, block, at}`，`block` 是它改的區塊（`doc_edit` 是寫的第一個區塊，整份 publish 是第一個不同的頂層區塊），`at` 是時間（毫秒），畫面用它讓游標在幾秒後淡出。
- **名字：** `<帳號> 的 <client>`，例如「logan.guo54 的 Claude Code」；client 不認得時是「<帳號> 的 agent」。client 來自該次 MCP 請求的 `clientInfo.name`（目前協定在每個請求的 `_meta` 傳送）；stateless 請求沒有提供名稱時顯示 agent，不沿用同 token 的 initialize 名稱，以免共用 token 的不同 client 互相冒名，以名稱開頭比對：Claude Code、Claude、Codex、Cursor、Windsurf、Gemini CLI、Copilot、VS Code、Cline、Zed、Goose、OpenCode、Hermes（`mcptools.ClientName`）。
  舊協定只在 initialize 說一次，server 以 token（的雜湊）在記憶體記住；重啟後沒再說過的是「agent」。HTTP token 的呼叫是「agent」。瀏覽器 session 的呼叫不是 agent，不顯示。
- **不是 socket 的連線：** server 注入（`Hub.Announce`）。再次呼叫同一個 key（帳號與 client）就移到新的區塊並延長停留；`AgentIdle`（預設 15 秒）沒有新的呼叫就被 `Hub.Sweep`（每 5 秒）移除。有 agent 在的 room 不會因閒置被卸載。
- **瀏覽器偽造不了：** 連線送來的 awareness 的 `user` 一律由連線自己的身分重建（`agent` 只有 server 注入的才有），連線不能用 server 持有的 client id 發佈或移除（`TestAConnectedBrowserCannotBeAnAgentOrSpeakForOne`）。
- 沒有 room（沒有人開過）時沒人看，也就不宣告。
