# 誰正在看

任何種類的 artifact 的 viewer 右上角都顯示「現在正在看它的人」：讀者、編輯者、正在讀它的 agent（#123），以及沒有登入、從 public 連結進來的訪客（#172）。
訪客以隨機暱稱、標上「訪客」出現；它只是一個瀏覽器，不是驗證過的人，也讀不到名單上有誰。

## Server

- **Heartbeat**：每個開著的 viewer 頁每 20 秒（`HeartbeatEvery(PersonStays)`；回應的 `CodeXArtifacts-Heartbeat-Ms` 標頭告訴頁面這台 server 的節奏）`POST /api/artifacts/{id}/viewers`，回應就是呼叫者能看的名單，所以一次呼叫同時是 heartbeat 與讀取；`GET` 只讀，只給登入的人。名單只放在記憶體（`viewerBook`，一個人或一個訪客在一個 artifact 一列，多個分頁共用一列）：它只在一分鐘內有意義，部署又只有一個 replica（[realtime.md](realtime.md)），重啟只讓頁面多等一次 heartbeat，所以不進資料庫。
- **到期**：人與訪客在最後一次 heartbeat 後 60 秒（`PersonStays`）離開名單，agent 在最後一次 `read` 後 15 秒（`AgentStays`，與 room 裡 agent 游標的閒置時間相同）。到期是讀取時算的；heartbeat 每秒最多一次順便清掉整本名單裡到期的列。時間長度可由 `Config.ViewerStays` 縮短（只有 smoke server 的瀏覽器測試用，`PICTOR_SMOKE_VIEWER_STAYS`）。
- **權限**：heartbeat 與名單都先經 `Service.Artifact`，看不到的人得到與「id 不存在」相同的 404；沒登入的人在看不到的頁面上得到 401（與其他開放給匿名的 API 相同）。誰能讀名單是 `Viewer.CanSeeViewers`（登入且看得到），誰能被列為訪客是 `Viewer.CanBeAGuest`（匿名、部署開著 public 連結、artifact 是 public；頁面的 grant 不算），兩者都在 `access.go`。名單上每一列讀取時再檢查那個人或訪客現在還能不能在這裡，不能的立刻從名單和記憶體移除：取消分享、改掉 public、部署關掉 public 連結後，下一次讀取就不再有他們，他們之後的 heartbeat 也被拒絕。
- **名字**：人是登入時讀到的顯示名稱；agent 是「<帳號> 的 <client>」，`agent: true`，規則與 room 的 agent 相同（[doc-edit.md](doc-edit.md)）。每一列有 `id`（人是帳號，agent 是「<帳號>/<client>」，訪客是 `guest:<摘要>`），room 的 awareness 對同一個人帶同樣的 `user.id`；`self` 是呼叫者自己。agent 自己不在自己拿到的名單裡。排序是自己、人與 agent（依名字）、訪客（依名字）。
- **訪客**：沒登入的頁面在 heartbeat 的 body 帶 `{"guest": "<瀏覽器的隨機 id>"}`。Server 只接受 16 到 64 個 `A-Z a-z 0-9 - _` 字元（body 最多 1 KB），其他一律 400。列出來的 `id` 是那個 id 的 SHA-256 摘要加上 `guest:` 前綴（帳號和 agent id 都不會有冒號，所以訪客永遠不會被當成或合併成某個人或 agent），暱稱是同一個摘要選出的「形容詞 + 動物」兩個英文字（`guests.go`），所以同一個瀏覽器在每個 artifact、每次重新載入都是同一個暱稱，而瀏覽器的 id 本身不會出現在任何回應裡，讀到名單的人也無法冒用它。同一份名單裡兩個訪客抽到同一個暱稱時，各自加上取自自己摘要、最短而不重複的標記（`Swift Otter #3f2a`），所以標記只看有誰同名、與名單順序無關；標記只給登入的讀者。訪客的 heartbeat 只回它自己那一列（`guest: true`、`self: true`，暱稱不帶標記，所以從回應看不出有沒有同名的訪客），永遠不含登入的人、agent 或其他訪客。沒帶 guest id 的匿名 heartbeat（session 剛結束、還開著的登入頁）得到 403，頁面當作已登出、停止送。訪客只是名單上的一個名字：它不是 `Viewer`，不帶來任何權限，沒登入的人仍然不能編輯、留言或寫共享資料。
- **上限**：只有匿名的流量能在沒有帳號的情況下新增列，所以訪客有上限：一個 artifact 最多 `MaxGuestsPerArtifact`（100）個、整台 server 最多 `MaxGuests`（5000）個訪客列。滿了之後新的訪客得到 429 與 `Retry-After`（一個人留在名單的時間），頁面照樣能讀、不記錯誤，等那麼久再試；已經在名單上的訪客照常續約，不會被擠掉。到期的列由每秒最多一次的清理騰出空間，被拒絕的請求不會另外走過整本名單，所以大量被拒的匿名請求在鎖裡的成本不超過清理的節奏。
- **已知限制**：上限擋的是記憶體，不是濫用者。刻意不依 IP 限流（訪客不用 IP 識別），所以一支不斷換新 guest id 的程式可以把某個 public artifact 的 100 個位置、或整台的 5000 個位置佔滿；這段期間真正的新訪客不會出現在名單上（頁面照樣能讀），直到那些假訪客停止後約一分鐘到期。已經在名單上的人、agent 與訪客不受影響。
- **已知限制（還開著的舊分頁）**：分頁的介面在載入時決定登入與否。沒登入時打開的分頁，在另一個分頁登入後，server 會依 cookie 把它當成登入的人（記錄帳號、移除這個瀏覽器的訪客），那個舊分頁會開始看到完整名單，但介面仍是訪客的樣子，直到重新載入。反過來，登入時打開的分頁在 session 結束後，若這個瀏覽器以前當過訪客，會以那個訪客出現、只看到自己；從沒當過訪客就停止 heartbeat。
- **登入**：登入後的頁面若這個瀏覽器以前當過訪客，heartbeat 也帶上那個 guest id，server 先移除那個訪客的列再記錄帳號，所以登入的瞬間名單上就是帳號、沒有重複的訪客。
- **Agent path**：MCP `read` 的結果有 `viewers`（`id`、`name`、`agent`、`guest`、`self`），權限與 `read` 相同，訪客的 `id`、暱稱和標記與 HTTP 對同一個 viewer 給的相同；工具說明告訴 agent 訪客暱稱是瀏覽器、不是驗證過的人。`read` 同時讓這個 agent 在接下來 15 秒內出現在別人的名單。沒有新工具。

