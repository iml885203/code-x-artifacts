# internal/realtime

即時同步層（`docs/realtime.md`）：`Hub`（room 的載入、卸載、壓縮、限制）、`Peer`（一條連線：y-websocket 訊框進出）、`Edit`（server 端改 room）、roster 與 agent 的 presence。不知道 socket、session 或文件的意思；只用 ygo 的 `crdt`／`sync`／`awareness`／`encoding`。
`interop/` 是產生與檢查 yjs fixture 的 Node 腳本（自己的 go.mod，理由同 `web/`）。socket 的 transport 在 `internal/web/live.go`。

## 不變式

- **一個 artifact 同時只有一份活的內容，就是記憶體裡的 room；被確認的 update 在回傳之前已經寫進 `rt_updates`，之後才廣播給別人。** `Peer.Receive` 與 `Hub.Edit` 回傳 nil 就代表那一列已在磁碟上；存不下來的 update 不送出、room 從記憶體丟掉並由磁碟重載。log 以排程、達到列數、卸載與關閉壓縮成一列，壓縮不改 epoch；只有重建 log（`Hub.Replace`）才換 epoch，舊 epoch 的 client 被告知從頭來過（`*EpochError`）。閒置的 room 卸載後沒有東西持有它。room 在記憶體，所以部署只能 1 個 replica。
  Tests: `TestTwoClientsConvergeAfterConcurrentEdits`, `TestAnAcknowledgedUpdateIsStillThereWhenTheRoomIsReopened`, `TestEveryAcceptedUpdateIsOnDiskBeforeItIsAcknowledged`, `TestAHandshakeThatChangesNothingAddsNoRow`, `TestTheUpdateLogIsCompactedAfterTheThresholdAndKeepsTheContent`, `TestTheLogOfARoomThatStaysOpenIsCompactedOnASchedule`, `TestAClientFromAnOlderEpochIsToldToStartOver`, `TestAnUpdateThatCannotBeSavedIsNotSentAndTheRoomIsReloaded`, `TestAnEditThatBreaksTheDocumentLimitKeepsNothingAndTheRoomIsReloaded`, `TestAnIdleRoomIsUnloadedAfterItsTimeAndComesBackFromDisk`, `TestAnUnloadedRoomsMemoryIsReleased`, `TestClosingTheHubDisconnectsEveryoneAndCompacts`, `TestEmojiAndCJKSurviveAnEditAndAReload`, `TestARoomOfADeletedArtifactCannotBeJoined`, `TestARoomsLogHasOneEpochAndOnlyForAnArtifactThatExists`, `TestAppendsKeepTheirOrderAndNeedTheEpochOfTheLog`, `TestARoomsLogGoesWithItsArtifact`（store）。

- **room 的限制：單一 update、room 的人數、awareness 訊息、文件大小（一般 2 MiB，seeded log 可提供種類的上限），超過時拒絕並在訊息裡寫出限制的名稱、這次的值和上限；被拒絕的 update 不套用、不存、不廣播。**
  Tests: `TestAnOversizedUpdateIsRefusedNamingTheLimitAndTheValueAndChangesNothing`, `TestAPeerOverTheCapIsRefusedNamingTheLimitAndTheValue`, `TestAnOversizedAwarenessMessageIsRefusedNamingTheLimitAndTheValue`, `TestADocumentOverTheCapIsRefusedNamingTheLimitAndTheValueAndKeepsWhatFitted`, `TestAConnectionThatDoesNotKeepUpIsDroppedAndTheOthersAreNot`。

- **唯讀的連線收得到編輯，但它的寫入在 server 丟掉：不進 room、不進 SQLite、不到其他 peer、不觸發 `OnUpdate`。** 作者與 roster 的名稱是連線被授權的身分，不是 client 在 awareness 自報的；一個 awareness id 只有發佈它的連線能改。
  Tests: `TestAReadOnlyPeersWriteNeverReachesTheRoomTheLogOrOthers`, `TestEveryUpdateIsAttributedToTheConnectionThatSentIt`, `TestTheRosterShowsTheAuthenticatedNameNotTheReportedOne`, `TestASelectionInAwarenessReachesOthersUnderTheNameTheServerKnows`, `TestAConnectionCannotSpeakForAnotherConnectionsAwareness`, `TestAConnectionThatLeavesIsRemovedFromTheRoster`, `TestAnAgentTheServerSpeaksForShowsInTheRosterUntilItLeaves`, `TestAnAwarenessMessageThatIsNotAnObjectIsRefused`, `TestDisconnectClosesOnlyTheMatchingConnections`。

