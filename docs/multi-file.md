# 多檔 artifact（`html` 與 `react` 帶檔案）

這份文件定案「一個版本 = 進入頁面 + 一組相對路徑的檔案」怎麼存、怎麼在 opaque origin 裡載入、套用什麼 CSP、怎麼發佈，
以及它守住的不變式。形狀跟 Claude 的多檔 artifact 一致（版本 = 頁面 + `files`，檔案與上傳媒體分開，限制相近），
刻意不同的只有執行環境（見「與 Claude 的差異」）。URL 方案已用 `spikes/multi-file/` 在真的 Chrome 驗證過（見「Spike 結果」）。

**實作範圍。** `html` 與 `react` 帶檔案都已完成：儲存、grant 路徑、`script-src` 去掉 `'self'`（react 加自己的 `/raw/{id}/` 前綴）、react 的 sibling import、
HTTP 與 MCP 的 `files`、還原、複製、匯出的拒絕、刪除。`doc`、`slides` 收到 `files` 一律拒絕（`kind.FileKind` 由 `html`、`react` 與 `design` 實作；`design` 的檔案是它的 frame，見下面「design 的 frame」）。`export/zip` 還沒做（見「後續 issue」）。

## 要解決的問題

目前一個 `html` artifact 是一份自帶一切的檔案，大一點的頁面只能把 JS、CSS、資料全塞進同一個檔案；
上傳的 assets（圖片、字型、PDF、CSV）不屬於版本、不能放程式。Agent 想要的是：

```
index.html   <script src="app.js"> <link href="styles/site.css"> fetch("data/rows.json")
app.js
styles/site.css
data/rows.json
```

而且改一支檔案就能發一個新版本，舊版本連同它的檔案一起留在歷史裡、能還原。

難處在 `/raw/{id}` 是 opaque origin（CSP `sandbox`，沒有 `allow-same-origin`）：

1. 文件的 base URL 是 `/raw/{id}`，所以 `app.js` 會解成 `/raw/app.js`，不是這個 artifact 的檔案。
2. 頁面的子資源請求不帶 cookie（initiator 是 opaque origin，視為 cross-site，Lax cookie 不送；module script 與 `fetch` 是 CORS 請求，不帶憑證），
   所以每個檔案請求要自己帶憑證，也就是 grant（`internal/web/session.go`）。
3. html 頁面沒有 `script-src`（`docs/script-src.md`）；react 頁面有，而且現在是 `'self'`。放進「使用者上傳的 JS」之後，`'self'` 會變成繞過（見下面「CSP」）。

## 與 Claude 的差異（刻意的）

- **CodeXArtifacts 的頁面維持 opaque origin：`sandbox` 不含 `allow-same-origin`。** 這是 goal 原則 1 的邊界，多檔不放寬它；
  所以頁面沒有自己的 `localStorage`、`sessionStorage`、cookie、IndexedDB，**要保存的資料用共享 db（`window.pictor.db`）**，它還能讓所有看得到頁面的人共用。
  Claude 的頁面可以用瀏覽器儲存；CodeXArtifacts 的 guide 要寫明這一點，也因此相對 URL 需要 grant 這一層（下面）。
- 其餘（頁面 + `files`、檔案與上傳媒體分開、限制的量級）跟 Claude 一致；react 只是寫同一種 artifact 的另一種方式，行為與 html 相同。

## 模型

- 一個版本 = **進入頁面**（現有的 `content`，不變）+ **0 到 N 個檔案**（路徑 → 文字內容）。
- 進入頁面的檔名固定是 `index.html`，在 URL 裡就是目錄本身（`…/` 與 `…/index.html` 是同一份）；檔案不能叫 `index.html`。
- 檔案只收**文字型**：`.html`、`.js`、`.mjs`、`.css`、`.json`（issue 的 allow-list），`files` 的值是字串，不需要 base64。
  **上傳的媒體（圖片、字型、PDF、CSV）留在 asset store（`/files/…`），不屬於版本，行為不變**（跟 Claude 一樣：檔案與上傳媒體是兩個東西）。
- `html` 與 `react` 收檔案，**行為相同**：react 只是寫這種 artifact 的另一種方式，頁面一樣用相對路徑載入同版本的檔案（`fetch("data/rows.json")`、`<link>`、`import`）。
  `doc`、`slides` 收到 `files` 時以「a doc artifact takes no files」之類的訊息拒絕，不是忽略。
