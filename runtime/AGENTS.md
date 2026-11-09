# runtime（頁面用的函式庫）

頁面用的函式庫（React、Recharts、lucide-react、CodeXArtifacts 的元件、Tailwind 瀏覽器版）的建置來源：`package-lock.json`、`build.mjs`、`src/ui/`。Node 只在建置時用；`make runtime` 建出 `internal/jsruntime/v1/`（提交進 git），見 `docs/runtime.md`。

## 不變式

- **頁面用的函式庫是 binary 的一部分，由鎖定檔建出來，不是執行時從外部載入：** `internal/jsruntime/v1/` 與 `runtime/` 重新建一次的結果逐 byte 相同（`make verify-runtime`，Docker 的 `runtime` stage 也做同樣的比對）；每個檔案名稱帶內容 hash、manifest 記錄每個 specifier 的 export 與每個檔案的 SHA-256、React 在所有檔案裡只有一份、沒有檔案引入 runtime 以外的來源、授權全文隨檔案內嵌。已發佈的版本不改變頁面看得到的內容，不相容的改變是新版本。
  Tests: `TestEveryEmbeddedFileIsInTheManifestWithItsHash`, `TestEveryFileNameCarriesAHashOfItsContent`, `TestRuntimeV1OffersExactlyTheLibrariesOfTheIssue`, `TestEverySpecifierIsAFileThatExportsWhatTheManifestSays`, `TestNoModuleBundlesItsOwnReactOrReachesOutsideTheRuntime`, `TestTheNoticesCoverTheBundledPackages`, `TestAVersionThisBinaryDoesNotCarryIsAnError`；與重建一致由 make verify-runtime 檢查。

- **`/vendor/{runtime}/{file}` 提供內嵌的函式庫：`Access-Control-Allow-Origin: *`、不需登入、帶 hash 的檔名 `immutable`，只提供 manifest 列出的檔案。**
  Tests: `TestReactCompileFailuresTellTheAgentWhatToFix`, `TestReactRefusesEmptyAndOversizedSource`, `TestReactStripsTypesAndDoesNotCheckThem`, `TestReactAScriptCloserInTheSourceCannotEndThePagesScript`, `TestReactThePageMapsEveryImportToTheRuntimeItWasPublishedWith`, `TestReactDeclaresExactlyTheInlineScriptsOfThePage`, `TestReactRenderIsMemoisedByContentAndRuntime`, `TestReactAFormatThisServerLacksIsRefused`, `TestReactEveryFormatVersionIsARuntimeThisBinaryCarries`, `TestReactIsRegisteredAndTeachesTheRules`, `TestPublishingReactOverHTTPAndMCPGivesTheSameResultAndTheSameRefusal`, `TestAnAgentLearnsTheReactKindFromKinds`, `TestAReactPageKeepsTheSandboxAndTheBridgeAndPinsItsScriptsByHash`, `TestOnlyPagesThePlatformGeneratesGetAScriptSrc`, `TestARuntimeFileIsServedToTheSandboxWithoutASessionAndCachedForever`, `TestOnlyTheFilesOfAnEmbeddedRuntimeAreServed`, `TestAReactPageThatShowsAnUploadedFileStillRunsAndLoadsIt`（檔案的 grant 先寫進 inline script 再算 hash）。**
