# internal/store

名錄快照是查找用的快取，不是登入 profile，也不參與 artifact 權限判斷。

- **只有完整且有效的快照能在一個 transaction 裡更換名錄；缺少的人保留名稱並停止建議，失敗／不完整讀取不更換名稱或成員。較早開始的讀取不能蓋過較新的結果。**
  Tests: `TestOnlyCompleteDirectoryGenerationsReplaceMembershipAndKeepNames`, `TestAnInvalidDirectoryGenerationCannotPartlyReplaceTheRoster`, `TestAWriteFailureRollsBackTheEntireDirectoryGeneration`.

- **`opens` keeps one row per person and artifact (the latest open), at most the given number of a person's newest, never touches another person's rows and goes with the artifact; a later open always sorts after an earlier one, even within the same instant.**
  Tests: `TestAPersonKeepsOnlyTheirNewestOpensAndALaterOpenAlwaysSortsAfter`, `TestADeletedArtifactTakesItsOpensWithIt`.

- **`usage_daily` keeps one count per day, person, kind, action (open, or publish via web/agent) and references no artifact, so a deleted artifact's usage stays; every saved version is counted in the same transaction as the version, and the sweep deletes days more than `UsageKeptDays` (400) old.**
  Tests: `TestUsageCountsOlderThanTheRetentionAreSweptAndNewerOnesStay`, `TestUsageCountsRepeatOpensOfADayInOneRowThatOutlivesTheArtifact`.

- **Agent activity contains only bounded nonsecret token/client metadata and references; version and live-log writes keep their activity atomically. Retained history is owner scoped, capped at 1,000 rows and expires after 30 days; connection rows coalesce within a minute. Revocation never removes the nonsecret historical reference. Owned active OAuth grants are live credential references alongside PATs; a different owner's grant never marks history live.**
  Tests: `TestActivitySurvivesRestartAndRevocationWithoutSharingAnotherOwnersHistory`, `TestActivityCoalescesConnectionsAndExpiresWithStableCursorsAndAnOwnerCap`, `TestActivityFailureRollsBackTheContentAndRealtimeLogWithItsMetadata`, `TestActivityUsesHistoricalTokenLabelsAndThrottlesReadsWithoutMakingHistory`, `TestOAuthActivityTreatsOnlyTheOwnersActiveGrantAsLive`.

Own-comment deletion requires artifact access and author ownership; it clears the durable body, preserves roots with surviving replies, never cascades, and an authorized repeat succeeds. Tests: `TestOwnCommentDeletionKeepsOtherRepliesAndScrubsTheBodyAcrossHTTPMCPAndRestart`, `TestCommentDeletionRefusesOtherAuthorsAnonymousAndInvisibleArtifactsThroughHTTPAndMCP`.

- **OAuth protocol records retain only caller-scrubbed metadata under hashed credential keys. Expired records cannot be read or consumed, and authorization-code/refresh consumption is atomic across SQLite connections. Refresh consumption and its replay tombstone share a transaction so concurrent reuse cannot lose its grant reference. Nonsecret client grants survive restart; owner-scoped revocation is durable and never changes another owner's grants.**
  Tests: `TestOAuthRecordsSurviveRestartExpireAndStayInTheirNamespace`, `TestOAuthCredentialConsumptionIsAtomicAcrossConnections`, `TestOAuthGrantsKeepOwnersAndRevocationAcrossRestart`, `TestOAuthRefreshConsumptionRemembersTheGrantBeforeAnyReplay`, `TestOAuthReplayTombstoneCanOutliveTheCode`.
