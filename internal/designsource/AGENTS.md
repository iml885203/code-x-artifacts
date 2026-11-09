# internal/designsource

Pure format 2 source parsing and planning. Never executes author code or writes
artifact versions. Existing designfmt remains the owner of ordinary drawing
rules and format 1.

- Source bytes survive unchanged except missing PID insertion; existing valid
  PIDs are stable, duplicate/invalid PIDs are errors, and spans address original
  UTF-8 bytes. Tests: `TestSourceIsPreservedAndMissingIDsAreAssignedOnce`,
  `TestInvalidSourceIsRejectedWithByteSpans`,
  `TestTextSegmentsAddressOriginalEntityAndBindingBytes`,
  `TestLogicHashUsesExactOriginalCRLFBytes`.
- Stored grammar has one typed source plan: explicit lookup paths, scalar text
  and conditions, typed keyed or explicit positional repeats, literal imports,
  and supported event names. No expressions or implicit object coercion.
  Tests: `TestDirectivesHaveTypedStructureAndPreserveHints`,
  `TestBindingsAreTextAndScalarConditionsDoNotCoerceContainers`,
  `TestRepeatIdentityIsTypedBoundedAndPositionIsExplicit`.
- Manifest/runtime pairing, complete owned frame sets and acyclic bounded
  imports are mandatory. Format 1 stays static and unchanged. Tests:
  `TestBundleChecksVersionOwnershipImportsAndStableDiagnostics`,
  `TestFormatOneRemainsStaticAndItsBytesAreUntouched`.
- Props are declared, bounded, and resolved into detached JSON values. Dynamic
  addresses pass the same drawing rules after resolution. Tests:
  `TestPropsAreDeclaredBoundedAndResolvedWithoutMutatingSource`,
  `TestDynamicAddressesAreValidatedAfterResolution`.
- AST checking inert logic is never executable admission. Native compilation
  and runtime proof are separate gates; this package has no author-code runner.
  Tests: `TestLogicSourceIsInertAndSyntaxDoesNotAdmitExecution`,
  `TestLogicRequiresARealClassASTAndDeclaredInstanceEvents`,
  `TestImporterClassHeaderSpansPreserveBodiesAndExposeRenameRisks`,
  `TestUnsupportedLogicAndExcessiveNestingAreRefused`.
- CSS data cannot change source declaration structure; number dimensions and
  whole value holes pass CSS grammar and existing drawing rules after resolution.
  Test: `TestStyleBindingsKeepDeclarationStructureAndTokens`.
- JSON input/output bounds and schema options are checked before serialization,
  with no callable/custom-marshaler execution. Tests:
  `TestDataLimitsRejectNonJSONAndPrototypeKeysBeforeSerialization`,
  `TestPropsSchemaAndValueBoundariesNameTheFailingPath`.

- Renderer templates exclude declaration scripts, and SVG namespace/case is
  supplied by the server AST without a parallel browser parser. Tests:
  `TestInertDeclarationsNeverBecomeRendererElementNodes`,
  `TestSVGNamespaceAndCaseAreResolvedByTheServerAST`,
  `TestRendererJSONFixtureIsTheExactServerFramePlan`.
- Typed numeric instance keys share JSON/JavaScript canonical spelling and
  negative-zero equality. Test:
  `TestNumericRepeatIdentityMatchesJSONNumberCanonicalization`.

- Source declarations stay outside visual Nodes while exact whole/content and
  complete direct-parent spans support source operations without another parser.
  Tests: `TestSourceDeclarationsRetainExactBlocksContentsAndDirectParents`,
  `TestDeclarationAbsenceAndEmptyContentAreRepresentedWithoutScanning`.

- Opaque source comments carry exact protected spans and complete direct parent
  identity without becoming visual nodes or requiring another source scanner.
  Test: `TestOpaqueSourceCommentsPreserveDistinctLiteralGapsAndParentIdentity`.

- Native Promise and async method syntax remains exact inert source, without
  compiled bytes or executable admission. Test:
  `TestNativePromiseAndAsyncMethodsRemainExactInertSource`.
- Table/select directive identity and literal/opaque UTF-8 spans follow the
  authored tree, independently of HTML repair. Test:
  `TestTableAndSelectDirectivesRetainAuthoredHierarchyAndSourceBoundaries`.
