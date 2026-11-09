# Artifact 的種類（kind）

`kind` 決定內容如何驗證、如何在 sandbox 畫出來，以及有哪些匯出。分享、編輯權、版本、留言、資產與共享資料仍由共用 service 處理。
網站把 `html` 與 `react` 都標為「網頁」；Docs、Slides、Design 各有種類圖示；Board 也有。種類不等於 beta 驗收範圍，見[文件索引](README.md#範圍與限制)。

## 目前的內容契約

| kind | 儲存與來源 | 人與 agent 的操作 | 匯出 |
|---|---|---|---|
| `html` | HTML 主內容與可選的版本檔案，預設種類 | 發佈完整頁面／多檔 bundle；頁面可經 bridge 讀寫共享資料 | 單檔 HTML；瀏覽器列印 PDF（頁面自己的 `@media print`）；多檔版本須逐檔讀取或複製，不能只下載入口 |
| `react` | TSX 主元件與可選的相對 import／樣式／資料檔；runtime 格式 `v1` | 發佈時以 esbuild 編譯，套件來自釘版內嵌 runtime；不是任意專案的依賴安裝或 build | 瀏覽器列印 PDF（頁面自己的 `@media print`）；可讀取來源／檔案或複製 artifact |
| `doc` | 格式 `2` 的分頁與穩定 id 區塊 JSON；Markdown 是寫入、讀回與匯出形式 | 人直接在 ProseMirror 編輯，agent 用 `publish` 或 `doc_edit`；兩者讀寫同一份 room | Markdown、Word；瀏覽器列印 PDF |
| `design` | 格式 `1` 的 `design.json` 與 `frames/*.html`，每個 frame 有穩定 id 與元素 `data-pid`，共用具名 token | 人在 sandbox frame 的外層畫布選取／改字／移動／調整屬性；agent 用版本檔案發佈與讀回，live 時合併進 room | 離線 ZIP；瀏覽器列印 PDF |
| `board` | 格式 `1` 的 JSON `{version, elements}`，元素是 Excalidraw 的元素；存成正規形式（不含已刪除的元素、依繪製順序、標籤依框排好），見 [boardfmt](../internal/boardfmt/AGENTS.md) | 用來畫想法（流程、架構、討論草圖）；可以從「New」開一張空白的；登入的人在 viewer 裡用 Excalidraw 一起畫（看得到彼此的游標，讀者只能看），agent 用 `publish`（可以只寫骨架：框的 `label`、箭頭的 `from`／`to`，server 展開成綁好的元素）、`board_edit`（只改幾個元素，以 `ifHash` 保護）與 `read`；圖片是上傳到白板的檔案（人貼上或拖進來，agent 用 `upload_asset` 再加 `image` 元素）；兩者讀寫同一份 room。舊版本、完整頁面、未登入的讀者與圖庫卡片看到 server 畫的 SVG（`/raw`，線條整齊、不是手繪風格，沒有 script） | SVG（server 畫的，agent 用 `export`）；開著白板的人匯出的 SVG 與 PNG 是編輯器自己畫的（手繪風格；SVG 只寫字型名稱，不帶字型檔）；沒開編輯器時 PNG 由瀏覽器從 server 的 SVG 轉成 |
| `slides` | 格式 `1` 的結構化 JSON（版面、元素與備忘稿） | 平台 renderer 與播放模式；有權限的人在 viewer 的縮圖列新增、複製、拖曳排序、刪除投影片，在舞台上就地改投影片上的字（標題、文字、清單、表格、卡片等）、從左上的工具插入文字、清單、表格、卡片或大數字，可復原與重做（同一組 slide 路由，改字與插入是 `replace_slide`）；agent 用 `publish` 或一次改一頁的 `get_slide`／`replace_slide`／`insert_slide`／`move_slide`／`delete_slide`；沒有 Docs／Design 的 live 編輯器 | 瀏覽器列印 PDF（一張投影片一頁、16:9、依順序） |

Design token 的變數語法、允許值、live 傳播與離線 ZIP／列印行為見 [Design 格式](design-format.md) 與 [Design room](design-room.md)。

格式與限制分別見 [Docs](doc-format.md)、[Design](design-format.md)、[多檔](multi-file.md)、[runtime](runtime.md) 與 [Slides 格式](artifact-types.md)。
Dashboard、Motion 與 Design System 的預留 UI 不代表有對應的可發佈 kind；server 拒絕未知種類。

## 程式、資料與信任邊界

- 通用網頁是作者的程式，執行在 `/raw/{id}` 的 opaque origin sandbox。React 發佈會驗證 import 與編譯結果；它依賴的 runtime 隨 binary 內嵌並按版本固定。
- Docs 是平台驗證與呈現的資料；renderer 跳脫文字、不輸出原始 HTML。能開啟 live doc 的登入者讀同一個 room；編輯者才有寫入能力，舊版本與匿名公開讀者看 sandbox 閱讀頁面。
- Design 保留 frame HTML 的可編輯原文及不屬於編輯器的位元組。frame 沒有 artifact service bridge，也不執行作者的 script；只有平台釘住 hash 的 inspector，且只回應自己的 parent。host 解析 frame，不把 frame HTML 塞入站台的 live DOM。它與 Docs 都使用 Yjs room，但各自有 codec 與 agent 合併規則。
- 靜態 Design frame 不提供 template repeats、props／logic 或可執行作者 runtime；這是 #147 追蹤的另外一份來源契約，不可用「HTML frame」推斷已支援。
- Board 是資料：元素種類與欄位都是 allow-list，連結只能是 `https:` 或 `mailto:`，任何欄位都不能有 `data:` 位址；發佈與 room 的每個更新用同一套規則（`internal/boardfmt`）。
- 身分、權限與共享資料不放進 kind。作者可指定編輯者，或在組織／公開受眾下開啟組織編輯；提及人名或留言不授予文件存取權。

## Live 內容與版本

Docs、Design 與 Board 的即時內容在 room，接受的 update 先進 SQLite log 才廣播。room 安靜兩分鐘後，snapshotter 驗證並產生不可覆寫的新版本；未改變的快照不多存一版。
還原先保護尚未切版的 room 內容，再把舊版內容寫回 room，存成新的還原版本。Docs 的整份 publish 與 Design 的檔案合併各有保護與衝突規則。
詳見 [同步層](realtime.md)、[Docs room](doc-room.md)、[Design room](design-room.md)；「Saved」是更新已持久化，不是每次輸入都新增一版。

## Agent 與實作入口

MCP `kinds` 回傳 server 實際註冊種類的 Guide；agent 先讀 Guide，再用共用 `publish`、`read`、`versions`、`restore`、`share` 與資產工具。Docs 的局部變更走 `doc_edit`，可用 `read(view: "blocks")` 取得區塊與 hash。Slides 的單頁變更走 slide 工具，以頁碼指頁、以 `baseVersion` 防止頁碼在別人改過之後指錯頁。
HTTP 與 MCP 共用 service 的驗證和權限。大小／路徑／格式不合法會在儲存前拒絕，錯誤指出問題與修正方式；不要透過 renderer 才發現發佈內容無效。

`internal/kind` 的基本介面是 `Kind`（`Validate`、`Render`、`Guide`）；可選介面提供格式轉換、檔案、frame、review、匯出與瀏覽器列印。新增種類的方法與相容性規則在 [adding-a-kind.md](adding-a-kind.md)。