- 路徑規則（發佈時驗證，拒絕而不是改寫，免得 agent 寫的相對路徑和存下來的不同）：
  `/` 分隔、最多 5 層、整條 ≤ 200 字元、每段只含 `A-Za-z0-9._-` 且不以 `.` 開頭、不含空段與 `..`、
  兩個路徑只差大小寫視為衝突、不能是 `index.html`。副檔名決定型別，內容必須相符：UTF-8、無 NUL、`.json` 能 parse。
- 檔案是版本的一部分，所以**不可變**：沒有「單獨替換一個檔案」的 API，換檔案就是發佈一個新版本。
  這是「改一支檔案就能更新」與「歷史完整」同時成立的唯一做法。

### `files` 是對上一版的差異

發佈時的 `files` 是 `{路徑: 文字 | null}`，套用在 `baseVersion`（沒帶就是最新版）的檔案組上：

| 寫法 | 結果 |
|---|---|
| 沒有 `files` | 檔案組原封不動帶到新版本（只改頁面不用重送檔案） |
| `"app.js": "…"` | 新增或取代 `app.js` |
| `"old.js": null` | 從新版本移除 |
| 沒提到的路徑 | 帶過去 |

理由：agent 不必每次重送所有檔案（token 成本），而 `baseVersion` 已經保證差異是套在它看過的那一版上
（版本不是最新就整個拒絕，既有不變式，沒有新規則）。這個套用與版本列都在同一個 transaction。

## 方案比較：相對 URL 怎麼解成帶 grant 的網址

| | A. 路徑帶 grant（建議） | B. `<base href>` | C. 改寫 HTML/CSS/JS 裡的參照 | D. blob URL + bridge 載入器 | E. 每個 artifact 一個子網域 |
|---|---|---|---|---|---|
| 做法 | 文件網址就是 `/raw/{id}/{grant}/`，相對 URL 由瀏覽器自然解成同層 | 注入 `<base href="/raw/{id}/{grant}/">` | server 掃描並改寫 `src`、`href`、`url()`、`import` | bridge 取檔、建 blob URL、產生 import map | `{id}.pages.host`，同源即可 |
| `<script src>`、`<link>`、CSS `url()` | 可 | 可 | 要 parser | 要改寫 | 可 |
| ES module `import`、`import()`、`import.meta.url` | 可 | 可 | 改不到動態字串 | 要 import map，動態的不行 | 可 |
| `fetch("data/x.json")`、`new URL(x, base)` | 可 | 可 | 改不到 | 要覆寫 `fetch` | 可 |
| `<a href="#x">` | 可 | **會離開頁面**（解成 base + `#x`） | 可 | 可 | 可 |
| 作者自己的 `<base>` | 沒影響 | 衝突 | 沒影響 | 沒影響 | 沒影響 |
| 巢狀頁面 `sub/page.html` 的 `../` | 可 | 要為每頁算 base | 要算 | 要算 | 可 |
| 需要的部署改動 | 無 | 無 | 無 | 無 | wildcard DNS、憑證、ingress：目前只有單一 host |
| 維護風險 | 低：一個路由 | 中：`#` 與 `<base>` 的怪行為 | 高：parser 與瀏覽器不一致就是壞頁面或繞過 | 高：自己實作一半的瀏覽器 | 低，但現在做不到 |

**建議 A。** 它是唯一同時滿足「完全不碰作者的內容」「所有瀏覽器原生的解析路徑都有效」「不需要部署改動」的方案。
B 看起來最簡單，但 spike 證實 `<a href="#bottom">` 會把 frame 導到另一個網址，agent 寫的頁面幾乎都有錨點。
E 是網頁標準做法，單一 host 的部署做不到；如果之後有了 wildcard 網域，A 的路由可以原樣搬過去，不需要改頁面。

## URL 與 grant（方案 A 的細節）

```
GET /raw/{id}                       cookie 驗證，不變；artifact 的這個版本沒有檔案就跟今天一樣直接回頁面
GET /raw/{id}?version=N             同上，只有作者
GET /raw/{id}/{grant}/{path...}     只靠 grant 驗證；path 空或 index.html 是進入頁面，其他是檔案
```

