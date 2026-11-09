# internal/i18n

介面文字的目錄（`locales/<語言>/<area>.json`，area 是 key 的第一段）與語言的選擇。頁面怎麼用語言（cookie、`Accept-Language`、`<html lang>`）見 `internal/web/AGENTS.md`。

## 不變式

- **介面文字只放在 `locales/<語言>/<area>.json`，每個 area 檔只放自己的 key（`nav.json` 只有 `nav.*`），每個語言的 area 檔與 key 與英文完全一致；shell 和 React 前端不寫死文字（React 拿到的是 shell 的 `pictor-boot` 區塊裡整份目錄）；API 錯誤訊息不翻譯，維持英文。**
  Tests: `TestAcceptLanguagePicksTraditionalChineseForAnyZhAndEnglishForTheRest`, `TestEveryLanguageHasExactlyTheKeysOfEnglish`, `TestEveryLanguageHasExactlyTheAreaFilesOfEnglish`, `TestAKeyOutsideItsAreaFileFailsTheLoad`, `TestTheAreasAreMergedIntoOneCatalogue`, `TestEveryTextTheInterfaceAsksForExistsInEveryLanguage`, `TestThePageLanguageFollowsAcceptLanguage`, `TestAPickedLanguageWinsOverAcceptLanguageAndAnUnknownOneIsIgnored`, `TestThePickedLanguageOverridesAcceptLanguageInBothDirections`, `TestSwitchingLanguageReturnsToTheSamePageAndOnlyWithinTheSite`, `TestSwitchingLanguageRefusesAnUnknownLanguageAndCrossOriginPosts`, `TestEveryPageHandsTheAppTheLanguagesToOfferAndWhereTheSwitchReturnsTo`, `TestEveryPageIsTheShellWithTheCatalogueOfTheLanguageServed`, `TestSignInErrorsAreTranslatedAndKeepTheirHeaders`, `TestAPIErrorsStayEnglishWhateverTheBrowserAsks`；切換鈕本身在 web/src/gallery/Gallery.test.tsx、web/src/viewer/Viewer.test.tsx、web/src/pages.test.tsx；前端的 key 型別與 area 檔一致在 web/src/catalogue.test.ts。
