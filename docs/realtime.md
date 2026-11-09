# 即時同步層（`internal/realtime`）

設計依據：#59（spike，`docs/spikes/realtime.md`）、
#62（hub）、#63（連線與存取）。
這份文件描述現在的樣子；還沒做的以 issue 連結標明。

## 它是什麼

一個 artifact 同一時間只有一份「活的」內容：記憶體裡的 room。人和 agent 都改這一份，改動即時送給所有在線的人，
並且在回應呼叫者之前先寫進 SQLite。CodeXArtifacts 仍然是一個 Go binary + SQLite，沒有外部服務。

```
transport（#63，WebSocket）        hub                          SQLite
  Peer.Receive(msg)  ───────▶  room（記憶體的 Yjs 文件）──▶  rt_updates（update log）
  ◀── Peer.Out()               Edit / Read / Replace          rt_rooms（epoch）
```

- **hub 不知道 socket、session，也不知道文件的意思。** transport 把已經授權過的 `Identity`（帳號、顯示名稱、是否唯讀）交給 `Hub.Join`；
  誰能編輯、誰只能讀仍然只由 `internal/service/access.go` 決定。
- 同步用 Yjs CRDT，Go 端是 [`github.com/reearth/ygo`](https://github.com/reearth/ygo)（MIT），**只用它的 `crdt`、`sync`、`awareness`、`encoding` 四個 package**。
- 傳輸格式是 y-websocket 的訊框（外層 0 = sync、1 = awareness、3 = queryAwareness），所以標準的 Yjs provider 可以直接接。

## 從同步找到版本與格式

update log 的持久化確認與 artifact 版本是兩件事。Docs／Design 的 snapshotter 在最後一次編輯後安靜兩分鐘才從 room 切版；還原前先保護 room 未切版的內容。
[Docs room](doc-room.md)解釋區塊、快照與整份 publish／restore，[Design room](design-room.md)解釋 frame 原文、元素對應與 agent 合併；各自的格式見 [Docs](doc-format.md) 與 [Design](design-format.md)。
所有入口與 beta 限制可從[文件索引](README.md)查找；live transport 不決定內容格式與誰能編輯。

## 決定：不用 ygo 的 websocket provider，自己的 handler

ygo 附一個 `provider/websocket`。spike 用它做出原型，但 hub 要的四件事它給不了，所以 hub 自己處理訊息，只借 ygo 的資料結構：

| hub 需要 | provider 的情況 |
|---|---|
| 每個連線有認證過的作者，每個 update 記得是誰送的 | 沒有「這個 update 是誰送的」 |
| 被接受的 update 在回應前已寫進磁碟 | 預設把 update 合併、每 2 秒才寫一次（崩潰時丟 2 秒），關掉後佇列仍是非同步 |
| 對某個人的連線、而不是整個 room，因存取變更而關閉 | 只有 `CloseRoom` 關整間 |
| 顯示 agent（不是連線的編輯者）、roster 的名稱由 server 決定 | 沒有 server 端的 awareness 注入；`user.name` 是 client 自報 |

自己的 handler（`hub.go`、`room.go`、`peer.go`、`presence.go`），沒有背景 goroutine：同步、持久化與廣播都在收到訊息的那次呼叫裡完成，
所以測試不需要 sleep，而且 room 沒有東西在背景持有它。

## 持久化與「已確認」

- 一個 room 的 log 是 `rt_updates` 的列（依 `seq`），room 的 epoch 在 `rt_rooms`；兩者都以 `ON DELETE CASCADE` 跟著 artifact。
- **`Peer.Receive` 或 `Hub.Edit` 回傳 nil，代表這個 update 已經 append 進 `rt_updates`，而且已排進其他 peer 的佇列。** 順序是：先 append、再廣播。
  所以別人看到的、或呼叫者被確認的內容，崩潰重啟後都還在（`TestAnAcknowledgedUpdateIsStillThereWhenTheRoomIsReopened`）。
- 存不下來（磁碟錯誤），或 `Edit` 的結果超過文件上限：這個變更不寫、不送，room 從記憶體丟掉、所有連線以 `ErrRoomReset` 結束，下一次加入從磁碟重載。
  記憶體因此永遠不會領先磁碟。
- 沒有寫入合併（ygo provider 的 2 秒窗口）：每個被接受的 update 一列，壓縮靠下面的機制。
- 空的 update（provider 對 step 1 的回應，沒有要補的）不是寫入：不存、不廣播、唯讀者也不算被丟棄。

### 壓縮與 epoch

- log 達到 `CompactAfter` 列、room 卸載、hub 關閉，或排程（`Sweep`，每 `CompactEvery`）時，整份文件的狀態折成一列。壓縮不改 epoch：歷史相同，只是存得小。
- **epoch 只在 log 被重建時改變**（`Hub.Replace`：用新內容建一份新文件取代整個 log）。重建後，已連線的 client 手上的是舊歷史，把它合併進新的會讓內容重複一份。
  所以 `Replace` 以 `*EpochError` 關閉該 room 的所有連線，`Hub.Join` 帶舊 epoch 也得到 `*EpochError`（附現在的 epoch）：client 要丟掉本機的 `Y.Doc` 從 server 重來。
  沒有本機狀態的 client 以空 epoch 加入。epoch 由 `Hub.Epoch` 取得，#63 把它放進 ticket 的回應。

## 限制

`Limits`（可由 `Config` 覆寫），超過時回 `*LimitError`，訊息寫出限制的名稱、這次的值和上限（例如 `update size (bytes): 70000 is over the limit of 65536`）：

| 限制 | 預設 | 說明 |
|---|---|---|
| `UpdateBytes` | 1 MB | 單一 update |
| `PeersPerRoom` | 50 | 值是「加入後會有幾個」 |
| `AwarenessBytes` | 4 KB | 單一 awareness 訊息（游標、名稱、選取） |
| `DocBytes` | 一般 2 MiB；Design 8 MiB | 文件大小 = 上次壓縮時的編碼大小 + 之後接受的 update 位元組，只會高估：刪字要等下次壓縮才釋出空間 |

被拒絕的 update 完全沒套用。但送出它的 client 本機有 room 沒有的變更，它之後的 update 可能依賴那份變更，所以 **transport 收到 `*LimitError` 就關閉連線**（socket 以 4002 關閉），讓 client 重連、重新同步。
另外一個連線最多宣告 8 個 awareness id、一則 awareness 訊息最多 32 筆。

## 作者、唯讀與 roster

- 每個 update 帶來源（`Identity`）：`Config.OnUpdate` 在 update 寫入之後、鎖釋放之後被呼叫，帶 `Room`、`Epoch`、`Author`、`Bytes`。作者是連線被授權的身分，**不是 awareness 裡自報的名稱**。
  `Hub.Edit` 的作者由呼叫者指定（agent 的工具呼叫、還原）。
- **唯讀的 peer 收得到所有編輯與 awareness，但它的寫入（sync step 2 與 update）在 server 丟掉**：不進 room、不進 `rt_updates`、不廣播、不觸發 `OnUpdate`，`Peer.Dropped` 計數。這不是錯誤，因為 provider 照常會對 step 1 回應。
- roster 的名稱由 server 寫：client 送來的 awareness 的 `user` 被換成 `Identity.Label`（沒有就用帳號），只保留 client 選的顏色；`cursor`、`selection`（design 畫布選取的元素）等其他欄位照舊。
  一個 awareness client id 只能由第一個發佈它的連線更新或移除；連線結束時它的項目被移除並廣播。
- **agent 不是連線**，所以由 server 發佈：`Hub.Present(room, label)` 在 roster 加上 `{name: label, agent: true}`，回傳的函式移除它。有人 present 的 room 不會因閒置被卸載。
- **agent 改 room 時出現在 roster：** `Hub.Announce(room, key, label, block)`（doc：它改的區塊）與 `Hub.AnnounceTouched(room, key, label, touched)`（design：`touched` 是 `{frame id: [pid…]}`，總共至多 `MaxTouched` 個元素）以 `{name: label, agent: true, at}` 加上這些欄位放進 awareness；同一個 `key`（帳號與 client）再宣告就取代並延長，`AgentIdle`（15 秒）沒有新的宣告，`Sweep` 把它移除。名字是「<帳號> 的 <client>」。連線不能自稱 agent，也不能發佈或移除 server 持有的 awareness id。

## 在 server 改 room：`Hub.Edit`

agent 的編輯、還原、之後的版本快照都走 `Hub.Edit(ctx, room, author, plan)`。它與 peer 的寫入走同一條路（存、廣播、`OnUpdate`）。
`plan` 先跑，可以隨意讀文件，回傳一個只寫的函式；寫的部分在 transaction 裡跑。這樣分是因為 **ygo 在 transaction 裡 `doc.GetText`／`doc.GetXmlFragment` 會永遠卡住**
（等一個 transaction 自己持有的鎖）；`TestReadingASharedTypeInsideATransactionNeverReturns` 固定這個行為，ygo 修好時它會失敗，提醒放寬這個設計。
`Hub.Read` 給只讀的存取（不可改文件）。

## room 的生命週期

- 第一個 `Join`／`Edit`／`Read` 從 log 載入 room（`Log.Load`，在 hub 的鎖內，避免兩個人同時載入同一個 room；log 在本機 SQLite，重播前先檢查該種類的文件上限）。
- `Sweep`（`Run` 以固定間隔呼叫）把沒有連線、沒有 present、閒置超過 `IdleAfter`（預設 5 分鐘）的 room 壓縮後卸載；它是時鐘的函式，測試用假時鐘。
  卸載後沒有東西持有 room（`TestAnUnloadedRoomsMemoryIsReleased` 用 weak pointer 讓 GC 證明）。
- `Hub.Disconnect(room, match, reason)` 關閉符合條件的連線，是存取變更（移除編輯者、改可見範圍、刪除）到達已開連線的入口，#63 接上。
- 跟不上的連線（佇列滿 `PeerQueue`）以 `ErrSlowPeer` 結束，不拖慢其他人。
- `Hub.Close` 中斷所有連線、壓縮、卸載。

## 單一 replica

room 在記憶體，hub 不與別的 hub 溝通：**部署必須是 1 個 replica**（`kubernetes-manifest/staging.yml` 的 Deployment 也這樣寫，並有註解）。
要擴充需要 sticky routing 之類的另一份設計，而且**不能用 Redis relay**（CodeXArtifacts 沒有外部服務）；不要把 `replicas` 改成 2。

## 依賴：釘版與 vendor 規則

- `go.mod` 釘 `github.com/reearth/ygo v1.51.12`：這是 spike 驗證過、並通過與 yjs 互通測試的版本。ygo 是 2026-04 才有 1.0 的單人維護專案，所以升版不是例行：
  升版的 commit 必須讓 `make verify`（含 `make verify-interop`）全過。
- **vendor 規則：** ygo 若 90 天沒有 release、而有一個擋住我們的 bug；或授權改變（目前 MIT），就把用到的四個 package vendor 進 repo（MIT 允許），之後自己維護。
- yjs 側的版本釘在 `internal/realtime/interop/package.json`（`yjs 13.6.33`，`package-lock.json` 提交）。

### 與真的 yjs 互通（interop gate）

`make verify-interop`（`verify-precommit` 的一部分）檢查兩個方向：

1. `internal/realtime/testdata/yjs/` 是**真的 `yjs` 函式庫**產生的 update 與它說會建出的內容（含 emoji、CJK、同時編輯、`XmlFragment` 的 block）；
   `interop/generate.mjs` 重新產生，結果必須與提交的檔案逐 byte 相同。Go 測試把這些 update 當成 peer 的 sync update 套進 room
   （`TestUpdatesFromTheRealYjsLibraryApplyInTheRoom`）。
2. `internal/realtime/testdata/go/` 是 **Go room 產生**的 update（固定 client id，所以可重現）；`interop/check.mjs` 用 yjs 套用它們並要求建出 Go 說的內容。
   Go 測試要求 room 產生的位元組與提交的檔案相同（`TestUpdatesTheRoomProducedAreWhatTheCommittedFixturesHoldAndApplyInYjs`；`PICTOR_UPDATE_FIXTURES=1 go test ./internal/realtime` 重寫）。

## 連線與存取（`internal/web/live.go`）

socket 是 hub 的 transport：同一個 origin 的 `/ws/{room}`（room = artifact id）。誰能編輯只由 `service/access.go` 決定，這裡只問它。

**開一條連線：**

1. 頁面用 cookie 與 `X-Pictor` 呼叫 `POST /api/artifacts/{id}/live`，得到 `{ticket, epoch, expiresInSeconds: 30, path}`。能看到 artifact 的登入者都拿得到；看不到的人得到與不存在的 id 完全相同的 404；匿名者與其他 API 一樣 401（socket 需要登入）。token（agent）不能換 ticket，agent 經由 server 進 room，不開 socket。
2. 頁面連 `/ws/{id}?ticket=…&epoch=…`（`epoch` 是它手上本機狀態的 epoch，沒有就省略）。升級時依序檢查：
   - **`Origin` 必須與 `Host` 相同。** `Origin: null`（sandbox 裡的頁面）、別的網站、沒有 `Origin` 一律 403，且不花掉 ticket。
   - **ticket 只能用一次、只給它發出時的那個 artifact、30 秒內有效；** 一出示就作廢，不論對錯。ticket 只有帳號與 artifact id，**權限在升級當下重新決定**：`Service.Edit` 成功 = 讀寫，`Forbidden`（看得到但不能編輯）= 唯讀（收得到編輯、寫入被丟棄），`NotFound` = 404，與不存在的 artifact 相同。
3. 升級之後才 `Hub.Join`。瀏覽器讀不到握手失敗的原因，所以 client 要處理的事用 **close code** 告訴它：

| code | 意思 | client 該做的 |
|---|---|---|
| 4001 | 文件被重建（epoch 變了），reason 是新 epoch | 丟掉本機的 `Y.Doc`，要新 ticket，以新 epoch 重連 |
| 4002 | 碰到限制，reason 寫出限制、這次的值和上限 | 本機有 room 沒有的變更：丟掉本機狀態重連 |
| 4003 | room 因存檔失敗被重載 | 重連 |
| 4009 | 跟不上 room | 重連 |
| 4403 | 誰能看或編輯這個 artifact 變了（或 artifact 被刪） | 要新 ticket，權限會重新決定 |
| 4408 | 連線到了最長存活（1 小時） | 要新 ticket，重連 |
| 1001 | server 正在重啟 | 稍後重連 |

**撤權。** `Service` 在受眾、編輯者、組織編輯開關改變或 artifact 被刪之後、回應之前，呼叫註冊的 `OnAccessChange`（`liveAccessChanged`）。它對這個 room 的每一種連線身分重問一次 `Service.Edit`，
權限與開連線時不同的就關掉（4403）：被移除的編輯者、新受眾看不到的人、組織編輯關掉後不是指名編輯者的人，刪除時是所有人。
被關的人重連時拿新 ticket、權限重新決定，所以「移除編輯者後的下一個寫入」要嘛沒有連線可送，要嘛是唯讀連線、被丟棄。

**存活。** 連線最長 1 小時（`socketLifetime`），到了以 4408 關閉，client 重連並重新授權。server 每 25 秒 ping 一次，client 的 awareness 每 15 秒回傳一次（room 也把 awareness 回給送出者，
所以獨自在 room 裡的 client 不會 30 秒沒收到東西而被 y-websocket 重連）。時間都來自 `Config.Clock`，測試用假時鐘。
Ingress（`kubernetes-manifest/staging.yml`）的 `proxy-read-timeout` 與 `proxy-send-timeout` 設為 3600 秒，遠大於心跳間隔。**這兩個值還沒有在 staging 的真實 ingress 上驗證**（驗證方式見 #63 的 staging check）。

`main` 在 HTTP server 旁啟動 `Server.Run`，它定期卸載閒置的 room、壓縮 log，結束時關掉所有 socket（socket 是從 HTTP server 劫持的，`Shutdown` 不會等它們）。

## 還沒有的

- 兩個 Ingress timeout 在 staging 真實 ingress 上的驗證，以及第二台裝置的寫入，仍待 #63 的 staging check 記錄；上面「存活」一節的數字是設定值，不是驗證結果。
- 版本、還原與整份 publish 怎麼走 room：見 [Docs room](doc-room.md) 與 [Design room](design-room.md)；編輯器、游標與 roster 的畫面見 [editor.md](editor.md)。
