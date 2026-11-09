# `/raw` 要不要加 `script-src`

結論：**html kind 不加；react、slides 與 doc kind 加**（已上線）。理由與代價如下。

## 現況

`/raw/{id}` 只帶 `Content-Security-Policy: sandbox …`（不含 `allow-same-origin`）。頁面是 opaque origin，拿不到看的人的 cookie，
也不能以看的人的身分呼叫 API，這是邊界（goal 原則 1），不需要 `script-src` 來守。頁面載入的東西（kit、字體、CDN 函式庫）從 opaque origin 看都是跨來源請求，
所以 `/kit/` 與 `/static/fonts/` 回 `Access-Control-Allow-Origin: *`。

## `script-src` 能多擋什麼

頁面的程式碼是作者寫的，作者本來就能在 sandbox 裡做 sandbox 允許的事，`script-src` 擋不了作者。它擋的是**第三者的字串變成程式碼**：
頁面把共享資料（`pictor.db`，任何看得到頁面的人都能寫）用 `innerHTML` 之類的方式畫出來，別人寫進去的 `<img onerror=…>` 就在受害者的頁面裡執行，
帶著受害者的 bridge：能讀 `whoami`、能以受害者的名字寫 db。sandbox 擋不住這件事，因為執行的是頁面自己的 origin。
沒有 `unsafe-inline` 的 `script-src`（hash 或 nonce）可以擋掉。

## 代價

| 項目 | html kind | react kind（見 [react-kind.md](runtime.md)） |
|---|---|---|
| inline `<script>` | 頁面是任意單檔，server 得解析 HTML、替每段 inline script 算 hash（或加 nonce）；解析器與瀏覽器不一致就是繞過或誤擋 | 頁面由 server 產生，只有 bridge、import map、一段 module script，hash 在產生時就知道 |
| inline 事件屬性（`onclick=`） | 很常見（agent 寫的頁面幾乎都有）；擋掉就壞頁面，放行要 `unsafe-hashes` 逐一列出 | 不存在 |
| CDN 函式庫 | guide 允許 cdnjs、jsdelivr、unpkg，這三個主機都要列進 `script-src`，列進去等於允許它們上面的任何套件 | 只有 `/vendor/…` |
| 既有頁面 | 已發佈的頁面會壞 | 還沒有頁面 |
| bridge | 注入的 inline script 本身也要 hash 或 nonce，每次回應不同就不能快取 | 同左，但回應本來就是 server 產生 |
| 回報 | 壞頁面的人看到空白，不知道是 CSP | 編譯器產生的頁面不會壞 |

html kind 的代價是壞掉既有與未來幾乎所有 agent 頁面，換來擋住一類「頁面本身有 XSS」的問題；那一類問題的第一道防線是教 agent 不要用 `innerHTML` 畫共享資料
（由 html kind 的 `Guide()` 負責），不是由平台替每個頁面補。react kind 的輸出是平台產生的，同樣的防護幾乎沒有代價，所以 react kind 加了。

## react kind 現在怎麼做

- react kind 實作 `kind.PageRenderer`：`RenderPage` 回傳頁面和每一段 inline script 的原文（import map、錯誤顯示 script、編譯後的元件）。
  server 的 `artifactCSP` 把它們和 bridge 各算 SHA-256，再加上 kind 宣告的 `Page.ScriptPaths`（react 是 `<origin>/vendor/`，函式庫與 Tailwind 的瀏覽器版），組成 `sandbox …; script-src <origin>/vendor/ 'sha256-…' …; frame-ancestors 'self'`，沒有 `unsafe-inline`、`unsafe-eval`。
  **不含 `'self'`**：`'self'` 是這個 host 上的每一支 script，而 artifact 的檔案現在可以是 `.js`（[multi-file.md](multi-file.md)），第三者把一個 `<script src>` 寫進受害者的頁面，就能指向自己 artifact 的 `evil.js` 並帶著受害者的 bridge 執行；e2e 在 Chrome 驗證它被擋下。`sandbox` 保持原樣，kind 沒辦法放寬它。
- 頁面是 server 產生的，所以 hash 在產生時就知道，編譯結果與 hash 一起記在記憶體快取裡。
- slides kind 同樣實作 `PageRenderer`：頁面只有一段固定的播放器 script（內容是資料、全部跳脫，不會變成 script），所以 `script-src` 只有 bridge 與播放器的 hash。
- doc kind 同樣實作 `PageRenderer`：頁面由平台從跳脫過的區塊產生，自己沒有任何 script，所以 `script-src` 只有 bridge 的 hash（預覽是預覽 stub 的 hash）。將來若有渲染錯誤讓文件的文字變成標記，它也不會執行；唯讀頁、列印（同一頁在 viewer 的隱藏 frame 裡）、舊版本與總覽卡片的預覽都是這個政策。編輯器在 React shell 裡，不經過 `/raw`。
- 不實作 `PageRenderer` 的 kind（html）不帶 `script-src`。

## 不做的事

- 不為 html kind 建立例外或開關。要重新評估，條件是出現第三者字串變成程式碼的實例，或有辦法在不壞頁面的前提下遷移。