1. `/raw/{id}`（與 `?version=N`）檢查完權限後，**只有該版本有檔案時**回 `302` 到 `/raw/{id}/{grant}/`。
   沒有檔案的 artifact 一個 byte 都不變，所以既有頁面、react、slides、doc 的行為與不變式完全不受影響。
   viewer 的 iframe 仍然指向 `/raw/{id}`，前端不需要知道 grant。
2. grant 現在簽 `artifactID`、`visibility`、`version`、`exp`：`grantPayload = "file:{id}:{visibility}:{version}:{exp}"`。
   既有的 `/files/…` 的 grant 不含版本，payload 前綴不同（`asset:` / `file:`），不會互相冒充。
3. **檔案網址是版本定址的**：grant 釘住簽發當下頁面的版本，這個網址只讀該版本的檔案。
   所以頁面開著的時候作者發了新版，舊頁面延遲載入的檔案（動態 `import()`、`fetch`）仍是**它自己那一版**的，不會混到新版的檔案，也不會 404；
   作者開舊版本（`?version=N`）得到的就是釘住 N 的 grant。能不能開舊版本是在簽發前由 `/raw/{id}?version=N` 的作者檢查決定的（既有規則）。
   代價（接受）：作者刻意拿掉的檔案，已經打開的頁面在 grant 有效的 6 小時內仍讀得到它當時那一版的檔案。
4. 失效條件同現有：**6 小時到期**（對齊到整點，頁面載入時取得一次，要看新內容就重新整理）、artifact 的受眾改變就失效。
5. 找不到的路徑、錯的 grant、看不到的 artifact、釘住的版本不能讀，**回應完全相同**（404），不洩漏任何一項存在與否。
6. 回應一律帶 `Referrer-Policy: no-referrer`（頁面載入第三方資源時不把含 grant 的網址送出去），`X-Content-Type-Options: nosniff`、
   `Cross-Origin-Resource-Policy: cross-origin`、`Access-Control-Allow-Origin: *`（opaque origin 的 module script 與 `fetch` 是 CORS 請求）。
7. server 的請求紀錄不寫 grant 那一段。

**這個網址本身就是 6 小時的 capability**：拿到 `/raw/{id}/{grant}/` 的人能讀那個版本的頁面與檔案，和今天的 `/files/{id}/{grant}/{name}` 同一個層級的風險
（私人 artifact 的圖片網址外流也一樣）。它不能擴大受眾：受眾一改就失效，而且簽發只發生在 `CanView` 通過之後。

## 每種檔案的標頭與 CSP

| 檔案 | `Content-Type` | 文件 CSP | bridge |
|---|---|---|---|
| 進入頁面、`.html` | `text/html; charset=utf-8` | **與進入頁面完全相同**（`sandbox …; frame-ancestors 'self'`；html kind 沒有 `script-src`），`Cache-Control: no-store` | 注入（`window.pictor` 在頁面互相連結後仍可用） |
| `.js`、`.mjs` | `text/javascript; charset=utf-8` | `sandbox; default-src 'none'` | 否 |
| `.css` | `text/css; charset=utf-8` | 同上 | 否 |
| `.json` | `application/json; charset=utf-8` | 同上 | 否 |

- 子頁面和進入頁面共用同一個 `artifactCSP`，所以 sandbox 不會因為是「另一頁」而不同；kind 與檔案都放寬不了。
  這由「頁面就是 opaque origin」這條不變式涵蓋，測試要掃每一個 `.html` 檔案。
- 非 HTML 檔案被直接開在分頁裡不會當文件執行（`nosniff` 加上 JS/CSS/JSON 的型別），再加上 `sandbox; default-src 'none'` 是和 active assets 同一種防線。
- `ETag` = 內容 SHA-256（含 grant 的雜湊，因為 `/files/{id}/` 換成帶 grant 的網址會讓 body 隨 grant 變），`Cache-Control: private, max-age=3600`（grant 每小時換，網址跟著換）。
- 檔案內容裡的 `/files/{id}/` 資產網址和進入頁面一樣被換成帶 grant 的網址（`assetGrants`，只套在 `.html`、`.css`、`.js`、`.mjs`，不動 `.json`），
  所以 `app.js` 裡的 `"/files/{id}/logo.png"` 與頁面裡的一樣能載入。

### react 頁面的 `script-src`：釘在 `/raw/{id}/` 前綴加 hash

