# Design source plan (format 2, parser module)

`ParseBundle(ParseInput{Format, Manifest string, Files map[string]string})
(BundlePlan, error)` accepts one complete source snapshot. `ParseFrame(file,
source string) (FramePlan, error)` is the frame-only entry for importers and
source editing. Errors are `Problems` and plans also retain the same diagnostics
with code, message, severity and half-open UTF-8 byte spans in the original file.

The bundle requires artifact format `2`, manifest version `2`, and runtime
`pictor-design/2.0.0`. Existing designfmt manifest/drawing rules remain the single
rules for dimensions, pages, tokens, ordinary HTML/SVG, addresses, CSS and kits.
Only a private validation copy changes manifest version and removes runtime.
No format 1 API, bytes, renderer or manifest is changed.

The IR retains source elements and inline marks, literal/path text segments,
attributes, event method names, condition paths, explicit keyed or positional
repeats, literal manifest-owned import targets, and hint metadata. `FramePlan.Nodes` excludes all declaration script nodes;
logic and props exist only as separate metadata and original source. Valid
render templates contain no script elements. The server HTML AST resolves SVG
namespace and canonical tag/attribute case, matched by source PID; Node.Namespace
is empty for HTML or `svg`, and Attribute.Namespace identifies `xlink`/`xml`
where needed. Attribute.Name stays qualified (for example `xlink:href`). The
renderer uses fixed namespace URI mapping, never its own HTML parser or SVG
name table. `SourceRef`
is file plus PID. Repeat instance identities use typed `InstanceKey` values,
never a delimiter-concatenated DOM index. Number key spelling follows JSON/JS
canonicalization, including negative zero as `0`. Dependencies are manifest-scoped and
cycles/depth over eight are errors. Expanding a projection and enforcing total
render-node/imported-instance budgets belongs to the renderer, not persistence.

`FramePlan.Source` is always the original source. `Rewritten` inserts only
missing data-pid attributes; all other bytes, including CSS, comments, logic and
props blocks, remain identical. Existing valid PIDs are preserved, invalid or
duplicate PIDs are rejected. A second parse produces identical rewritten bytes.
Spans continue to refer to original bytes; reparsing rewritten source establishes
new spans. Authored text Segment.Span identifies each raw literal run or hole,
including original entity spelling. Entities decode only within literal runs,
so encoded braces remain literal text. Computed/attribute/style segments can
have an empty-file span; their enclosing Node.Span remains authoritative. Logic
source and its SHA-256 use exact original bytes, including CRLF line endings.
An error returns a diagnostic/inspection plan only; it must never be rendered
or persisted as an admitted bundle. This strict tokenizer tree refuses malformed/misnested/unclosed tags
rather than silently using browser-repaired source identities.

Logic is inert source plus SHA-256 and AST-derived declared instance methods.
`ValidateClassicClass(source, className, baseName)` uses pinned parser-only
`github.com/tdewolff/parse/v2 v2.8.19`; esbuild has no public AST API. The same
maintained package parses CSS declarations and retains otherwise discarded
source-attribute conflicts. It adds no VM, engine, evaluator or execution path.
The helper admits exactly one named classic class extending the expected base,
with literal, unreserved instance methods. Fields, static blocks/methods,
getters/setters, private/computed members and duplicate methods are diagnosed.
AST nesting is bounded at 128, source at 128 KiB. Standard Promise/JSON/collection/math/
string/date/error globals are allowed; undeclared host/network/storage/worker
and code-generation globals, imports/exports, direct dynamic code generation,
with statements and explicit prototype traversal are diagnosed. These are input
contract checks, **not a JavaScript sandbox** or proof against indirect native
capability access. The native Worker/CSP boundary remains mandatory.

The helper returns original header identifier byte spans and class/base reference
flags so an importer can replace only a validated header, preserving method body
bytes and refusing scope-sensitive renames. Event bindings must name an actual
declared instance method. LogicSource's existing `SyntaxChecked` field now means
successful full source-contract analysis; `Methods` contains the sorted names.
`CompiledSHA256`, `CapabilityProfile` and `WorkerProgram` remain empty pending
the runtime owner's exact pinned program-byte builder and proof #148.
`RequireExecutable` still refuses logic/event plans. There is no production
format dispatch, renderer, broker, compiler-byte admission or browser HTML parser.

