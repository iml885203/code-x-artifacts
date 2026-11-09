# doc 匯出成 Word（`internal/docx`）

設計依據：#85。Export 選單的「Word (.docx)」，路由與 Markdown 相同：`GET /api/artifacts/{id}/export/docx`。

## 做法

`docx.Write(doc, title, images)` 用 `archive/zip` 與字串組出 Office Open XML，沒有第三方函式庫、沒有外部服務（所以沒有授權要查）。
零件：`[Content_Types].xml`、`_rels/.rels`、`docProps/core.xml`／`app.xml`、`word/document.xml`、`styles.xml`、`numbering.xml`、`_rels/document.xml.rels`、`word/media/*`。
zip 內的時間固定，同一份 doc 輸出同樣的位元組。

| doc | Word |
|---|---|
| 分頁 | 多個分頁時每頁一個 Title 段落（第二頁起換頁）；單一分頁沒有 |
| heading 1 到 6 | `Heading1` 到 `Heading6` 樣式 |
| 段落與 mark | bold、italic、underline、strike、highlight（黃）、code（等寬、底色）；換行是 `<w:br/>` |
| 連結 | `https:`、`http:`、`mailto:` 是外部超連結；`#片段` 在檔案裡沒有目標，只留文字 |
| bullet、numbered 清單 | `numbering.xml`；每個編號清單有自己的計數，`start` 用 `startOverride`；巢狀 9 層 |
| task list | 項目前面 ☑ 或 ☐ |
| quote | `Quote` 樣式（左邊線、縮排），巢狀加深縮排 |
| code | 每行一個 `CodeBlock` 段落（等寬、底色） |
| rule | 下邊線的段落 |
| table | `w:tbl`，欄寬平均、`tblGrid` 與每列的格數和 doc 相同；表頭列（`header`）粗體、灰底、`tblHeader`；對齊是 `jc` |
| image | 見下 |
| pending | 灰色虛線框，「To be written: 意圖」 |

文字中的字元照原樣進檔案（CJK、emoji 都是 UTF-8 的 `w:t`）；不能放進 XML 的字元由 `encoding/xml` 處理。東亞字型指定 Microsoft JhengHei，沒有時 Word 與 LibreOffice 會替換。

## 圖片

- 上傳到這份文件的檔案（`/files/{id}/{名稱}`，相對或完整位址）以檢視者的身分讀出並嵌入（`FileExporter`，單張上限 20 MB）；`data:image` 位址直接解碼嵌入；同一個來源只存一份。
- 嵌入的是 PNG、JPEG、GIF（別的格式，例如 WebP，或讀不出尺寸的）；寬度超過文字寬度時等比縮小。
- **別的伺服器上的圖片不會被抓**（匯出不連外，檔案也就不依賴那台伺服器）：寫成 `[image: 說明]`，`https:` 位址是連到它的超連結。
- 嵌入的圖片以 alt 作為 Word 的替代文字。

## 驗證

- `internal/docx/docx_test.go`：zip 與必要零件、每個 XML 零件良構、每個 relationship 指向檔案裡的零件；用標準 `encoding/xml` 讀回：標題、表格的列與欄、連結位址、清單的編號實例、圖片的位元組、CJK 與 emoji 的文字、同一份 doc 同樣的位元組。
- LibreOffice：有 `soffice` 的機器上 `TestLibreOfficeConvertsTheFileToAPDFWithTheText` 把檔案轉成 PDF；沒有就跳過（CI 目前沒有）。
- **手動步驟**（stg 檢查）：匯出一份有標題、清單、表格、圖片、連結與中文的 doc，用 Word 開啟：標題樣式在導覽窗格裡、表格的欄列正確、圖片在、連結可點、清單編號正確；沒有「修復檔案」的提示。
- e2e：`e2e/tests/export.spec.ts`，Export 選單的 Word 下載一個以 `PK` 開頭、含 `word/document.xml` 等零件的檔案。

## 不做

儲存格顏色、Word 匯入 CodeXArtifacts；agent 沒有這條路（它用 `read`）。
