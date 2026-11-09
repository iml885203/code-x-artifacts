# 新增一個 artifact 種類

每個 artifact 有一個 `kind`，它決定內容怎麼驗證、`/raw/{id}` 回傳什麼頁面、能匯出成什麼。
目前有 `html`（單檔 HTML，自帶 renderer）、`doc`（Markdown 文件，由 CodeXArtifacts 的閱讀頁面畫出來）、`react`（一個 TSX 元件，發佈時編譯）、`slides`（JSON 簡報，平台產生頁面）、`design`（manifest 加每個 frame 一個 HTML 檔，見 [design-format.md](design-format.md)）和 `board`（Excalidraw 元素的 JSON 白板，見 `internal/boardfmt`）。`doc` 是「內容是資料」的第一個例子，可以當範本：`internal/kind/doc.go`、`doc_guide.md`、`doc.css`、`doc_test.go`；`react` 是帶函式庫與編譯器的例子：`react.go`、`react_guide.md`、`react_theme.css`、`react_test.go`。各種類的比較與建議的實作順序見 [artifact-types.md](artifact-types.md)。

## 哪些事屬於 kind，哪些不屬於

屬於 kind（`internal/kind` 的 `Kind` 介面）：

- `FormatVersions()`：這個 build 能驗證與繪製的內容格式版本，舊的在前、最後一個是新 artifact 得到的版本；沒有版本的 kind（`html`）回傳 `nil`。見下面「內容格式版本」。
- `Validate(content, format)`：拒絕不合格的內容，含這個 kind 自己的大小上限。錯誤訊息會原樣回給發佈的人，要寫成 agent 讀了就知道怎麼改。
- `Render(content, format)`：用 artifact 發佈時的版本（不是最新版）回傳 `/raw/{id}` 要顯示的 HTML 文件。
- `Guide()`：給 agent 看的撰寫指南（Markdown，建議用 `go:embed` 放在 `<name>_guide.md`）：合法內容長什麼樣、頁面能做和不能做什麼、怎麼寫才好。
  agent 透過 MCP 的 `kinds` 工具讀到它，這是 agent 學會寫這個種類的唯一文件，所以要自足；`TestEveryKindTeachesItsAuthoringRules` 擋掉空的指南。
- 選用的 `Reviewer`：`Review(content, format, previous)` 回傳發佈者自己看不到的提醒（`slides` 的頁數變化、可能超出畫布的頁、沒有備忘稿的頁），
  放在發佈結果的 `review`（HTTP 與 MCP 相同）；提醒不會拒絕發佈，拒絕是 `Validate` 的事。
- 選用的 `PageRenderer`：頁面完全由平台產生的 kind（`react`、`slides`、`doc`）實作 `RenderPage`，回傳頁面和它每一段 inline script 的原文，
  server 就會在 `/raw` 加上只允許這些 script（hash）、bridge 與 `Page.ScriptPaths`（這個 kind 自己載入 script 的站內路徑，例如 `/vendor/`）的 `script-src`，沒有 `'self'`（[script-src.md](script-src.md)）。頁面沒有自己的 script 時回傳空的（不是 nil）`InlineScripts`，`script-src` 就只有 bridge（`doc`）。頁面是作者自己的標記的 kind（`html`）不實作它，也不會得到 `script-src`。
