# internal/docx

doc 的 Word 匯出：區塊轉 Office Open XML（zip、樣式、編號、表格、圖片、連結）。純函式，圖片由呼叫者給；見 `docs/doc-docx.md`。

## 不變式

- **doc 可匯出成 Word（`export/docx`）：Go 手寫的 OOXML（沒有第三方函式庫與外部服務），標題、段落與 mark、清單（每個編號清單自己計數）、表格（列與欄與 doc 相同）、連結、code、quote 都對應；上傳到這份文件的圖片與 `data:image` 嵌入檔案，別的伺服器的圖片不會被抓、寫成連結；同一份 doc 輸出同樣的位元組。**
  Tests: `TestTheOutputIsAValidZipWithTheRequiredPartsAndEveryRelationshipPointsAtAPart`, `TestHeadingsReadBackFromTheXMLWithTheirLevels`, `TestTablesKeepTheirRowsAndColumns`, `TestLinksAreExternalWithTheRightAddresses`, `TestListsHaveTheirOwnNumberingAndTheTextOfEveryBlockIsThere`, `TestEveryCharacterOfACJKAndEmojiDocIsInTheFile`, `TestPicturesAreEmbeddedFromDataAddressesAndFilesAndOthersAreLinks`, `TestAnEmptyDocAndABlocklessTabStillMakeAFileWordCanOpen`, `TestLibreOfficeConvertsTheFileToAPDFWithTheText`（有 soffice 才跑）, `TestAWordExportEmbedsThePicturesUploadedToTheDocAndTheDownloadIsNamedAfterTheTitle`；e2e export.spec.ts。（docs/doc-docx.md）

- **日期欄位匯出為目前日期的普通文字，不帶 Word 互動表單控制項。**
  Tests: `TestWordExportsFieldValuesAsTextWhereverRunsAreAllowed`.
