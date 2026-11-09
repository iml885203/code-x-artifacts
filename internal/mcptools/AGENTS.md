# internal/mcptools

`/api/mcp`：MCP 工具（官方 go-sdk，streamable HTTP，stateless）、工具說明、delete 的 confirm token。只翻譯，不做任何權限判斷。
每個功能一個檔案，工具說明與輸入型別放在該工具旁邊；`mcptools.go` 的 `server` 依序呼叫各檔的 `registerX`。`server` 在第一個請求時才跑、只跑一次、所有請求共用（註冊工具要做 schema 反射，很貴；`New` 不跑它，因為多數 web server——每個測試都起一個——根本不會被問到工具）；工具從請求的 context 取呼叫者（`t.caller(ctx)`），不可把 Caller 存進 `tools`。

## 不變式

- **HTTP API 與 MCP 工具是同一個 service 層（`internal/service`）的兩個入口：權限、驗證、kind 檢查只在那裡，兩邊對同一個 viewer 給同樣的結果和同樣的拒絕。** `/api/mcp` 只認個人 token（`Authorization: Bearer`），不認 cookie，匿名一律 401；MCP 是 stateless，沒有 session。`delete` 要問兩次：第一次只回計畫和 confirm token，第二次帶 token 才刪；token 綁呼叫者、工具和參數的 hash，只能用一次、十分鐘內有效，存在記憶體（重啟就作廢）。每個工具和每個參數都要有給 agent 看的說明；`kinds` 回傳每個 kind 的撰寫指南（`Kind.Guide()`）。
  Tests: `TestAnAgentPublishesUpdatesSharesAndListsThroughTheMCPProtocol`, `TestAnAgentReadsAndWritesTheSharedDataThroughTheMCPProtocol`, `TestTheEndpointNeedsAPersonalTokenAndIgnoresTheBrowserSession`, `TestRefusalsTellTheAgentWhatWasWrongAndWhatToSendInstead`, `TestAToolAndItsHTTPCounterpartGiveTheSameResult`, `TestDeleteWithoutAValidConfirmTokenDeletesNothing`, `TestEveryToolAndArgumentIsDescribedForTheAgent`, `TestAConfirmTokenWorksOnceForTheSameCallerToolAndArguments`, `TestAConfirmTokenExpires`, `TestEveryKindTeachesItsAuthoringRules`, `TestTheConnectPageHandsTheAppThisServersAddressAndTheTextOfTheSetupInBothLanguages`, `TestTwoCallersThroughOneEndpointOnlyEverSeeTheirOwn`, `TestARequestDoesNotRegisterTheToolsAgain`。

- **每個功能都有 agent 的路徑：** 人能在介面上做的事，agent 也能用 MCP 工具做（`docs/goal.md` 原則 5）。這條由 issue 的 **Agent path** 欄位守住，沒有 agent path 就不算完成。

- **讀 live frame 與 publish 使用同一個 agent 身分，讓共用 service 以該 client 讀到的原始碼當三方合併基底；已讀到的人類編輯不能再當成 agent 新增的文字套一次。** 同帳號不同已識別 client 的基底分開；真正重疊的 stale 修改拒絕且 room 不變。HTTP 的讀檔與發佈走相同 service 規則，不改 rich text、未修改的來源位元組或刻意重複的文字。
  Tests: `TestAnMCPFrameReadAndPublishExtendHumanTextExactlyOnce`, `TestLiveFrameReadPublishKeepsRichTextRepeatedWordsAndLaterIndependentEditsOverHTTPAndMCP`, `TestDifferentMCPClientsKeepTheirOwnFrameReadBaseAndAStaleOverlappingEditIsRefused`, `TestHTTPFramePublishRefusesStaleOverlappingWordsWithoutChangingTheRoom`.

- **MCP `read(version)` 的 Doc 內容與 blocks 是該份儲存的版本，包含顯式指定的最新版；只有沒指定 version 才讀 live room。** 舊版讀取維持 service 的 history 權限與拒絕，顯式指定最新版維持看得到 artifact 就能讀的規則，保護還原前的版本不能被還原後的 live 內容冒充；歷史 Design 檔案與省略版本的 live frame 路徑保持原有規則。
  Tests: `TestExplicitMCPDocVersionsKeepSavedBodyAndBlocksBeforeAndAfterProtectiveRestore`, `TestMCPExplicitVersionReadsUseTheSameHistoryPermissionsAndRefusalsAsHTTP`, `TestMCPHistoricalDesignFilesStaySavedWhileOmittedVersionReadsTheLiveFrame`.

- **MCP `export` 與 `GET /api/artifacts/{id}/export/{format}` 呼叫 web 層同一個匯出函式（`Config.Export`），對同一個 viewer 給同樣的檔案（名稱、型別、位元組）與同樣的拒絕；`version`／`?version=N` 經 `Service.ArtifactAt`：省略或最新版給看得到的人，舊版本守 history 的規則（讀者 403，看不到的與不存在的相同）。** 檔案不超過 `upload_asset` 的上限（`Config.MaxUploadBytes`，預設 5 MB；base64 兩個方向都經過 agent 的 context）才放進回應（`text` 或 `base64`）；超過就只回 name、contentType、size、version、`url` 與要它帶 token 下載 `url` 的 `note`，從不給截斷的檔案。HTTP 路由不受這個上限影響。
  Tests: `TestTheExportToolDownloadsTheSameFileAsTheExportRouteAndRefusesTheSameViewers`, `TestTheExportToolHandsOverAFileOverTheMCPLimitOnlyAsItsURLAndOneUnderItInline`, `TestExportingASavedVersionDownloadsThatVersionAndOnlyThoseWhoReadHistoryGetAnOlderOne`, `TestADesignsOlderVersionExportsTheFramesItHadThen` (internal/web).

- **`usage_summary` 與 `GET /api/usage` 對同一個 viewer 給同樣的 JSON 與同樣的拒絕；權限在 `Viewer.CanReadUsage`，工具只翻譯。**
  Tests: `TestAToolAndItsHTTPCounterpartGiveTheSameResult`, `TestOnlyTheNamedUsageReadersMayReadTheSummaryAndBadQueriesSayWhatToSend` (internal/web).

- **`agent_activity` reads the same private bounded history as HTTP and never changes its own usage summary; client provenance is taken from parsed protocol metadata on each request, not an account/token cache.**
  Tests: `TestAgentActivityKeepsPerRequestClientsAndDoesNotInventStatelessIdentity`, `TestAgentActivityReadsAgreeAndHideTargetsAfterAccessLossOrDeletion` (internal/web).

- **Self-reported client display names accept arbitrary nonempty bounded labels, preserve canonical known prefixes and never establish authority; activity, presence and version attribution agree. Empty names remain unknown without a version.**
  Tests: `TestClientNamesAreRecognisedByTheStartOfWhatTheClientCallsItself`, `TestAgentActivityKeepsPerRequestClientsAndDoesNotInventStatelessIdentity`, `TestTheClientNamedOnAnMCPRequestIsTheNameOfTheAgentInTheRoom` (internal/web).

Own-comment deletion requires artifact access and author ownership; it clears the durable body, preserves roots with surviving replies, never cascades, and an authorized repeat succeeds. Tests: `TestOwnCommentDeletionKeepsOtherRepliesAndScrubsTheBodyAcrossHTTPMCPAndRestart`, `TestCommentDeletionRefusesOtherAuthorsAnonymousAndInvisibleArtifactsThroughHTTPAndMCP`.
