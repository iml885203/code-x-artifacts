# 測試策略

依據：one-for-all 的 `docs/adrs/0002-test-strategy-accepted.md`（ADR 0002），這裡是它在 CodeXArtifacts 的寫法。
測試的成本差好幾個數量級（規則層毫秒、e2e 要整個 server 加兩個瀏覽器），所以每一層該有幾個案例由成本決定，不由習慣決定。

## 四層與 CodeXArtifacts 的位置

| 層 | 回答 | 在 CodeXArtifacts 哪裡 | 案例數 |
|---|---|---|---|
| e2e | 瀏覽器 + server（+ 即時同步）的流程通嗎？ | `e2e/tests/*.spec.ts`（Playwright，CI 跑 Chromium 與 Chrome） | 刻意安排、很少 |
| 整合 | 模組之間、模組與資料一致嗎？ | `internal/web`、`internal/mcptools`、`internal/service` 的 Go 測試（HTTP / MCP / ws，真的 SQLite）；`web/` 的 vitest 元件測試（真的目錄文字、真的 store） | 每條驗收條件一個，加每種不同的落地結果一個；權限、可見度、驗證、回應契約 |
| 規則 | 規則對嗎？ | 純 Go 套件（`docfmt`、`designfmt`、`files`、`roomkind`、`service/access.go`）與 `web/` 的純函式 | 窮舉：每個分支、邊界、錯誤 |
| 邊界（例外） | 讀第三方的格式讀對了嗎？ | Yjs/ygo 線上格式、ProseMirror、OOXML、AD；`testdata/` 錄下來的 payload | 每種轉換一個 |

入口（handler、元件）與規則之間的一切（service、store、mapper、中間層）都**經由整合層**覆蓋，自己沒有測試。

## e2e 三個條件

要三個都成立才寫 e2e：

1. 它跨過 server 到達前端（真的瀏覽器才看得到）；
2. 較低的層真的證明不了；
3. 悄悄壞掉的代價很高。

只放有價值的 happy path：一個功能一條主流程，優先擴充既有 spec 的流程，不要新開。
錯誤、拒絕、邊界、回歸一律放 Go 或 vitest。數量由 `.e2e-budget` 守住（`make verify-e2e-budget`，`verify-precommit` 的一部分）：
超過就失敗；要調高就在同一個 commit 改那一行數字，review 必須說明為什麼上面三個條件成立、為什麼整合層不夠。調低隨時可以。

## 深度由成本決定

兩個案例只差在「規則層決定的值」，就是一個整合案例加數個規則案例：入口留一個代表，窮舉往下推。
十幾個規則測試加三個整合測試是正常的形狀。

## 可以替換的東西

只有第三方的 port（這裡是 AD：`fakeDirectory`）和不確定性（時鐘、隨機）。自己的 service、store、room 一律真的跑。

## 不測編排，測結果

不寫「X 被呼叫了」「呼叫了幾次」的 assertion；斷言對外看得到的結果：回應、存下來的資料、畫面。

## 測試擁有自己的資料

測試平行執行（`-shuffle=on`，Playwright 多 worker）：用自己握有的 id 建資料，assertion 只看那些 id，只刪自己建的。整表清空只在一個測試獨占那張表時才合法。

## 用行為命名

測試名稱寫它涵蓋的行為或規則，不寫它呼叫的函式或類別：`TestAToolAndItsHTTPCounterpartGiveTheSameResult`，不是 `TestServiceCreate`。

## 測試不是程式碼的鏡像

不要一個原始檔配一個測試檔、一個函式配一個測試：那樣等於維護兩份程式碼。規則套件只在它的公開 API 測；中間層沒有自己的測試。
檢查方式：**一個不改行為的重構不該需要改任何測試。** 要改，就表示測試綁在實作上：改成對行為斷言，或刪掉。
這一條靠 review 判斷，沒有機器檢查（鏡像測試無法可靠地偵測）。
