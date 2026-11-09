# internal/designrender

Pure source-context preparation and ordinary projection only; no author execution, runtime registry,
production dispatch, resource grants or executable document emission.

- Contexts require one explicit stored version or live revision, an owned frame
  and an explicit purpose. They validate the complete snapshot with designsource,
  and retain exact source dependency bytes; history is immutable. Tests:
  `TestEveryContextPurposeRetainsOneStoredSnapshotAndSourceClosure`,
  `TestContextRefusesMissingSnapshotIdentityAndNeverFallsBackToRunnableOutput`.

- Projection lowers typed repeats/conditions/imports into ordinary nodes using
  explicit plain-data snapshots only; source IR and declarations are immutable.
  Tests: `TestProjectionLowersTableAndSelectDirectivesWithoutHTMLRepair`,
  `TestProjectionUsesExplicitImportSnapshotsAndDeclaredInstanceProps`,
  `TestProjectionRetainsKeyedAndExplicitPositionalIdentityOnReorder`,
  `TestNestedRepeatsAndImportsKeepIndependentSuppliedInstanceData`,
  `TestProjectionResolvesRootSavedPropsWithoutInventingState`.
- Invalid data, context identity, expansion and output bounds fail atomically
  with source diagnostics, without partial output or author execution. Tests:
  `TestProjectionRejectsInvalidDataAtomicallyWithOriginalSourceSpans`,
  `TestProjectionBoundsExpansionIncludingEmptyLoopsAndRepeatedText`,
  `TestProjectionBoundsAggregateInstanceDataBeforeExpansion`,
  `TestProjectionRetainsTheSharedPlainDataDepthBoundary`,
  `TestProjectionRefusesReservedDuplicateUnusedAndNonJSONSnapshots`,
  `TestProjectionRevalidatesSourceAndPinnedRuntimeIdentityWithoutAdmission`.
- Attributes, SVG, token references and passive event/navigation metadata retain
  their source meaning without producing runnable documents. Tests:
  `TestProjectionResolvesAttributesAndSVGWithoutEnablingSourceEvents`,
  `TestContextRequiresExactPinnedRuntimeIdentifierAndFormatPair`.