- **在 server 改 room 只經 `Hub.Edit`（與 peer 的寫入同一條路：存、廣播、回報作者）；plan 先讀、write 只寫，因為 ygo 在 transaction 裡取共享型別會永遠卡住。**
  Tests: `TestAnEditFromTheServerIsSavedSentToEveryPeerAndMayReadBeforeItWrites`, `TestReadingASharedTypeInsideATransactionNeverReturns`。

- **依賴釘死、與真的 yjs 互通：`go.mod` 釘 ygo 的確切版本，yjs 釘在 `interop/package.json`；升版要讓 `make verify`（含 `make verify-interop`）全過。**
  Tests: `TestUpdatesFromTheRealYjsLibraryApplyInTheRoom`, `TestUpdatesTheRoomProducedAreWhatTheCommittedFixturesHoldAndApplyInYjs`；make verify-interop。

- **兩個人各自合理的搬動合起來是環時，由 hub 在保留變更之前檢查（`realtime.Config.Validate` → `designroom.Check`）：不存、不廣播，room 重載，那個連線以 `ErrRoomReset` 結束。**
  Tests: `TestAChangeTheHubsCheckRefusesIsNeitherSavedNorSentAndTheRoomGoesBackToWhatItWas`

- **agent 透過 live room 寫 doc 時出現在 roster：名字是「<帳號> 的 <client>」（client 由 MCP 的 clientInfo 判斷，不認得是「agent」），帶它改的區塊與時間，閒置 `AgentIdle`（15 秒）後移除；它是 server 注入的（`Hub.Announce`），瀏覽器連線既不能自稱 agent，也不能用 server 持有的 awareness id 發佈或移除。**
  Tests: `TestAnAgentPublishesUpdatesSharesAndListsThroughTheMCPProtocol`, `TestAnAgentReadsAndWritesTheSharedDataThroughTheMCPProtocol`, `TestTheEndpointNeedsAPersonalTokenAndIgnoresTheBrowserSession`, `TestRefusalsTellTheAgentWhatWasWrongAndWhatToSendInstead`, `TestAToolAndItsHTTPCounterpartGiveTheSameResult`, `TestDeleteWithoutAValidConfirmTokenDeletesNothing`, `TestEveryToolAndArgumentIsDescribedForTheAgent`, `TestAConfirmTokenWorksOnceForTheSameCallerToolAndArguments`, `TestAConfirmTokenExpires`, `TestEveryKindTeachesItsAuthoringRules`, `TestTheConnectPageHandsTheAppThisServersAddressAndTheTextOfTheSetupInBothLanguages`。

- **agent 發佈進 design 的 live room 時（`Hub.AnnounceTouched`）出現在 roster 的方式與 doc 相同（同樣的名字規則、server 注入、`AgentIdle` 後移除），帶它改的元素 `touched: {frame: [pid…]}`（總共至多 `MaxTouched` 個）；瀏覽器自稱 `touched` 不會讓它變成 agent。**
  Tests: `TestAnAgentAnnouncedWithTheElementsItChangedShowsThemToEveryoneAndLeavesAfterTheIdleTime`, `TestAnAnnouncementNamesNoMoreElementsThanTheLimit`, `TestWhatABrowserSaysItTouchedDoesNotMakeItAnAgent`。

## 坑

- fixture 在 `testdata/yjs`；重新產生用 `$(NODE) internal/realtime/interop/generate.mjs internal/realtime/testdata/yjs`（Makefile 的 `verify-interop` 失敗訊息會印出完整指令）。

- Peer receive acquires the shared process admission before the room lock and checks closed state inside it. Native Hub operations do not reacquire admission; authority-aware replacement requires exclusive caller admission. Callbacks never acquire outer admission. Tests: `TestRevocationInFlightExcludesPeerPublishAndSocketJoinUntilTheWatcherClosesOldRights`, `TestSocketRightsAndJoinCannotStraddleTheRevocationWatcher`.

Encoded load and replacement respect the document budget before replay/persistence. A refused replacement retains connected writers and the prior epoch. Tests: `TestAnOversizedReplacementKeepsTheLogEpochContentAndConnectedWriters`, `TestDesignSeedsAndLoadedRoomsUseTheSameBoundWithoutDestroyingAnOversizedLog` (internal/roomkind).
