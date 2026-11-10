# internal/docfmt

`doc` 格式 2（區塊 JSON）：模型、驗證、正規化（補 id）、唯讀 renderer、Markdown 與區塊互轉（`docs/doc-format.md`）；`internal/kind/doc.go` 把它接到 `Kind`。

## 不變式

- **格式 2 是帶穩定 id 的區塊 JSON，和 Markdown 一樣是資料：每個區塊類型與 mark 都過驗證、每個不合格的內容以路徑開頭的訊息拒絕且什麼都不存（限制的訊息說出哪個限制和現在的值）；** 連結只接受 `https:`、`http:`、`mailto:`、`#片段`，圖片 `src` 只接受 `https:`、本文件的檔案、小的 `data:image`；renderer 對每個字串跳脫、沒有原始 HTML；HTTP 與 MCP 的 `blocks` 發佈結果與拒絕相同；沒帶 id 的區塊由 server 補上、已帶的保留，存下來的是正規形式。
  Tests: `TestEveryBlockTypeAndMarkValidates`, `TestEveryInvalidDocIsRefusedWithThePathOfTheProblem`, `TestTheLimitsNameWhichLimitAndTheCurrentValue`, `TestADocAtEveryLimitStillValidates`, `TestTheListItemLimitNamesTheCount`, `TestRenderDrawsEveryBlockTypeAsReadingMarkup`, `TestRenderEscapesEveryStringSoScriptsAndHandlersStayInert`, `TestRenderNeverEmitsAnAddressThatCouldRunScriptEvenFromUnvalidatedData`, `TestNormalizeGivesMissingIdsAndKeepsTheOnesSent`, `TestNormalizeIsCanonicalSoEqualDocsAreEqualBytes`, `TestADocOfFormat2IsValidatedAndRenderedAsBlocks`, `TestPublishingBlocksOverHTTPAndMCPGivesTheSameResultAndTheSameRefusal`, `TestABadBlockIsRefusedTheSameOverHTTPAndMCPAndNothingIsStored`, `TestBlocksAreRefusedWhereTheyDoNotBelong`, `TestABlockDocShowsInertWhateverItsStringsContainInsideTheSandbox`。圖片的 `size` 只有 `small`、`medium`、`large`、`xlarge`（renderer 畫成 224、336、504、672 的 `width`，Markdown 寫在圖片的標題位置，如 ![alt](url "large")）；上傳檔已被刪掉的圖片在頁面上畫佔位框（`MissingPictures`，由 server 傳入現有的檔案）。Tests: `TestAnImageHasOneOfFourSizesOrNone`, `TestAnImageSizeSurvivesMarkdownAsATitleAndOtherTitlesAreIgnored`, `TestAPictureWhoseFileWasDeletedIsAPlaceholderAndOthersAreLeftAlone`。

- **Markdown 是 agent 寫 doc 的方式，也是讀回與匯出的格式：** 發佈的 Markdown 轉成區塊（`<!-- pictor:pending 意圖 -->` 一行是灰色佔位的 `pending` 區塊、不是錯誤），`read`、版本 API、衝突時的 `current` 與 `export/md` 把格式 2 轉回 Markdown；Markdown 轉區塊轉 Markdown 轉區塊得到相同的區塊，表格的對齊與表頭保留；重新發佈時內容沒變的區塊沿用 id；不合格的連結或圖片位址使整個發佈失敗並說出行號，HTTP 與 MCP 相同；原始 HTML 不進區塊。
  Tests: `TestMarkdownToBlocksKnowsEveryConstruct`, `TestMarkdownBlocksMarkdownBlocksIsTheSameBlocks`, `TestATableKeepsItsAlignmentAndHeaderRowThroughTheRoundTrip`, `TestRepublishingWithOneParagraphChangedKeepsEveryOtherBlocksId`, `TestAPendingLineIsAPlaceholderThatStoresItsIntent`, `TestAnUnsafeLinkOrImageRefusesTheWholeConversionAndNamesTheLine`, `TestLinksOfEveryAllowedKindConvert`, `TestRawHTMLIsLeftOutAndNeverReachesTheBlocks`, `TestTabsBecomeMarkerLinesAndBack`, `TestWhatMarkdownCannotHoldDegradesWithANoteAtTheEnd`, `TestTextThatLooksLikeMarkdownComesBackAsTheSameText`, `TestDocComposeConvertsMarkdownAndKeepsTheIdsOfBlocksThatDidNotChange`, `TestAnAgentPublishesAMarkdownSkeletonAndPeopleSeePlaceholdersAndReadsItBackAsMarkdown`, `TestRepublishingMarkdownKeepsTheIdsOfTheBlocksThatDidNotChange`, `TestAMarkdownLinkThatCouldRunScriptRefusesThePublishTheSameOverHTTPAndMCPAndNamesTheLine`, `TestAFormat2DocDownloadsAsTheMarkdownOfItsBlocks`, `TestAStaleUpdateOfAFormat2DocHandsBackTheNewestAsMarkdown`.

- **Mention 是只有 `user`、`name` 的行內 run，沒有 mark 或地址；Markdown 的 `mention:` 連結保留它，renderer 只畫跳脫的 `@name`。**
  Tests: `TestAMentionIsValidWhereverTextIs`, `TestAMentionThatBreaksTheRulesIsRefusedWithThePathOfTheProblem`, `TestAMentionSurvivesMarkdownInEveryPlaceItCanAppear`, `TestRenderShowsAMentionAsItsNameWithAnAtAndNoMarkupFromTheName`.

- **Date 是只有 `date` 的行內 run，值為有效的 YYYY-MM-DD（0001–9999）；不與其他 run 欄位混用。靜態頁面、Markdown 與 Word 只輸出日期值，blocks 保留可編輯欄位。**
  Tests: `TestADateIsAValidCalendarDayAndHasNoOtherRunFields`, `TestADateExportsOnlyItsVisibleValueAndContributesToTheTitle`, `TestFieldsAreEditedAndRefusedTheSameOverHTTPAndMCPAndSurviveSQLiteReopening`, `TestWordExportsFieldValuesAsTextWhereverRunsAreAllowed`.

- **Mention accounts accept the original directory-account alphabet or the exact assigned `oidc:[0-9a-f]{32}` namespace. Markdown and blocks retain the same identity; arbitrary colon identifiers remain refused.**
  Tests: `TestAssignedOIDCAccountsRemainMentionsAcrossBlocksAndMarkdown`, `TestOtherColonIdentifiersCannotBecomeMentions`.
