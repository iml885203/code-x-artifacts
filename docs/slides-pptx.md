# slides 匯出成 PowerPoint（`internal/pptx`）

設計依據：#20。Export 選單的「PowerPoint (.pptx)」，路由與其他匯出相同：`GET /api/artifacts/{id}/export/pptx`。
權限與其他匯出相同（看不到的與不存在的一樣回 404）；agent 用 MCP `export`（或同一條路由加 token）下載，`kinds` 的 `exportFormats` 列出 `pptx`。

## 做法

`kind.Slides.Export` 把版本 1 的 deck 解析成 `pptx.Deck`（每個元素一個型別，`**bold**` 與 `` `code` `` 變成 run 的粗體與等寬字），
`pptx.Write(deck)` 用 `archive/zip` 與字串組出 PresentationML，沒有第三方函式庫、沒有外部服務。
零件：`[Content_Types].xml`、`_rels/.rels`、`docProps/core.xml`／`app.xml`、`ppt/presentation.xml`、`presProps.xml`、`viewProps.xml`、`tableStyles.xml`、
`theme/theme1.xml`（投影片）與 `theme2.xml`（備忘稿）、一個 slide master、一個「Title Only」版面、notes master、`slides/slideN.xml`、有講者備註的投影片才有的 `notesSlides/notesSlideN.xml`。
zip 內的時間固定，同一份 deck 輸出同樣的位元組。

投影片是 16:9（12192000 × 6858000 EMU）：瀏覽器的 1920 × 1080 畫布一個像素等於 6350 EMU（半點），字級與間距照 `slides.css` 換算。
每張投影片先放標題，元素由上往下排；字級和瀏覽器一樣在 0.75 到 1.2 倍之間取放得下的最大值；估計放不下時所有元素等比縮短，
沒有形狀超出投影片，文字框帶 `normAutofit`。配色是 kit 的淺色主題，字型 Calibri、東亞字型 Microsoft JhengHei、等寬字 Consolas，沒有時 PowerPoint 會替換。

| slides | PowerPoint |
|---|---|
| 標題 | 標題 placeholder（大綱與協助工具看得到）；`title` 版面置中、字比較大，元素在它下面，整組垂直置中 |
| `text` | 文字框；`\n` 是換行 |
| `bullets`、`numbered` | 一個文字框，每個項目一個段落：項目符號 `•`、自動編號 `1.` |
| `code` | 有框線的矩形，每行一個等寬段落；`language` 是右上角的小字 |
| `table` | 原生表格（`a:tbl`）：欄寬平均、表頭列粗體灰字、`tone` 是儲存格的字色與底色 |
| `stats` | 並排的圓角矩形：大數字加說明 |
| `quote` | 左邊的色條加斜體文字，`by` 是「— 名字」 |
| `steps` | 並排：編號方塊加標題與說明的文字框 |
| `columns` | 欄裡的元素照同樣規則排在各欄的寬度裡 |
| `cards` | 並排的圓角矩形：標題、說明、項目清單；`highlight` 換框線色與底色 |
| `progress` | 每條：標籤與數值兩個文字框、軌道與長度等於數值的填色條 |
| `notes` | 那張投影片的講者備註（備忘稿），一行一個段落 |
| 解析不了的投影片 | 標題「Slide N cannot be shown」，問題列在等寬文字裡，其他投影片照常 |

所有字串都是 `a:t` 裡的文字，經 `encoding/xml` 跳脫；不能放進 XML 的字元換成 U+FFFD。
slides 版本 1 沒有圖片元素，所以匯出沒有圖片、也不抓任何東西。

## 驗證

- `internal/pptx/pptx_test.go`：每個零件有 content type、每個 XML 零件良構、每個 relationship 指向檔案裡的零件、`presentation.xml` 依序列出每一張投影片且沒有多的投影片零件；
  用標準 `encoding/xml` 讀回：每種元素的文字在 `a:t` 裡、清單的項目符號與編號、表格的列與欄、粗體與等寬是 run 的屬性、講者備註在自己投影片的備忘稿、CJK 與 emoji、每個形狀都在投影片內、同一份 deck 同樣的位元組。
- `internal/kind/slides_export_test.go`：每種元素都變成文字、壞掉的一張投影片不拖垮其他、惡意字串全部是文字。
- LibreOffice：有 `soffice` 的機器上 `TestLibreOfficeConvertsTheDeckToAPDF` 把檔案轉成 PDF；沒有就跳過（CI 目前沒有）。
- **手動步驟**：匯出一份每種元素都有、含中文與講者備註的 deck，用 PowerPoint 開啟：沒有「修復」提示、每張投影片的文字可以直接編輯、表格是表格、備忘稿有講者備註。

## 不做

像素級相同的版面、PowerPoint 匯入 CodeXArtifacts、動畫與轉場。
