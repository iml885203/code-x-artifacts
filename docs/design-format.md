# design 內容格式 1（manifest 加 frame）

設計依據：#86、#87、`docs/spikes/design-editor.md`（#60，E1：frame 是靜態、無 script 的 HTML，畫在 opaque origin 的 sandbox 裡，編輯器在 shell）。
`design` 的內容格式是 `1`：內容（`content`）是 `design.json`，一份只描述結構的 manifest；每個 frame 是版本的一個檔案 `frames/<名稱>.html`（[multi-file.md](multi-file.md)）。
實作在 `internal/designfmt`（manifest 與 frame 的規則、正規形式、review），`internal/kind/design.go` 與 `design_render.go` 把它接到 `Kind` 介面，frame 怎麼送出見 [multi-file.md](multi-file.md#design-的-frame)。

## design.json

```json
{"version": 1, "pages": [
  {"id": "home", "name": "Home", "rows": [
    [{"id": "hero", "name": "Hero", "file": "frames/hero.html", "w": 1440, "h": 900},
     {"id": "hero-mobile", "name": "Hero mobile", "file": "frames/mobile.html", "w": 390, "h": 844}],
    [{"id": "footer", "name": "Footer", "file": "frames/footer.html", "w": 1440, "h": 240}]]}]}
```

- 欄位只有這些：根的 `version`（必須是 `1`）、`pages` 與可省略的 `tokens`；page 的 `id`、`name`、`rows`；frame 的 `id`、`name`、`file`、`w`、`h`。其他欄位被拒絕（訊息列出有哪些），不是忽略。
- `pages` 至少一個、至多 20 個；每個 page 至少一列（`rows`），每列至少一個 frame；整份設計至多 60 個 frame。列是並排的 frame，總覽也照列畫。
- `id`：page 與 frame 的 `id` 是 1 到 40 個小寫字母、數字、`-`、`_`，開頭是字母或數字。page 的 id 在 pages 之間不重複，frame 的 id 在整份設計不重複。id 是留言與編輯指的名字，改名時保留。
- `name`：人看到的名字，非空、至多 80 字。
- `file`：`frames/<名稱>.html`，一層深，名稱遵守一般檔名規則（`internal/files`）；每個 frame 一個自己的檔案。
- `w`、`h`：整數像素，1 到 8192。
- 大小：`design.json` 至多 256 KB，一個 frame 至多 512 KB。

## 驗證：全部列出、什麼都不存

`Validate`（發佈、還原、複製都經過）一次列出所有問題，每一條以位置開頭；有任何一條就拒絕，什麼都不存（錯誤訊息開頭「this design cannot be saved (N problems), nothing was stored」，最多顯示 25 條）。

```text
Page "Home" frame "Hero" element 3: tag script is not allowed (a frame is static markup and runs no scripts)
Page "Home" frame "Hero" element 7: attribute src: external address "https://x.test/a.png" is not allowed: …
Page "Home" frame "Footer": file frames/footer.html is not among this design's files; send it with files
Page "Home" frame "Again": frame id "h" is already used by Page "Home" frame "H"; a frame id is unique in the design
file notes/readme.html: only frames are files of a design: …
```

`element N` 是作者送的那份檔案裡的第 N 個開始標籤（以 tokenizer 計，不是樹），所以編號對得上作者寫的東西，也看得到 parser 會移動或丟掉的元素。manifest 的位置是 `Page "名稱" frame "名稱"`，沒有名稱時是 `pages[0]`、`rows[1][0]`。

規則（`internal/designfmt`，窮舉的 table test 在 `designfmt_test.go`）：

- **manifest**：上面的欄位、範圍與唯一性；每個 frame 的 `file` 要在這個版本的檔案裡；版本裡所有檔案都必須是 `frames/<名稱>.html`。
- **標籤的 allow-list**：文字與結構（`div span p h1`-`h6 a b i u s em strong small mark sub sup code pre kbd blockquote br hr …`）、清單、表格、`header footer nav main section article aside figure dialog …`、
  看起來的表單元件（`form input button select option textarea label …`）、`img picture source`、inline SVG 的繪圖元素（`svg g path circle rect line text defs lineargradient clippath mask filter 與 fe* …`）、`html head body title meta style`。
  `script`、`noscript`、`template`、`iframe`、`object`、`embed`、`base`、`video`、`audio`、`canvas` 與其他不在清單的標籤被拒絕。
- **屬性**：`on*` 事件屬性、`srcdoc`、`http-equiv` 被拒絕；其他屬性（含任何 `data-*`、`aria-*`）都保留。
- **位址**（`<a>` 的 `href` 可以是 `https:` 或 `mailto:`，見「allow-list 的寬度」）：`href`、`src`、`srcset`、`poster`、`action`、`formaction`、`background`、`data`、`cite`、`xlink:href` 等屬性、CSS 的 `url()`、其他屬性值裡的 `url(`，只接受 `#片段`、`data:image/…`／`data:font/…`、`/files/…`（這個 artifact 上傳的檔案）與 `/static/fonts/…`；其餘（含 `https:`、相對檔名、`java\tscript:`）都是「external address」。
- **CSS**（`<style>` 與 `style` 屬性）：屬性在一份保守的 allow-list（畫、排版、動畫、字型相關的屬性，含 grid、flex、transform、filter、SVG 的 fill 與 stroke），自訂屬性（`--x`，值可含 `{}`）與廠商前綴屬性（`-webkit-`、`-moz-`、`-ms-`、`-o-`，除了 `-moz-binding`）也收；
  值不限（漸層、`calc()`、`var()`、`!important`），at-rule 只收 `@media`、`@supports`、`@keyframes`、`@font-face`、`@layer`、`@container`、`@page`、`@property`、`@charset`，`@import` 與其他被拒絕；
  值裡的 `expression(`、`javascript:`、字串以外的反斜線（用來把 `url(` 拼寫成跳脫序列躲過檢查）被拒絕。
  這份清單的寬度有量測根據，見下一節；frame 的 CSP 本來就不讓它載入任何外部資源，這些規則是第一層，不是唯一一層。

沒有被 `Validate` 拒絕的 CSS 與屬性，**發佈後一個位元組都不會被改**（見下）。

## 正規形式（`Normalize`）

每個 frame 在發佈時、`Validate` 通過之後，經過一次 `Normalize`，存的是它的結果：

1. 以 HTML 的解析器（`golang.org/x/net/html`，scripting 關閉）解成樹，寫回。缺的結尾標籤補上（`<td>` 的 `<tbody>`、`<p>` 的 `</p>`）、屬性值一律加引號、`<!doctype html>` 補成標準模式（沒有 doctype 的片段會變成標準模式）。**這是唯一對 bytes 的改動**：每一個屬性、每一個 style 宣告、每一段 CSS、註解、文字都原樣保留。
2. `<body>` 與它底下的每個元素有 `data-pid`；已經有的、合格的（1 到 40 個字母、數字、`-`、`_`，在這個 frame 內不重複）保留，缺的、重複的、不合格的拿新的 `e<數字>`，數字接在這個 frame 目前最大的 `e<數字>` 後面，所以一個 id 在它的元素還在檔案裡的時候不會被別人拿去用。`html` 與 `head` 沒有 id。
3. 冪等：對正規形式再做一次得到完全一樣的 bytes（不是一次就穩的標記，最多再試三次，仍不穩就以「simplify it」拒絕）。所以 agent `read` 之後原封送回不是一個改動，沒有改的 frame 不會出現在 `rewritten`。

發佈結果的 `rewritten`（HTTP 與 MCP 相同）是這次送的檔案中、存下來的文字與送出的不同的那些：路徑對應存下來的文字，agent 因此在不用再 `read` 的情況下拿到 `data-pid`。沒有改變的檔案不在裡面。

## allow-list 的寬度

清單太窄的代價是 agent 的設計整份被拒絕，太寬的代價是多一個不該有的東西進 frame。寬度由真實 agent 寫的頁面量測，不憑感覺加減（#88）。

**樣本**：64 份內容不同（以內容雜湊去重）的 HTML，全是 agent 寫的，來源是這個部門自己的工作：Claude Code 對話裡做的頁面（11 份）與 stg-game-test 的報告範本（1 份）、Claude Design 匯出的 `*.dc.html`（19 份）、Capykachu 每天產生的 insights 與 dashboard（14 份）、jev-game-poc 的 STG 測試報告（7 份）、lodog 的 case 報告（5 份）、one-for-all 的文件頁（5 份）、archify 的圖（2 份）。
目前還沒有真正的 design frame（design 還在 beta），所以樣本是**整頁**而不是 frame：頁面有 `<script>`、`<link>`、外部網址、嵌了圖片的大檔，這些被拒絕是格式的本意，不是清單的寬度問題。要看的是「清單自己決定的那部分」：標籤、CSS 屬性、at-rule、屬性。
樣本是內部的頁面，不進 repo；量法是對每一份跑 `Validate`（不設每個 frame 12 條的上限）、把每一條問題依種類與名稱歸類，計算有那種問題的檔案數。

| | 檔案 | 說明 |
|---|---|---|
| 樣本 | 64 | 29,815 個開始標籤，其中 187 個在標籤清單外（`script`、`link`、`noscript`、Claude Design 的 `helmet`／`x-dc`／`sc-for`／`sc-if`、`template`） |
| 被拒絕 | 60（94%） | 頁面本來就不是 frame |
| 只因格式本意被拒絕 | 38 | `script`、`link`、外部網址（`href` 42 份、`src` 25 份）、超過 512 KB（6 份，內嵌圖片與字型） |
| 因清單的寬度被拒絕 | 調整前 34 → 調整後 22 | 調整後剩下的 22 份有 19 份是 Claude Design 的框架專用標籤（見下） |

只看 45 份不是 Claude Design 匯出的頁面：因清單寬度被拒絕的從 15 份（33%）降到 3 份（7%），那 3 份是 `@theme`（Tailwind 的建置指令，瀏覽器不認得）、`http-equiv`、`template`。

**加進清單的**（每一個都有頁面用過，或是已在清單裡的家族的另一個成員）：

- CSS 屬性：`scroll-margin-*`（5 份用 `scroll-margin-top`）與 `scroll-padding-*`（2 份）的每個長手寫、`overscroll-behavior` 與它的軸（2 份）、`scrollbar-color`／`scrollbar-width`／`scrollbar-gutter`（2 份）、`break-before`／`break-after`／`break-inside` 與舊名 `page-break-*`（2 份）、`orphans`／`widows`、SVG 的幾何屬性 `x y cx cy r rx ry`（`rx` 2 份）、`grid-gap`／`grid-row-gap`／`grid-column-gap`（`gap` 的舊名，樣本沒有用到，加進來因為 `gap` 本來就在清單裡）。`scroll-snap-stop` 與 `scroll-snap-type` 同組。
- at-rule：`@page`（2 份）、`@property`（1 份）。
- 標籤：`dialog`（2 份；沒有 script 時只在有 `open` 屬性才顯示，是靜態的）。

**維持拒絕的**：`@theme`（不是 CSS）、`template`、`http-equiv`、Claude Design 的 `helmet`／`x-dc`／`sc-for`／`sc-if`（那個框架在瀏覽器裡用 script 展開它們，沒有 script 的 frame 不會畫出任何東西）、`behavior`、`-moz-binding`。**沒有收窄任何東西**：樣本裡沒有證據顯示清單裡有哪個屬性是多餘的。

**外部連結與大小**（Logan 的決定，跟 Claude 的 Docs 與 Slides 一樣讓人加外部連結）：

- `<a href>` 可以是 `https:` 或 `mailto:`；其他標籤與屬性（圖片、字型、CSS `url()`、`action`、`area`…）的外部位址仍然拒絕。發佈時連結被正規化成 `target="_blank"` 與 `rel="noopener noreferrer"`（原本有的 `target`、`rel` 被換掉），所以連結一律在新分頁開、不帶 opener 與 referrer。
  樣本裡這條規則影響很小：64 份只有 1 份有 `https:`／`mailto:` 的 `<a href>`，全部沒有 script、沒有外部圖片的頁面從 4 份增加到 6 份。樣本裡多數被拒絕的 `href` 是 `<link href>`（22 份）與頁面之間的相對連結（`../insights/….html`，20 份），這兩種仍然拒絕。
- 一個 frame 至多 512 KB，不放寬：樣本有 6 份超過（內嵌 base64 的圖片與字型）。超過時的拒絕訊息說要怎麼做：用 `upload_asset`（或把檔案當設計的檔案送）上傳圖片或字型、在 frame 裡用回傳的網址，並指出找到的最大的 `data:` 位址（大小與型別）。

## Review

`Review` 對這次發佈送的 frame（不是整份設計）提醒作者自己看不到的事，放在發佈結果的 `review`（不會拒絕發佈）：

- `img` 沒有 `alt` 屬性（空的 `alt=""` 是合格的裝飾圖）。
- 一個以 inline style 用 `position: absolute|fixed` 與像素 `left`／`top`／`width`／`height` 擺在 frame 外面（看不到）或超出邊緣的元素，以 `data-pid` 指名。
- 文字多半放不進它的框：元素的 inline style 給了像素的 `width` 與 `height`，估計的行數乘行高超過高度（`overflow` 不是 `auto`／`scroll`、沒有 `line-clamp`）；或文字設了 `white-space: nowrap`，估計的一行比它的寬度（沒有寬度時比 frame）寬，而且沒有 `text-overflow: ellipsis`。估計與 slides 的 review 同一套（每個字的寬度依字元種類、CJK 為一個字寬、換行留 7% 餘裕），超過 15% 才提醒。
  只讀 inline style：由 `<style>` 的 class 決定大小的元素不評估；字級不是像素（`em`、`%`、關鍵字）、元素裡有非行內的子元素（子元素各自評估）、`svg` 裡的文字都不評估。訊息說「likely」，不是一定。
- 沒有任何現行瀏覽器會套用的屬性（`-ms-*`、`-o-*`、`-webkit-overflow-scrolling`）：照原樣保留、什麼也不做，訊息以屬性名指名，在 `style` 屬性裡以 `data-pid`、在 `<style>` 裡說「a <style> rule」。`-moz-*` 與其他 `-webkit-*` 不提醒，Firefox 與 Safari 仍然套用其中一些。清單外的屬性不是這一則，是被拒絕、整份不存，訊息已逐一指名。
- frame 裡沒有任何文字。
- 這次送的檔案沒有任何 frame 用到（nothing shows it）：加一個 frame，或送 `null` 刪掉。

每個 frame 至多五則，其餘合併成一行；訊息以 `Page "…" frame "…":` 開頭。

## 總覽頁與畫布

`/raw/{id}`（進入頁面）是 design 的總覽，由平台產生（`PageRenderer`）：每個 page 一個區段，每列的 frame 並排（寬度與 frame 的 `w` 成比例，所以同一列的縮圖是同一個比例），每個 frame 是一個 `<iframe sandbox="allow-scripts">`，`src` 是它的檔案（與總覽同一層的 grant 網址）。每個 frame 立刻載入、不用 `loading="lazy"`：總覽就是瀏覽器列印的頁面，列印時還沒載入的 frame 會印成空白頁（zip 的 `index.html` 同理）。
縮圖只用 CSS 縮放到格子的寬度（格子是 `container-type: inline-size`，縮放是 `scale(tan(atan2(100cqw, <w>px)))`，也就是格子寬除以 frame 寬），頁面沒有任何 script，所以 gallery 卡片裡沒有 `allow-scripts` 的預覽也畫得出縮圖（#49 其餘的預覽問題不在這裡）。名稱經過跳脫。

viewer 開 design 時不用總覽，而是畫布（`web/src/design/`）：shell 向 `GET /api/artifacts/{id}/design[?version=N]` 讀 `{version, manifest, base, live}`（`base` 是那個版本的檔案的 grant 網址，以 `/` 結尾；舊版本只有作者與編輯者，與 `/raw?version=N` 相同；匿名的 public 讀者也能讀，規則同頁面；`live` 只在沒有 `?version`、而且看的人能進 room（登入且看得到）時才有，是 room 現在的 frame 的位址，見 [Design room](design-room.md) 的「畫布上的 frame 從哪裡來」），
照 `layoutPage` 擺：一列的 frame 由左到右、頂端對齊（間隔 96），列由上到下（間隔 168，名字在 frame 上方），四周留 120。每個 frame 是一個 `<iframe sandbox="allow-scripts">`，位置與大小用畫布座標，整個世界以 `translate` 與 `scale` 移動縮放，frame 不接收指標事件（畫布擁有指標）。

- 縮放 25% 到 200%（右側浮動列的 − 與 +、縮放比例的選單、`+`／`-`、`1` 回 100%、Ctrl／Command 加滾輪或雙指縮放，以指標為中心），Fit（`0`，打開與換頁時自動做，不放大過 100%，頁面太大時可以低於 25%）；滾輪平移，拖曳畫布平移（選取工具與手形工具都是），手形工具（`H`，`V` 回選取）與按住 Space 時按下就是平移。
- 浮動工具列在畫布上方：左邊是 Page ▾（換頁；有編輯權限的人還有新增、重新命名、刪除目前頁面，刪除問一次，Undo 帶回來）、選取（`V`）、手形（`H`），有編輯權限的人再加文字（`T`）、Frame（`F`，在畫布上拖出新 frame 的大小，點一下用桌面尺寸，加在目前頁面的新一列）與形狀 ▾（矩形 `R`、橢圓 `O`、圖片）；右邊是簡報模式（播放目前頁面的第一個 frame）、側欄開關（大綱與屬性）與縮放（− 比例 ▾ +，選單有 100% 與 Fit）。只有一頁的設計，沒有編輯權限的人沒有 Page 選單。有編輯權限、看最新版本的人在 Frame 與形狀之間還有畫筆（`P`，見[畫筆](#畫筆)，#137）；便利貼還沒有，不在 beta 範圍（#136）。新版本到來時留在原頁與原位置，換頁才重新 Fit。
- 房間連上之前，大綱的位置就保留著（空白的同寬欄位），所以房間打開時畫布不會往右跳；沒有房間的人（未登入、看舊版本）與關掉側欄的人不保留。
- Play：每個 frame 名字旁的按鈕以全尺寸看一個 frame；Fit 是整個 frame 看得到（不超過真實大小，視窗比較小就縮小），Fill 是寬度等於視窗、往下捲動；Esc 或關閉鈕回到畫布，焦點回到按下的 Play。
- 管理 frame：編輯者可以點畫布上的 frame 名稱或左側 Frames 列選取整個 frame（不選 iframe 內的元素），以 Delete／Backspace 或該列刪除按鈕刪除；Undo 連內容一起還原。點已展開的 Frames 列可收合尺寸控制。每頁至少留一個 frame，最後一個的刪除按鈕停用並顯示原因。換頁或 frame 結構改變會清除選取；讀者與舊版本沒有 frame 修改控制。
- 選取（選取工具，沒有移動的點擊）：畫布把點擊換成 frame 自己的像素座標，問那個 frame 裡的 inspector「這裡是什麼」（`hit`）再問「它是什麼」（`report`），選取框畫在畫布上元素的位置，右邊的 Code 面板唯讀地列出最後選的元素的標籤、`data-pid`、屬性、行內樣式與一組計算後的值（尺寸、邊距、顏色、字型、flex 的幾個屬性）；Shift 加入或移出，Esc 或面板的關閉清掉。選取不改設計，讀者與舊版本一樣能用。inspector 的訊息格式與頁面端的規則見 `web/src/design/AGENTS.md`。
- 就地改字（選取工具，雙擊文字）：frame 裡的 inspector 讓雙擊的元素可編輯，輸入在 frame 內發生；Enter 或失焦提交、Esc 放棄、Shift+Enter 換行、Cmd+B 與 Cmd+I 加粗與斜體，輸入法組字中的 Enter 不提交；編輯結束時元素的屬性逐 byte 還原（Chrome 會改寫 `style`）、焦點還給頁面，所以接著按 Cmd+Z 就能還原。寫進 room 的方式與限制見 [design-room.md](design-room.md)。
- 舊版本（版本紀錄選的）用同一個畫布、讀那一版的 manifest 與檔案；沒有任何編輯控制。讀者與舊版本只有 Page 切換、選取、手形、簡報、側欄與縮放，沒有文字、Frame、形狀、outline 把手與屬性面板，這條由元件測試守著；編輯控制只在 `canEdit` 且在最新版時才出現。

## 畫布上的留言

可留言的人（包含讀者）可以按 frame 名字旁的「留言」，或選取元素後按「在所選元素留言」。畫布上的編號釘子與既有留言面板共用同一份 thread；按釘子打開 thread，按面板的「位於…」會切到它的頁面並顯示位置。釘子的大小不隨縮放改變。回覆與未讀數沿用一般留言規則。

anchor 是 `{"type":"design","frameId":"hero","pid":"e2","name":"Heading","x":120,"y":80}`：frame id 與來源 `data-pid` 指向原元素（frame 留言省略 `pid`），`x`／`y` 是未縮放的 frame 像素座標，範圍 0 到 8192。元素有目前幾何資訊時釘子跟著它；元素被刪或暫時無法定位時不畫釘子，不改 anchor、不刪 thread。只有 room 確認來源身分已不在時，面板才標示原始元素已移除；inspector 未就緒、隱藏或暫無幾何資訊則標示目前無法顯示原始位置。Undo 恢復同一個來源身分後，釘子重新連回。省略 pid 的畫框位置留言仍使用存下的位置（限制在畫框邊界內）。整個 frame 不在時，留言仍留在面板。格式不符的 anchor 不畫釘子，留言本身照常顯示。

`comment` 可以帶同樣的 anchor，`comments` 與 HTTP 都原樣回傳；回覆不帶自己的 anchor。`name` 是可省略的短名稱；anchor 最多 2000 bytes，識別字與名稱保持簡短。

登入且仍有 artifact 存取權的人可以刪除自己的留言：面板先確認，HTTP `DELETE /api/artifacts/{id}/comments/{comment}` 與 MCP `delete_comment` 共用作者檢查。同一作者重複刪除成功；不能刪除別人的內容。刪除會清除持久化 body；有回覆的根留言保留原 id、anchor 與「作者已刪除留言」標記，其他人的回覆不受影響。已刪回覆與沒有剩餘回覆的已刪討論串不列出；計數只包含未刪內容。讀取與 MCP 不回傳已刪 body。

## 匯出

- **zip**（Export 選單的「Zip (.zip)」、`GET /api/artifacts/{id}/export/zip`）：`index.html`（總覽，縮圖指向 `frames/…`，設計成不載入任何 server 的東西、可以從 `file://` 開；Go 測試證明 zip 內每個位址都解析在 zip 內，`e2e/tests/design-export.spec.ts` 有本機 `file://` 的開啟旅程；下載 staging 上真實設計並離線開啟是 [acceptance-beta.md](deployment.md) 的手動驗收步驟，沒有記錄就不是已驗證）、`design.json`、`frames/<名稱>.html`、`assets/<名稱>`（frame 用到的上傳檔）、`fonts/…`（用到的平台字型）。frame 裡的 `/files/<id>/<名稱>` 與 `/static/fonts/…` 改成相對路徑（`../assets/…`、`../fonts/…`）；找不到的檔案（別的 artifact 的、已刪的）換成空的 `data:,`。權限與其他匯出相同。
- **PDF** 是瀏覽器列印總覽頁（`/raw/{id}` 的 `@media print`）：一個 frame 一頁，頁面大小等於 frame，frame 不縮放、文字仍是文字（伺服器沒有 PDF，與 doc 相同）。viewer 的「PDF」項目由 `BrowserFormats` 宣告；畫布沒有可列印的頁面，所以 viewer 在畫面外載入總覽，等它載入後經 bridge 請它列印。列印的是目前看的版本（最新的已存版本或正在預覽的歷史版本），包含所有 page 的 frame，不只畫布目前顯示的 page。列印 frame 的樣式屬於 viewer，不依賴文件編輯器的載入。
- PNG 與 PPTX 不支援。
- 圖庫卡片（`/raw/{id}?preview`）只載入 manifest 順序的第一個 frame（跳過沒有 frame 的頁面），不論設計有幾個 frame；沒有 frame 時是空白預覽。總覽、列印與匯出照舊包含每個 frame；預覽沿用同一個版本 grant、sandbox、CSP 與 `no-store`，不新增快取或截圖。

## 與 agent 的路徑

`kinds` 的 design guide（`internal/kind/design_guide.md`）教 agent 寫這個格式；guide 裡的每個範例都通過 `Validate`（測試掃描每個 fenced block：`json design.json`、`html frames/<名稱>.html`、沒有名稱的 `html`、`css`），範例設計不需要 review、正規化是冪等的。
`publish` 帶 `kind: "design"`、`content`（design.json）與 `files`；`read` 回 design.json 與檔案清單，`read` 的 `file` 回一個 frame 存下來的文字。

## 共用設計 Token

`design.json` 的 `tokens` 是 CSS 自訂屬性到字串的平面物件，例如 `"tokens":{"--accent":"#336699","--space":"16px"}`。名稱包含 `--`，後面 1–80 個 ASCII 英文字母、數字、底線或 dash，以字母或底線開始；值非空且至多 1000 bytes。接受顏色、字型、間距、圓角、陰影、`var()` 別名與算術／顏色函式；拒絕網址、跳脫、註解、控制字元、宣告或規則分隔符、HTML 與 `!important`。未定義或成環的別名由 CSS fallback 規則處理。

frame 保留 `color:var(--accent)` 等原始宣告。平台只在送出的副本注入變數，不把計算值寫回檔案。properties 的顏色選單選 token 寫入 `var(--name)`；Design tokens 區塊改共用值，一次提交是一步 Undo，所有 live frame 同步。agent 讀最新 manifest 後用 `publish` 加 `baseVersion` 改 token，未變的 frame 不必重送。歷史、raw、Play、print/PDF 與 ZIP 都使用對應 manifest 的 token。

`<link rel="stylesheet" href="/kit/v4/pictor.css">` 可用（已發佈 v1–v4）；只接受 kit 路徑與 `rel`、`href`、`media`、`data-pid` 屬性。平台把受信任 kit CSS 放進送出的 frame，維持 inline-only CSP，ZIP 把字型一起收進來；其他 stylesheet link 仍拒絕。

## 畫筆

有編輯權限、看最新版本的人按 P 或選畫筆，在 frame 內拖曳畫線；點一下是圓點。顏色與寬度在畫筆選單設定，寬度為 0.5–64 個 frame 像素。取消、frame 外起筆與非法座標不寫 room。每個手勢是自己的單一 transaction，所以一步 Undo 會移除整筆，不影響別人的筆畫。

筆畫是帶穩定 `data-pid` 的普通 SVG 外框與 path：外框的 `left`、`top`、`width`、`height` 負責移動與調整尺寸，`viewBox` 保留向量座標；path 的 `d`、`stroke`、`stroke-width` 經 room、存檔與重新載入保留。顏色變數如 `var(--accent)` 保持參照，不展開為計算值。刪除、複製與自己的 Undo 沿用一般元素規則。

Agent 以同一份 `read`／`publish` 讀寫 SVG。路徑的有限數值、大小與命令驗證和完整範例見 [Design Guide](../internal/kind/design_guide.md#pen-strokes)，發佈與 room 共用驗證。沒有壓力感應、橡皮擦、筆刷種類或形狀辨識。
