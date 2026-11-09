# internal/pptx

slides 的 PowerPoint 匯出：`Deck`（投影片、元素、run）轉 PresentationML（zip、master、版面、主題、投影片、備忘稿）。純函式，不知道 deck 的 JSON，由 `internal/kind` 轉成 `Deck`；見 `docs/slides-pptx.md`。

## 不變式

- **deck 可匯出成 PowerPoint（`export/pptx`）：Go 手寫的 PresentationML（沒有第三方函式庫與外部服務）；每個零件都有 content type、每個 relationship 都指向檔案裡的零件、`presentation.xml` 依序列出每張投影片；每種元素都是原生的文字框、形狀或表格（沒有圖片，文字可以編輯），粗體與 code 是 run 的屬性；講者備註是自己那張投影片的備忘稿；每個形狀都在投影片內；字串只經過 `esc` 成為 `a:t` 的文字；同一份 deck 輸出同樣的位元組。**
  Tests: `TestTheFileIsAValidPackageWhereEveryPartIsTypedAndEveryRelationshipResolves`, `TestEverySlideIsInThePresentationInOrderAndNoSlidePartIsLeftOut`, `TestEveryElementTypeIsNativeTextShapesOrATable`, `TestBoldAndCodeAreRunPropertiesAndTheMarkersAreGone`, `TestSpeakerNotesAreTheNotesSlideOfTheirSlideAndASlideWithoutNotesHasNone`, `TestEveryCharacterOfACJKAndEmojiDeckIsInTheFileAndCharactersXMLCannotHoldAreReplaced`, `TestEveryShapeIsOnTheSlide`, `TestTheSameDeckIsTheSameBytesAndAnEmptyDeckIsStillAFile`, `TestLibreOfficeConvertsTheDeckToAPDF`（有 soffice 才跑）；kind：`TestADeckExportsAsAPowerPointFileWithEveryElementAsTextAndItsNotes`, `TestADeckWithABrokenSlideExportsTheRestAndSaysWhichSlideCannotBeShown`, `TestEveryStringOfADeckIsTextInThePowerPointFileAndNothingBecomesMarkup`, `TestSlidesExportOnlyAsPowerPointAndRefuseAnythingElse`；web：`TestAnAgentPublishesADeckAndDownloadsItAsAPowerPointFileNamedAfterItsTitle`。（docs/slides-pptx.md）