- 選用的 `Composer`：`Compose(format, written, structured, previous)` 把發佈者送的（`content` 或結構化的 `blocks`）組成要存的內容，結果仍會交給 `Validate`；`doc` 用它替區塊補 id（[doc-format.md](doc-format.md)）。
- 選用的 `FileKind`：內容可以帶檔案（script、樣式、資料，相對路徑）的 kind（`html`、`react`）實作 `AcceptedFileTypes()`；內容要連同整組檔案一起判斷的 kind（`react`、`design`）再實作 `FileValidator`（`ValidateFiles`，發佈、還原、複製都用它），把檔案編進頁面的 kind（`react`）更進一步實作 `Bundler`（再加 `RenderPageFiles`）；路徑、型別、大小規則與儲存由 `internal/files` 和 service 統一處理，見 [multi-file.md](multi-file.md)。不實作的 kind 收到 `files` 會被拒絕。
- 選用的 `FileNormalizer`：檔案要存成一種正規形式的 kind（`design` 的 frame）實作 `NormalizeFile(format, path, text)`（冪等）；service 在內容通過驗證之後對這次送的每個檔案套用，存下來與送來不同的檔案以 `rewritten` 回給發佈的人。選用的 `FileReviewer`：`ReviewFiles(content, format, set, changed)`，`Reviewer` 的檔案版。
- 選用的 `FrameKind`：`FileKind` 的 HTML 檔案不是作者自己的頁面、而是平台框住的文件（`design` 的 frame）時實作 `RenderFrame(path, markup)`，回傳加上平台 script 的文件與那段 script；server 以 `frameCSP`（沒有 bridge、`script-src` 只有那段 script 的 hash）送出，見 [multi-file.md](multi-file.md#design-的-frame)。
- 選用的 `Exporter`：`Formats()`（匯出格式）與 `Export(content, format, exportFormat, origin)`（`format` 是內容格式版本，`origin` 是人們連到這台 server 的位址，離開 server 的檔案用它寫出能開的檔案位址），讓 `GET /api/artifacts/{id}/export/{format}`（`?version=N` 是那一版）與 MCP `export` 下載，檔名取自標題。viewer 的 Export 選單列的就是 `Formats()`，每個格式要在 `internal/i18n/locales/<語言>/export.json` 有 `export.<格式>` 標籤。

不屬於 kind，所有種類一律相同，新增 kind 不會碰到它們：

- 誰能看、誰能改（`internal/service/access.go`）。
- `/raw/{id}` 的 sandbox 標頭與 bridge 注入。`Render` 只回傳文件，標頭和 `window.pictor` 由 server 加上，kind 沒辦法放寬 sandbox，也不會漏掉 bridge。
- 共享資料（`window.pictor.db`）、`whoami`、gallery 與 viewer 外殼。
- 發佈 API 的欄位與權限檢查。

## 步驟

1. 在 `internal/kind/` 新增 `<name>.go`，實作 `Kind`（要匯出就再實作 `Exporter`）。
   `Name()` 是存進資料庫與 API 的識別字，小寫，發佈後不能改。
2. 把它加進 `internal/kind/kind.go` 的 `all`。`Lookup`、`Names` 與錯誤訊息裡的種類清單都從這裡來。
3. 在 `internal/kind/` 為它寫規則層測試：合格與不合格的內容（含剛好在大小上限與超過上限）、`Render` 的輸出、每個匯出格式。
4. 內容是 JSON 或 Markdown 的 kind，或 renderer 帶有函式庫的 kind：用 `FormatVersions()` 列出版本，驗證與繪製都依 `format` 走（見下面「內容格式版本」）。
5. renderer 如果需要靜態資源，以 `go:embed` 內嵌，不新增其他部署物。
6. MCP 的 `kinds` 工具會自動帶出新種類和它的 `Guide()`；`pictor` plugin 的 skill 只指向 `kinds`，不需要改。

`internal/web` 與 `internal/store` 不需要改：store 只存 kind 的名字，web 透過 `kind.Lookup` 找實作。

## 內容格式版本

每個 artifact 存一個 `format`：它發佈時所對的那一版內容格式與執行環境（例如 slides JSON 的版本、react kind 的 React／Recharts 那一組函式庫）。
目的是讓升級 renderer 時，已發佈的頁面不變（goal 原則 7）。

- **誰決定值。** 新 artifact 得到 `kind.CurrentFormat(k)`，也就是 `FormatVersions()` 的最後一個；`html` 沒有版本，值是空字串。
  發佈時可以用 `format` 指定另一個版本（必須在 `FormatVersions()` 裡，否則 400 並列出有哪些）。
- **更新。** 省略 `format` 就沿用 artifact 自己的版本，內容用那一版驗證；指定 `format` 就是作者明確升級（或釘住），內容必須通過那個版本的驗證，不通過時什麼都不改。
  不會自動升級，也不會自動遷移舊內容（那是個別 kind 的後續工作）。
- **規則。** 已經發佈的版本不能從 `FormatVersions()` 移除，`Render` 要能畫每一個列出的版本。
  升級大版本時：新增版本、保留舊版（`/vendor/v1/…` 與 `/vendor/v2/…` 並存），舊 artifact 照舊。
- **這個 build 不認得的版本**（較新的 build 寫的、之後又回滾）：`/raw/{id}` 與匯出回 501，不猜。
- **看得到的地方。** HTTP 與 MCP 的 artifact 結果有 `format`（空字串時省略）；MCP `kinds` 工具列出每個 kind 的 `formatVersions` 與 `currentFormat`；MCP `publish` 與 HTTP 發佈接受選用的 `format`。
- **測試。** `internal/kind/kindtest` 提供一個有多個版本的測試 kind（`kindtest.Register(t, "v1", "v2")`），在還沒有有版本的 kind 時用來測 service 與 server；
  新 kind 要寫自己的測試：每個版本的 `Render` 輸出、舊版內容在新版 build 仍能畫。

## 現況與限制

- 發佈請求的欄位是 `{title, kind?, format?, content, visibility?}`。`html` 是 `content` 以前的名字，只對 `html` 種類接受。`format` 見上面「內容格式版本」。
- `kind` 省略時：新 artifact 是 `html`，更新時沿用原本的種類。artifact 的種類建立後不能改。
- 未知的 `kind` 在發佈時以 400 拒絕，訊息列出這台 server 認得的種類。
  資料庫裡出現這台 server 不認得的 kind（較新的版本寫的），`/raw/{id}` 回 501，不會當成 HTML 輸出。
- 儲存層只有一份主內容（`artifacts.html` 欄位，欄位名保留是為了回滾到舊版時仍讀得到）加上 `format`。
  二進位資產要等第一個需要它們的 kind 再加，見 artifact-types.md。