react 頁面原本是 `script-src 'self' 'sha256-…'`。`'self'` 涵蓋 `/vendor/…`，**也涵蓋同一個 host 上任何別人上傳的 `.js`**：
只要第三者能把一個 `<script src>` 寫進受害者的頁面（`docs/script-src.md` 防的就是這個），他就能指向自己 artifact 的 `evil.js`，
帶著受害者的 bridge 執行。spike 證實：`script-src 'self'` 會載入另一個 artifact 的腳本，把 `script-src` 釘在 `/raw/{id}/` 前綴就會擋下，而自己的檔案、module、動態 import 都照常載入。

react 與 html 一樣走 grant 路徑（頁面在 `/raw/{id}/{grant}/`），所以：

- react 頁面的 `script-src` = `<origin>/vendor/`（`Page.ScriptPaths`：執行環境的函式庫與 Tailwind 的瀏覽器版）+ 有檔案時加 `<origin>/raw/{id}/`（它自己的檔案，路徑前綴比對，與 grant 無關）+ bridge 與頁面 inline script 的 hash。
  沒有 `'self'`、`unsafe-inline`、`unsafe-eval`。另一個 artifact 的 `/raw/{其他id}/…/evil.js` 不在清單內，注入不了；沒有檔案的 react 頁面沒有 `/raw/` 前綴。
- `.js` 檔案型別讓**`'self'` 本身變成風險**，不論 react 有沒有檔案，所以所有 `PageRenderer` 頁面（react、slides、doc）一併把 `'self'` 換成明確的來源
  （slides 只需要 bridge 與播放器的 hash，doc 只需要 bridge 的 hash）。這與多檔同時發佈。`/kit/`、`/static/fonts/` 是 CSS 與字型，不在 `script-src`。
- html 仍然沒有 `script-src`（`docs/script-src.md` 的決定不變）。

## react 的 sibling import

react 的內容是一個 TSX component，發佈時由 esbuild 編成**一支** module script（inline、hash 進 `script-src`）。有檔案時：

- esbuild 的 `resolver` plugin 以 `pictor:component`（`content`）為根，相對路徑 `./Chart`、`./lib/format.ts`、`./data.json`、`./styles.css`
  由該版本的檔案組解析（esbuild 的 namespace 是 `sibling`；`file` 留給磁碟上的真實路徑）。路徑從 import 它的檔案所在的資料夾讀（`../lib/format`），
  含 `..` 逃出檔案根目錄就拒絕。TSX/TS/JSX 瀏覽器不能直接執行，所以在發佈時編進同一支 module；`.js`、`.mjs`、`.json` 被 `import` 時一起編進去，
  沒被 import 時就是一般檔案，頁面照樣能 `fetch("data/rows.json")`（react 與 html 行為相同）。副檔名省略時依序試 `.tsx .ts .jsx .js .mjs .json .css`（與 esbuild 預設一致）。
  `.html` 不能被 import。不是相對路徑的名稱只能是 runtime 的 specifier：同名的檔案不會蓋掉套件。
- react 可以收的檔案型別與 html 相同，另外多 `.tsx .ts .jsx`（`files.SourceTypes`，只能被 import，不會被當檔案提供：grant 路徑上要它們是 404，作者仍能用 API 讀）。
  被 import 的 CSS 以 `<style>` 內嵌進頁面（inline style 不受限；`url()` 原樣留給瀏覽器對頁面位址解析），含 `</style` 的 CSS 被拒絕；被 import 的 JSON 編進 bundle。
- 解析不到、含 `..` 逃出根目錄、想 import 一個沒有的檔案：編譯錯誤，帶行號（`reactProblems`），一次回報所有問題；沒有任何檔案時訊息說要用 `files` 發佈，有檔案時列出現有的路徑。
- 沒被 import 的檔案照樣存（放著沒有害處，agent 可能分段發佈）。TypeScript 會丟掉沒用到的 import，所以沒用到的 import 不會被解析。
- 編譯快取的 key 是 `(format, content, 每個檔案的路徑與內容)`；只改標題或可見範圍不重編。
- bundle 大小上限：輸出的 script 與 style 各 ≤ 16 MB，超過就以可讀的原因拒絕，避免用檔案撐出巨大的 inline script。
- 第三方套件仍然只能 import runtime 提供的 specifier（`react`、`recharts`…），不變。

