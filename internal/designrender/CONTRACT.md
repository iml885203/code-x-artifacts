# Inert source contexts

`BuildContext(Snapshot, Request)` validates the complete format-2 snapshot with
`designsource.ParseBundle` before selecting a manifest frame. Snapshot acquisition
must be one service-authorized immutable version or atomic live room read. The
pure function cannot establish authorization or atomicity. `CanEdit` is the
service result; history always clears it. Version is positive with no live
Revision, or zero with an explicit server-issued Revision.

Purposes are canvas, play, preview, history, raw, overview, offline and print.
These labels select source metadata only; they do not implement those routes.
There is no default purpose, current-version substitution or format-1 fallback.
Existing format-1 dispatch remains owned by its current implementation.

The returned plan owns parsed maps and exact immutable source strings.
Dependencies contain the root and transitive manifest-owned imported source
files, sorted by file, with SHA-256 over their original UTF-8 bytes. The dependency
digest hashes Go JSON of format, runtime ID, exact manifest and the ordered source
digests. It includes manifest tokens and import closure; it is not an asset,
compiled-program, granted-document or executable-runtime digest. Unrelated frame
source changes do not enter this closure. Source normalization still requires
reparsing before consumers use spans against newly persisted bytes.

`RuntimeSHA256` is empty. There is no registered executable runtime, document
builder, asset resolver, bridge role, FrameDocuments output or production wiring
in this module. It must not be treated as completion of #149 context/route/runtime
acceptance or as evidence for #148. Runtime pinning and resource/document hashing
require independently accepted runtime bytes and service integration.

## Ordinary projection from supplied data

`Project(RenderContext, []InstanceData) ([]*ProjectedNode, error)` performs pure
projection from exact source and explicit plain JSON values/state. It reparses
context sources and verifies format, runtime identifier, frame and dependency
identity before projection. Runtime IDs must match the stored explicit
`pictor-design/2.0.0` and format-2/manifest-2 pair exactly. This is structural
validation, not executable registry admission; nonempty RuntimeSHA256 is refused.

Every visited root/import requires one InstanceData record keyed by owning source
file and structured Path. Missing, duplicate or unused instance snapshots are
errors. Empty data is explicit; no guest initial state, renderVals, lifecycle,
handler, promise or native program is evaluated or synthesized. Values cannot
supply props/state roots. Resolved props come from schema defaults, saved values,
then declared import-instance overrides; imported values/state are supplied
separately for that exact instance. Inputs are bounded plain JSON and detached
before projection. Imports only use the same validated source snapshot.

Repeat/condition/import wrappers disappear from the ordinary projected tree.
Projection constructs typed element/text records directly, without serializing
custom directive tags and applying HTML tree repair. A Path segment names its
source site; a nonnil typed Key means a repeat, and nil Key means an import site.
Nested sites append segments. SourceRef and Span retain original owning source
and half-open UTF-8 identity. Keyed reorder retains logical item identity;
explicit positional reorder retains slot identity. Text nodes carry their
nearest owning source ref and their original text span. Declaration scripts and
opaque comments remain in source metadata only. Projected attributes retain
namespace and resolved value; dynamic CSS/address validation reuses designsource.
Bound hostile text is a string value, never HTML. Event methods/navigation targets
are passive source metadata and grant no dispatch, navigation or edit authority.

One root is limited to 10,000 source-node visits/repeat iterations (including
empty loops) and 1 MiB aggregate text/attribute payload. Repeat/data/schema/import
limits also retain the parser contract. Errors return no partial tree; dynamic
resolution errors carry a projection code and original enclosing source span.
Projection trees are transient output and must never replace template source.

The shared parser interface is pinned at
`39b9dfef951757143496c8a30dfe8bae6bc2676d`: ParseInput, ParseBundle, ParseFrame,
FramePlan, Node, SourceBlock and their JSON fixture remain unchanged by projection.
Projection-only types live in designrender; #150 source operations and #154
importers continue to consume the same source IR. The sole main/push integrator
composes these modules; source projection does not activate format-2 HTTP/MCP
admission, an executable renderer, a registry, assets/grants, ZIP or print.