## 前端

`web/src/presence/`：`AvatarStack`（共用的元件：前 4 個頭像、「+N」按鈕打開所有人的名字、agent 有 AI 小圖示、訪客是中性底色的頭像並標「訪客」、你自己標「你」）、`useViewers`（開著時立刻、之後每 20 秒 `POST`，頁面隱藏時不送、顯示時立刻送，artifact 不見或登出就停）、`guest.ts`（瀏覽器的訪客 id）、`mergePeople`（以種類和 `id` 合併，不看顯示名稱，所以同一個帳號只出現一次、名字取比帳號更多的那個；訪客永遠不會和同 `id` 或同名的人、agent 合併）。
Viewer 的標題列右上角掛一個 `Viewing`，登入與沒登入的人都有；手機上收進「⋯」選單，heartbeat 照常。沒登入的人只看到自己（標「訪客」與「你」）。live room（Docs 編輯器、Design 畫布）自己的人（awareness）由 `RoomPresence` 經 `publishRoomPeople` 交給同一個堆疊，與 heartbeat 名單合併（同一個 `id` 只出現一次；自己在 awareness 裡是瀏覽器寫的帳號，在名單裡是顯示名稱，只有 `id` 對得起來），所以 room 不需要自己的第二份名單；Docs 與 Design 只各畫一個連線燈（`live`、`reconnecting`、`offline`），不再有自己的 roster。訪客不會進 room（沒登入的人只看版本），所以只從 heartbeat 名單來。

**訪客 id**：沒登入的頁面第一次 heartbeat 時，`guest.ts` 用 `crypto.getRandomValues` 產生 128 位元的隨機 id（22 個 base64url 字元），存在 `localStorage` 的 `pictor-guest`，所以同一個瀏覽器設定檔的每個分頁、每次重新載入都是同一個訪客。不同瀏覽器、無痕視窗或清掉網站資料會是新的訪客。瀏覽器擋掉 `localStorage` 時改存 `sessionStorage`（這個分頁的 session），兩者都擋時只留在開著的頁面；讀頁面不受影響。不做任何裝置指紋，也不用 IP。登入後的頁面只讀、不產生這個 id。