service 對 `kind.Bundler`（react）才讀出整組檔案的文字（發佈、還原、複製、`/raw` 的頁面），驗證時內容與檔案一起編譯；`html` 的檔案只由瀏覽器載入，發佈不讀它們。
所以 react 的多檔 = 編譯輸入從一個字串變成一個檔案組（sibling import），加上與 html 完全相同的檔案服務（相對路徑、grant、同一套規則）。

## 儲存

位元組放在資料卷上、以內容雜湊定址；每個版本的清單（manifest）放在 SQLite，與版本列同一個 transaction。

| | 做法 |
|---|---|
| 位元組 | `internal/assets` 的 `Disk` 同一個資料卷：`{assets dir}/{artifactID}/blobs/{sha256}`。內容定址，同一份內容在很多版本只存一份，寫入冪等；每個 artifact 自己一個目錄，刪 artifact 就刪整個目錄 |
| manifest | SQLite 表 `version_files`：每個版本的每個路徑一列（路徑、sha256、content type、大小），主鍵 `(artifact_id, version, path)`，外鍵 cascade 到版本 |
| 原子性 | 發佈先把新的 blob 寫到磁碟（冪等），再開一個 transaction：`baseVersion` 檢查、版本列、`version_files` 列一起成功或一起失敗。看得到的狀態只有「整個版本」或「什麼都沒有」；失敗只會留下沒被任何 manifest 引用的 blob（孤兒），只佔空間 |
| 孤兒 | 啟動時與每日一次掃描，刪掉沒被引用且超過 1 天的 blob（比任何進行中的發佈久，不會刪到剛寫好、還沒進 transaction 的）。版本永不刪除，所以沒有別的 blob 回收 |

```sql
CREATE TABLE version_files (
  artifact_id  TEXT NOT NULL,
  version      INTEGER NOT NULL,
  path         TEXT NOT NULL,
  sha256       TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size         INTEGER NOT NULL,
  PRIMARY KEY (artifact_id, version, path),
  FOREIGN KEY (artifact_id, version) REFERENCES artifact_versions(artifact_id, number) ON DELETE CASCADE
);
```

- `artifacts` 列繼續存最新版的 `html`（回滾到舊 binary 仍讀得到，現有不變式）；舊 binary 不認得 `version_files`，只會看到進入頁面，不會壞。
- 既有 artifact 沒有檔案，不需要資料遷移；migration 只新增這個表。「最新版的檔案」就是 `version_files WHERE version = artifacts.version`。
- 讀檔時以 manifest 的 sha256 開 blob；blob 缺了是明確的 500 與 log，不是空檔。

## 限制

固定在程式碼裡（和 assets 一樣，讓工具說明能直接告訴 agent 數字），量級對齊 Claude：

| 項目 | 值 |
|---|---|
| 單一文字檔（`files` 的一項） | 16 MB |
| 單一二進位檔（asset store） | 15 MB（現在 10 MB，一併調高；MCP 內嵌上傳的 5 MB 不變） |
| 一次 publish 的檔案數 | 255 |
| 一個版本的檔案數 | 511 |
| 一個版本的檔案總和 | 256 MB |
| 一次 publish 請求本體 | 64 MB（現在的 `maxRequestBytes` 10 MB 對帶 `files` 的請求提高到這個值；沒帶 `files` 的請求不變） |

- 進入頁面的 `html` 上限也對齊 16 MB（現在 `MaxHTMLBytes` 是 10 MB）；`doc`、`react`、`slides` 自己的上限不變。
- 所有版本的 blob 因內容定址而共用；歷史永不刪除，磁碟用量只增不減，與現有版本歷史的取捨相同。發佈時 log 寫出該 artifact 的 blob 總量，讓它可觀察；要不要加上限等有實際資料再決定。
- react 另有 bundle 上限：輸出的 module ≤ 16 MB，超過以可讀的原因拒絕。
- 所有拒絕訊息都要說出是哪個限制、現在多少、怎麼改。

## 發佈的形狀

HTTP 與 MCP 是同一個 service 呼叫，欄位相同（既有慣例）。

**HTTP**：`POST /api/artifacts`、`PUT /api/artifacts/{id}` 的 JSON 多一個選用欄位 `files`（`{路徑: 字串 | null}`）。
回應的 artifact 多 `files: [{path, contentType, size, sha256}]`（最新版；沒有檔案時不帶；列表不帶），不含內容。
新增 `GET /api/artifacts/{id}/files/{path...}`（`?version=N`，舊版本只有作者與編輯者）讀單一檔案的原文，回 `text/plain`、`nosniff`、`sandbox; default-src 'none'`，不會被當文件開；
`GET /api/artifacts/{id}/versions/{n}` 回應多 `files` 清單；`GET …/versions` 的每筆多 `fileCount`。
帶 `files` 欄位的請求本體上限 64 MB，沒帶的維持 10 MB。

