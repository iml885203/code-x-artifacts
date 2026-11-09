# internal/designsourceedit

Pure source read identities and bounded explicit read snapshots. This package does
not authorize callers, persist changes, register executable formats or broadcast rooms.

- Proofs bind exact source bytes and artifact/file/format/version/epoch/principal context; missing, altered or pre-restart proofs refuse.
  Tests: `TestReadBasesAreExplicitAndBoundToTheirExactSourceAndContext`, `TestAReadBaseCannotBeIssuedForAnIncompleteOrAmbiguousContext`.
- Independent reads retain their own baselines; scope, expiry, eviction, bounds and retirement cannot select a guessed fallback.
  Tests: `TestExplicitReadBasesSurviveIndependentReadsAndRefuseExpiryEvictionAndRestart`, `TestReadBaseLookupIsScopedAndDoesNotMistakeASaveForALiveEpochChange`, `TestConcurrentReadsOfOneAccountKeepTheirOwnExactBaselines`, `TestReadBaseRegistryRejectsInvalidBoundsOversizedSourcesAndUnboundContexts`, `TestRetiredProofsCannotReviveAfterTheSameSourceIsReadAgain`.

- An absent frame read is distinct from a present empty source and retires with the artifact. Tests: `TestAbsentReadBasesAreDistinctFromPresentEmptySources`.

## Inert operation modules

# internal/designsourceedit

Pure conditional edits of authoritative format 2 source, plus read-base proofs
shared by format 1 and 2. No authorization, persistence, executable admission or
room broadcasting lives here.

- A read base is authenticated over exact source and artifact/file/format/saved
  version/live epoch/account; a second read cannot replace the first base, and
  restart/mismatched context requires a fresh read while preserving the draft.
  Tests: `TestReadBasesAreExplicitAndBoundToTheirExactSourceAndContext`,
  `TestAReadBaseCannotBeIssuedForAnIncompleteOrAmbiguousContext`.
- Conditional operation batches return either a detached fully validated bundle
  and own undo, or no bundle and the entire detached draft. Props compare per
  authored key, logic plus schema/default declarations compare as one unit, and
  own undo cannot overwrite a later writer's unit.
  Tests: `TestIndependentPropChangesMergeAndStaleOrInvalidBatchesKeepTheEntireDraft`,
  `TestLogicAndDeclarationsAreOneConditionalUnitAndNeverOverwriteConcurrentProps`,
  `TestConditionalUnitsRejectBindingInjectionBadDeclarationsAndAliasedTargets`,
  `TestAuthoredPropAbsenceAndNullHaveDifferentDigestsAndUndoKeepsSource`,
  `TestDeletingWholeLiteralRunsKeepsTheirOwnUndoTargetsAcrossTransport`,
  `TestSchemaAndAuthoredValueTransitionsValidateTheFinalBatchInEitherOrder`,
  `TestDeclaredPropsWithoutAnAuthoredValuesObjectAcceptConditionalOverrides`,
  `TestLogicWithoutPropsKeepsAbsentDeclarationsAndOwnUndo`.
- Literal edits target one source template run, preserve binding holes and
  surrounding rich marks/source bytes, and cannot introduce bindings or markup.
  Test: `TestLiteralEditsAddressOneTemplateRunAndPreserveBindingsMarksAndUnownedBytes`.

- Authored entities remain literal source and opaque comment boundaries keep
  distinct run targets through whole-run Undo. Tests:
  `TestTypingTemplateDelimitersKeepsThemLiteralAndRoundTripsOriginalEntityBytes`,
  `TestOpaqueCommentsSeparateSourceLiteralTargetsAndWholeRunUndo`.

- Read-only inspection returns source hashes and half-open UTF-8 literal
  coordinates, never a write proof or resolved/default value hash. Tests:
  `TestInspectionReturnsAuthoredUnitDigestsAndExactLiteralSourceCoordinates`,
  `TestInspectionDistinguishesAbsentOverridesFromDefaultsAndHasNoWriteToken`.

- Explicit bounded read snapshots do not replace each other by account/client.
  Missing, expired, evicted, retired or pre-restart proofs never fall back to a
  saved/latest source. Fresh nonces prevent retired input revival. Tests:
  `TestExplicitReadBasesSurviveIndependentReadsAndRefuseExpiryEvictionAndRestart`,
  `TestReadBaseLookupIsScopedAndDoesNotMistakeASaveForALiveEpochChange`,
  `TestConcurrentReadsOfOneAccountKeepTheirOwnExactBaselines`,
  `TestReadBaseRegistryRejectsInvalidBoundsOversizedSourcesAndUnboundContexts`,
  `TestRetiredProofsCannotReviveAfterTheSameSourceIsReadAgain`.

- Declaration creation/deletion shares the logic/schema CAS unit. Props deletion
  also compares authored values, and exact whole-block Undo checks placement and
  never replaces a concurrent declaration. Overlapping parent literal/prop edits
  reject the entire batch. Tests:
  `TestAbsentLogicAndPropsCanBeCreatedAndDeletedWithConditionalExactUndo`,
  `TestBlockDeletionUndoPreservesExactDeclarationsAndUnrelatedConcurrentSource`,
  `TestBlockLifecycleRejectsStaleUnitsAndValuesAndRetainsDetachedDrafts`,
  `TestMalformedBlockCreationRejectsTheWholeBatchWithoutExecutingLogic`,
  `TestDeclarationRestorationIsOrderedIdempotentAndConflictsOnChangedPlacement`,
  `TestRestoredDeclarationsRejectMovedBlocksAndKeepDetachedDrafts`,
  `TestDeclarationDeletionRejectsOverlappingLiteralAndPropTargetsAtomically`,
  `TestDeclarationPresenceTransitionsKeepIndependentPropsAndExposeDeletionDigest`.