Style bindings use explicit property/value declarations, with immutable property
names and typed CSS value holes. Dimension suffixes require finite JSON numbers;
other holes are CSS values, parsed after resolution and checked through the
existing drawing/property/address validator. Data cannot supply declaration
boundaries, escapes, comments, priorities or quoted-string/url fragments.
Whole values such as `var(--ink)` retain token references. Source CSS bytes are
never serialized from the IR. Plain scalar attributes and whole URL holes also
validate after substitution.

Props use schema/values, defaults then saved values then import overrides, with
whole object/array replacement, bounded JSON types and undeclared-key rejection. Duplicate JSON keys, invalid
type-specific schema fields/defaults, prototype keys and over-limit data are
refused. CheckData is the shared plain-JSON data boundary (256 KiB, depth 16,
10,000 primitive leaves), with no reflection or custom marshal callbacks.
The current module does not wire CAS, services, source edit transactions, HTTP,
MCP, rooms, render contexts, exports, executable routes or CSP. Passing parser
unit tests does not complete #149 or enable runnable production rendering.


## Renderer fixture

`testdata/source.plan.json` is the exact default Go JSON serialization of the
validated `FramePlan` for `testdata/source.html` / `testdata/design.json`, checked
by `TestRendererJSONFixtureIsTheExactServerFramePlan`. Field casing matches the
exported Go fields (props/schema retain their explicit JSON tags). Nil arrays/maps
serialize as null. It includes the final declaration-free visual Nodes, SVG
namespaces/case, original text spans, inert logic metadata, props, and typed
bindings. Consumers render **Nodes only**, never Source/Rewritten/Logic.Source.
The original source strings belong to trusted source editing; worker compilation
is an independently admitted artifact. A whole BundlePlan's Go map keyed by
SourceRef is an internal index, not a separately defined JSON transport schema.


## Source declarations

`FramePlan.Declarations []SourceBlock` is source-only metadata. Each record has
`Kind` (`logic` or `props`), exact whole `Span`, exact raw UTF-8 `ContentSpan`,
complete direct authored `ParentSpan`, and `ParentPID` when the parent has a PID.
No parent is represented by an empty-file ParentSpan and empty ParentPID. A head
parent can have no PID while retaining its complete source span. Records are in
source order; absent declarations produce no records. Empty content intervals
have equal start/end offsets, including on a diagnostically invalid empty logic
block. `FramePlan.Source[ContentSpan.StartByte:ContentSpan.EndByte]` is the sole
raw content; no duplicated source bytes or scanner is needed. CRLF stays exact.

Visual Nodes remain declaration-free. Source-operation owners use declarations
as non-text boundaries when identifying literal gaps, including separate text
runs before/after inert blocks and protected declarations in the head. This
metadata is not a write token or executable admission. The representative exact
Go JSON fixture includes the additive declaration records.


`FramePlan.Opaque []SourceBlock` retains HTML comment token boundaries as
`Kind="comment"`. Span and ContentSpan both identify the complete protected
opaque token (including an empty comment); it exposes no editable interior.
ParentSpan/ParentPID use the same exact direct authored-parent contract as
Declarations, with an empty-file parent sentinel at document level. Opaque
records are source-ordered, never visual Nodes. Source layout counts them as
non-text boundaries so text before/after a comment cannot alias one literal
gap and whole-run deletion keeps a valid own-Undo target. This is source
serialization metadata, independent of the product comment feature.


## Consumer compatibility

The inert #150 compatibility baseline is `224d07b504b0e3a344e6538eb9b7f1032ce5975d`,
including declaration metadata from `17b9c83` and opaque comment metadata from
`60e6b0c`. ParseInput, ParseBundle, ParseFrame, FramePlan, SourceBlock and their
half-open original UTF-8 spans retain that interface. No source-operation or
sourcecodec writer is added here. Source consumers must reparse normalized bytes
before using spans against those normalized bytes.

Native Promise references and async/await instance methods are accepted as inert
classic source; their exact bytes and hash are retained. This permits no native
execution or microtask/watchdog claim. Other class-member input restrictions
listed above remain explicit diagnostics, not sandbox enforcement.

Strict authored table/select trees can contain repeat/condition source directives
around tr/option nodes. Source PIDs, literal holes and opaque boundaries retain
that authored hierarchy even where HTML tree construction would foster-parent
or discard directive elements. Consumers must lower directives from typed IR,
not serialize directive wrappers into an HTML string and reparse it. The inert
round-trip fixture proves source identity only; actual ordinary-node projection
and browser behavior remain separate #149 acceptance work.