**MCP**（`internal/mcptools`）：

- `publish` 多 `files`（`map[string]*string`）：「路徑 → 這個檔案的完整文字；`null` 刪除；沒寫到的檔案原封不動帶到新版本」。
  工具說明要寫清楚：進入頁面的 `content` 用 `<script src="app.js">`、`<link href="styles/site.css">`、`fetch("data/rows.json")` 這樣的**相對路徑**引用；
  react 用 `import … from "./Chart"`；不要用 `/` 開頭的路徑；檔案型別、數量與大小限制；上傳的圖片仍用 `upload_asset` 得到的網址。
- `read` 回應多 `files: [{path, contentType, size}]`，並多一個選用參數 `file`（路徑）：給了就回那個檔案的文字。agent 因此能「讀、改一支、只送那一支」。
  `version` 參數同樣適用。
- `versions` 的每筆多 `fileCount`（檔案數，沒有檔案時不帶，與 HTTP 相同）。`restore` 的行為見下。
- `kinds` 的 html 指南加一段多檔的寫法（`Guide()` 是 agent 學會它的唯一文件，`TestEveryKindTeachesItsAuthoringRules` 的精神）；react 的指南也有（`import Chart from "./Chart"`）。
- 不新增 `put_file` 之類的單檔工具：檔案屬於版本，單獨改一個檔案就是發一個帶一個 `files` 項目的版本。

**kind 介面**：`Validate`、`Render` 的簽名不變。新增選用介面（與 `Reviewer`、`Exporter` 同一種模式），宣告「這個 kind 收檔案」：

```go
// FileKind is implemented by kinds whose content can come with files.
type FileKind interface {
    Kind
    // AcceptedFileTypes lists the extensions the kind takes.
    AcceptedFileTypes() []string
}
```

檔案的路徑、型別與內容規則在 `internal/files`，由 service 對每個 kind 一視同仁地套用（`html` 只需要宣告收哪些型別）。
不實作 `FileKind` 的 kind 收到 `files` 一律拒絕。編譯檔案的 kind（react）另外實作 `kind.Bundler`：`ValidateFiles(content, format, files.Set)` 與 `RenderPageFiles(content, format, files.Set)`，讓 esbuild 看得到整組檔案。

## 歷史、還原與複製

- **發佈**：新版本的檔案組 = 上一版（`baseVersion`）套差異；版本列與檔案列同一個 transaction。
- **舊版本**：`/raw/{id}?version=N` 對作者回 302 到釘住 N 的 grant 網址；頁面與檔案都是 N 的，bridge 仍是唯讀（現有規則）。
  非作者 403、看不到 404、編號不對 400，與現有一致。
- **還原**：新版本，檔案組 = 被還原那一版的檔案組（共用 blob，只多版本列）。保留該版本的 `format`，受眾不變，`restoredFrom` 照舊。
  還原前和現在一樣重新驗證，這次含檔案（`ValidateFiles`）。
- **複製（#55）**：複本的 version 1 帶**最新版的整個檔案組**（新 artifact 自己的 blob）。
  相對路徑不需要改寫（這是方案 A 的好處之一）；需要改寫的只有 assets 的絕對網址 `/files/{來源id}/` → `/files/{新id}/`，
  且要套在進入頁面**和每個 `.html`、`.css`、`.js`、`.mjs` 檔案**，改完重新計算 SHA-256。複製與 assets 共用一個改寫函式，#55 的實作要用它。
  複製不帶舊版本、共享 db、留言（#55 已定）。
- **匯出**：`export/html` 對有檔案的版本會是壞掉的下載（相對參照指向不存在的檔案），所以**拒絕並說明**，不靜默給出壞檔；
  `export/zip`（進入頁面為 `index.html` 加所有檔案）是後續 issue（見下）。
  拒絕是 400，訊息說明有幾個檔案、怎麼取得（`GET /api/artifacts/{id}/files/{path}` 或複製）。
