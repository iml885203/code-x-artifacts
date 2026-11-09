# e2e（瀏覽器測試）

Playwright，自己的 go.mod 理由同 `web/`：`tests/*.spec.ts`、`tests/support.ts`（登入、發佈、console error 的規則）、`playwright.config.ts`（起 smoke server）。npm、版本釘死、`package-lock.json` 提交；Node 版本與 `web/` 相同。

## 不變式

- **瀏覽器測試在 pipeline 的獨立 job `e2e` 裡（不在 `make verify`），任何 console error 都讓它失敗，除非那個錯誤列在 `tests/support.ts` 的 `knownConsoleErrors` 並附上追蹤它的 issue（目前沒有；總覽的卡片預覽若剛好對到別的測試剛設成私人或刪掉的 artifact，會得到 404，那一則另外放行）；`e2e` job 紅燈時不部署 staging。** 失敗的測試留下 trace 與截圖當 artifact。
  Tests: e2e/tests/ 的每一個測試都帶這條檢查（support.ts 的 consoleRule，自動套用）；bridge.spec.ts、viewer.spec.ts、language.spec.ts、kinds.spec.ts。

- **spec 裡的 `boundingBox()` 只能量頁面上的東西（要在同一行或上一行標 `// page-level`）；frame 裡的元素一律經 `support.ts` 的 `onScreen`。** Playwright 對縮放的 frame 裡的元素，Chromium 141 回螢幕上的框、Chrome 155 回沒縮放的框，所以直接量的測試只在一種瀏覽器過。`make verify-e2e-boxes`（`verify-precommit` 的一部分）檢查。

## 坑

- 新增 e2e 之前先讀 `docs/test-strategy.md`：三個條件都成立才寫，錯誤、邊界與回歸放 Go 或 vitest。總數由 `.e2e-budget` 守住（`make verify-e2e-budget`）；調高那一行要在 review 說明理由。

- 只有真的瀏覽器才證明得了的才放在這裡，數量少而且是刻意安排的：bridge 的 postMessage 來回（含 `pictor.download` 問過本人才存）、sandbox、viewer 的切換與選單、版本紀錄的唯讀、即時 socket 的 Origin、開著的 viewer 自動換成新版本、react 頁面、上傳檔在 sandbox 裡載入、從 viewer 上傳與刪除檔案、語言切換、卡片的相對時間（假時鐘）與釘選（gallery-cards.spec.ts）、白板（board.spec.ts：Excalidraw 畫在 canvas 上，每個瀏覽器的場景用全選加複製從剪貼簿讀出）。
- `make e2e`（第一次先 `make e2e-browser`，或設 `PICTOR_E2E_CHANNEL=chrome` 用系統的 Chrome）；埠由 `PICTOR_E2E_PORT` 決定。
- 改了 `bridge.js`、`web/src/viewer/`、`web/src/editor/`、sandbox 屬性或前端的 shell 之後要跑一次。
- 編輯器的測試共用 `tests/editor-support.ts`（多個人各自的瀏覽器、agent 的 MCP 呼叫）；smoke server 另有 carol/pw-c。
- 在 modal（`<dialog>`）上點按鈕之前先 `presented(dialog)`（`support.ts`）：modal 後面若有沙箱 iframe（它是獨立的 process），畫面還沒畫出 modal 時點擊會被送進 iframe，modal 永遠收不到。等進場動畫結束就是它已經畫出來了；不要用固定等待。
