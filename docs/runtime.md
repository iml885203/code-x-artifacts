# 內嵌的執行環境（runtime）

頁面（`react` kind，見 [spikes/react-kind.md](runtime.md) 的設計依據）會 import 的函式庫由 CodeXArtifacts 自己提供，
不從外部 CDN 載入。這些函式庫以版本化的 ESM 內嵌在 binary 裡；Node 只在建置時用，執行時沒有 Node、也不需要網路。

## 內容

| 位置 | 是什麼 |
|---|---|
| `runtime/` | 來源：`package.json`、`package-lock.json`（版本釘死並提交）、`build.mjs`、CodeXArtifacts 自己的元件（`src/ui/`、`src/lib/utils.ts`） |
| `internal/jsruntime/v1/` | 建出來的檔案，**提交進 git**，由 `go:embed` 放進 binary |
| `internal/jsruntime/` | 讀取內嵌檔案與 manifest 的 Go 套件（`Load(jsruntime.V1)`）；`Load` 的結果會記住，不重複讀檔 |

v1 提供的 specifier：`react`、`react/jsx-runtime`、`react-dom`、`react-dom/client`、`recharts`、`lucide-react`、
`@/components/ui/{badge,button,card,dialog,progress,tabs,tooltip}`、`@/lib/utils`（`cn`），另有 Tailwind 的瀏覽器版（傳統 script，不是 module）。

每個 specifier 一個 ESM 檔，檔名帶內容 hash（`recharts-EDLCT6MQ.js`），所以可以用 `immutable` 快取。React 與 React DOM 在其他檔案裡都是 external：
頁面的 import map 把它們指到同一份，頁面不會有兩個 React。共用的程式碼拆成 `chunk-*.js`。

`manifest.json` 記錄：每個 specifier 對應的檔案與它所有的 export 名稱、Tailwind script 的檔名、每個檔案的 SHA-256。
編譯器的允許清單、import map 都從它產生。`NOTICES.txt` 收了每個打包進來的套件的授權全文（沒附授權檔的套件，記錄它宣告的授權與作者；兩者都沒有就讓建置失敗）。

## 怎麼提供給頁面

`GET /vendor/{runtime}/{file}`（`internal/web/vendor.go`）提供 manifest 列出的檔案：`Access-Control-Allow-Origin: *`（opaque origin 的 module script 是 CORS 請求）、
不需要登入、檔名帶 hash 的檔案是 `Cache-Control: public, max-age=31536000, immutable`（`NOTICES.txt` 不帶 hash，只快取一小時）。
這個 binary 沒有的版本、manifest 沒列的檔案（含 `manifest.json`）、路徑穿越都是 404。
`react` kind 的頁面用 manifest 產生 import map（specifier → `/vendor/{format}/{file}`），Tailwind 的瀏覽器版是 manifest 的 `scripts.tailwind`。

## 建置與 gate

```
make runtime          # 重建 internal/jsruntime/v1（約 0.5 秒）
make verify-runtime   # 型別檢查 runtime/src，並比對「重新建一次」與已提交的檔案
```

`verify-runtime` 屬於 `verify-precommit`，所以 `make verify` 包含它。它在暫存資料夾重新建一次，逐 byte 比對 `internal/jsruntime/v1`：
差異、缺少、多出的檔案都會失敗並列出名稱。建一次不到一秒，所以不做快取，也不會有「跳過」的情況需要判斷。
`go test ./internal/jsruntime`（`test-unit` 的一部分）不需要 Node，檢查內嵌的檔案與 manifest 的 hash 一致、檔名帶 hash、
v1 的 specifier 清單沒有被改動、沒有任何檔案引入 runtime 以外的來源、React 只有一份、授權文件涵蓋打包的套件。

輸出只取決於 `package-lock.json`、`package.json`、`runtime/src/` 和 `build.mjs`，不含路徑與時間，所以筆電、CI 和 Docker 建出相同的 bytes。
Dockerfile 的 `runtime` stage 用鎖定檔重新建一次並執行同樣的比對，通過才把檔案交給 Go 的 build stage：image 裡的 binary 內嵌的，就是被 review 過、提交進 git 的檔案。

## 升級一個函式庫

1. 改 `runtime/package.json` 裡的版本（精確版本，不用 `^`），在 `runtime/` 跑 `npm install`（更新 `package-lock.json`）。
2. `make runtime`，看 `git diff --stat internal/jsruntime/v1`：被改的檔案、`manifest.json` 的 export 清單與 `NOTICES.txt` 都會出現在 diff 裡，這就是供應鏈變更的審查點。
3. `make verify`。
4. 一起提交 `runtime/package.json`、`runtime/package-lock.json` 和 `internal/jsruntime/v1/`。

## 什麼時候要新的 runtime 版本

已發佈的頁面要永遠用發佈時的那一組函式庫，所以 `v1/` 一旦有頁面依賴就不可改變它「頁面看得到的內容」：

- **需要新版本（v2）**：移除或改名一個 specifier、移除任何一個 export、React 或其他函式庫的大版本升級、元件（`src/ui/`）的 props 或行為不相容地改變、
  Tailwind 的主要版本改變。新版本是並存的：新增 `internal/jsruntime/v2/`，v1 的檔案原封不動留著，直到沒有頁面依賴它。
- **不需要新版本**：只更新 patch／minor 版本且 manifest 的 export 只增不減、頁面的行為沒有不相容的改變。這種情況重建 v1 即可，檔名的 hash 會跟著變，
  舊頁面透過 manifest 取得新檔名。

artifact 的 `format` 就是它發佈時的執行環境版本（`react` kind 的 `FormatVersions()` 與 `internal/jsruntime` 的版本一一對應），所以新增 v2 不會改變任何已發佈的頁面。
新增 v1 沒有的函式庫（例如 d3）也是新增一個版本，不是修改 v1。