- **刪除**：檔案隨 artifact 一起刪（`ON DELETE CASCADE`）；MCP `delete` 的確認清單多列檔案數。
- **assets 與檔案的關係**：兩者網址空間不同（`/files/…` 與 `/raw/{id}/{grant}/…`），互不衝突。多檔頁面引用上傳的圖片仍寫 `upload_asset` 回傳的絕對網址。

## Spike 結果

`spikes/multi-file/`（Node、零相依，`PLAYWRIGHT_DIR=<有 playwright 的資料夾> node spikes/multi-file/run.mjs`，用系統 Chrome），不是產品程式，也不在 `make verify`。
模擬：cookie 驗證的 `/raw/{id}` 302 到 `/raw/{id}/{grant}/`，grant 為 HMAC，檔案只靠 grant 驗證，iframe 為 `sandbox="allow-scripts allow-forms allow-popups allow-modals"`。結果（23 項全過）：

- 文件 origin 是 `null`；`<script src="app.js">`、`<script type="module" src="./m.js">`、靜態 `import "./lib/dep.js"`、動態 `import("./lib/dyn.js")`、
  `new URL("data/rows.json", import.meta.url)`、`fetch("data/rows.json")`（`Origin: null`）、`<link rel=stylesheet>` 與 CSS 內的 `url(../img/dot.svg)` 都解到正確的檔案。
- `<a href="#bottom">` 停留在同一頁；`<a href="other.html">`、`sub/page.html`、巢狀的 `../styles/site.css`、`../app.js`、`../other.html` 都對，子頁面仍是 opaque origin。
- 只有 frame 的導覽請求帶 session cookie；23 個子資源請求**沒有一個**帶 cookie（grant 是唯一憑證），module 與 `fetch` 請求的 `Origin` 是 `null`。
- 別的 artifact 的 grant、被竄改的 grant、不存在的路徑、沒有 cookie 也沒有 grant：全是 404。
- 對照組：`<base href>` 讓 `<a href="#bottom">` 把 frame 導到另一個網址（拒絕方案 B 的依據）。
- CSP：`script-src 'self'` 載入了另一個 artifact 的 `evil.js`（繞過成立）；`script-src <origin>/raw/{id}/` 擋下它，同時自己的 script、module、動態 import 都正常。
  （spike 的 CSP 為了測試加了 `'unsafe-inline'`，產品不加。）

## 不變式與測試

不變式與它們的測試名稱在 `internal/files/AGENTS.md` 與 `internal/web/AGENTS.md`，這裡只留設計層面的對照：

- **版本 = 進入頁面 + 檔案組**，兩者同一個 transaction，永不覆寫；還原是新版本、帶回那一版的檔案組。
- **頁面載入自己的檔案只靠網址裡的 grant**：grant 釘住一個版本與一個受眾，受眾一改就失效、6 小時到期、舊版本只有作者與編輯者能取得；看不到、錯的 grant、不存在的路徑、讀不到的版本，回應完全相同（404）。
- **沒有檔案的 artifact 的 `/raw/{id}` 與以前完全相同**（不轉址）。
- **每個 HTML 檔案和進入頁面一樣在 opaque origin 的 sandbox 裡、帶 bridge；非 HTML 檔案不是文件。**
- **`PageRenderer` 頁面的 `script-src` 不含 `'self'`**，所以別人上傳的檔案不能被注入成 script（react 帶檔案時再加自己的 `/raw/{id}/` 前綴）。
- **檔案只收文字型、路徑嚴格驗證、限制明確**；不收檔案的 kind 拒絕而不是忽略。
- **位元組內容定址、manifest 與版本同一個 transaction**；缺的 blob 是錯誤，不是空檔。
- **頁面沒有瀏覽器儲存（opaque origin），指南教 agent 用共享 db。**
- **HTTP 與 MCP 在 `files` 上一致**；複製、匯出、刪除、log 都有規則。
- react 的 sibling 原始檔編譯進同一支 module：`TestAReactArtifactImportsItsSiblingFilesAndTheyAreBundledIntoOneScript`、`TestAReactImportOfAMissingFileNamesTheFilesThatExist`、
  `TestAReactImportCannotEscapeTheArtifactsFiles`、`TestAReactCSSImportBecomesAStyleElementNotAScriptSource`、`TestTheReactCompileCacheKeyIncludesTheFiles`、
  `TestAReactPageAllowsScriptsFromVendorItsOwnFilesAndItsOwnHashesOnly`，e2e 在 Chrome 跑 sibling component 與相對 `fetch`。

e2e（`e2e/tests/multifile.spec.ts`）：兩檔頁面（`index` + `app.js` + 樣式 + module 與動態 import + `fetch` JSON）執行且 JS 改了頁面；連結到另一頁、巢狀頁、頁內錨點；
作者看舊版本時檔案是舊的、沒有權限的讀者每個網址都是 404；另一個 artifact 的 script 載不進 react 頁面。react 帶 sibling component 與相對 `fetch` 的 e2e 也在同一個檔案。

## 實作切分

1. 完成：store migration（`version_files`）、blob 寫入與掃描、限制的調整（16 MB、15 MB、64 MB）、`internal/files`（路徑與型別規則，純函式）、service 的發佈／還原／讀取／複製帶檔案、`FileKind`（html）。
2. 完成：grant 加版本、`/raw/{id}` 轉址與 `/raw/{id}/{grant}/{path...}` 路由、`PageRenderer` 的 `script-src` 去掉 `'self'`。
3. 尚未實作：react 走 grant 路徑、`script-src` 釘 `/raw/{id}/` 前綴、sibling import。
4. 完成：HTTP 與 MCP 欄位、`read` 的 `file`、說明與 guide、匯出的拒絕、刪除確認的檔案數。
5. 完成：各目錄 `AGENTS.md` 的不變式與根目錄的結構、`docs/adding-a-kind.md`（`FileKind`）、`docs/script-src.md`（來源清單）。

## 後續 issue（不在 #56）

- `export/zip`：有檔案的版本下載成 zip。
- assets 併入版本檔案：讓上傳的圖片也屬於版本（還原會帶回圖片）、且能用相對路徑引用。Logan 已決定上傳的媒體留在版本之外（跟 Claude 一樣），所以這條不做。
- JS 語法檢查：發佈時用 esbuild 對 `.js` 做 parse，讓 agent 在發佈當下就知道語法錯誤（現在 html 頁面也沒有）。

## 已決定（Logan）

- react 與 html 行為相同：可用相對路徑 `fetch`，走 grant 路徑，`script-src` 釘 `/raw/{id}/` 前綴加 hash。
- grant 維持 6 小時。
- 上傳的媒體留在版本之外（asset store 與 `files` 分開）。
- 限制對齊 Claude（見「限制」）；位元組在資料卷、內容定址，manifest 在 SQLite。
- 舊版本寬鬆：檔案網址版本定址，打開的舊頁面繼續讀它自己那一版的檔案。
- CodeXArtifacts 保持 opaque origin，不加 `allow-same-origin`；頁面用共享 db，不用 `localStorage`。

## 開放問題（給 Logan）

沒有阻擋實作的問題。一件小事：`export/zip` 要不要在 #56 一起做（目前列為後續 issue，`export/html` 對有檔案的版本先拒絕）。

## design 的 frame

`design` 的內容是 `design.json`（manifest），檔案是每個 frame 一份 `frames/<名稱>.html`，只收 `.html`。檔案的儲存、grant、404、差異語意、還原、複製與其他多檔 artifact 完全相同；不同的只有 `.html` 檔案怎麼送出：
`design` 實作 `kind.FrameKind`，所以 grant 路徑上的 `.html` 不是「作者的頁面加 bridge」，而是平台框住的 frame：

- 沒有 bridge（frame 沒有 `window.pictor` 要用）。
- 政策：`sandbox allow-scripts; default-src 'none'; script-src 'sha256-<inspector>'; style-src 'unsafe-inline'; img-src data: <origin>/files/{id}/; font-src data: <origin>/static/fonts/ <origin>/files/{id}/; base-uri 'none'`，沒有 `frame-ancestors`：frame 被 opaque origin 的總覽頁嵌著，`'self'` 對 opaque 的祖先不成立，而嵌入不給對方任何東西（憑證是網址裡的 grant）。
  比一般頁面更嚴：沒有 `allow-forms`、`allow-popups`、`allow-modals`、`allow-downloads`；沒有 `'self'`、`unsafe-eval`；唯一能跑的 script 是 inspector（`internal/kind/design_inspector.js`，放在 `<head>` 之後、作者的任何東西之前，只回應 `source === parent` 的訊息）。
- 進入頁面（`/raw/{id}`）是 design 的總覽，一般文件、帶 bridge（見 [design-format.md](design-format.md)）。
