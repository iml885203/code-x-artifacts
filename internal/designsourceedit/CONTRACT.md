# Conditional source modules

These modules are inert and have no production HTTP/MCP/socket wiring. They do
not enable format 2 execution or complete #150.

`RevisionIssuer.Issue(Basis, exactSource)` returns an authenticated `sb1` proof;
`Verify(token, Basis, exactBaseSource)` checks a caller's explicit read base.
Basis contains artifact, file, format, saved version, live epoch and account.
There is no account/client last-read cache. A later read leaves earlier proofs
usable; another account, changed bytes/context or process restart refuses the
proof with `ErrSourceBase`. A caller must keep its draft, read again and reconcile.
Current service authorization is mandatory independently of a valid proof.
The issuer key is process-local, generated once for a service lifetime; no key
is persisted. Existing session signing is private to web, not a service API.
Sharing a domain-separated existing signer at composition is a wiring decision.

`Apply(current ParseInput, []Op) (Result, error)` copies the input and parses the
complete before/after bundles. Operations accumulate in source cells and only
the final batch is schema-validated; schema/value type transitions work in either
order without persisting an invalid intermediate state. `set_logic` compares logic and schema/defaults
as one unit; omitted schema keeps current declarations, and logic-only frames
retain absent props. Empty logic removes its whole declaration. An omitted schema
keeps props; an object creates/updates schema, and JSON `null` removes the whole
props declaration. Creating an absent combined unit compares `UnitDigest("", nil)`;
logic and props may also be created/deleted independently. New blocks are prepended
as top-level declarations without changing existing source bytes. Authored props
remain independent. `set_prop` compares one authored
key, distinguishes absence/null and validates the final schema. `set_literal`
addresses a literal source slot in a PID's text gap; immutable bindings and
surrounding marks remain source bytes. Repeated/imported projections never
receive copies of source edits. Unsupported operation names fail.

A rejected batch returns no output bundle and `Rejected` retains a detached
complete draft, index and current digest. Success returns inverse conditional
operations. Exact literal source spelling is carried in `rawText` for Undo;
it must encode the supplied text and pass the same text-only rule. An inverse
can conflict with a later writer and cannot restore over that writer.

Text gaps count preceding non-text children and literal slots count preceding
binding holes, including empty slots. Deleting an entire run preserves its Undo
target; transported inverse operations retain original entity spelling.

Props deletion requires `expectedValuesSHA256`, comparing the presence-sensitive
authored `values` object as well as the combined logic/schema digest. `Inspect`
returns `propsValuesSHA256` for this purpose; missing values and an empty values
object have distinct digests. This prevents deleting a collaborator's values.
Deleting props cannot share a batch with per-key prop edits. Newly created props
receive authored values through a subsequent per-key batch.

Deletion inverses carry `restoreBlocks`: exact whole declaration bytes in source
order and conditional placement anchors derived from parser node boundaries.
Element paths ignore text nodes, retain PID/tag identity and compare the source
gap bytes before insertion. Unrelated source edits survive Undo; changed placement
or a different existing declaration conflicts with the detached draft retained.
Exact repeated restoration is idempotent. Recreated props restore authored values
and supply the values digest for conditional redo. Deletion and literal edits
on the declaration's direct parent are overlapping targets and reject atomically
in either order; other literal targets can share the batch. The final complete
bundle is validated before any output is returned.

Current module limits (#150): rich-mark/structure/style operations are not wired here.
Literal operations use authored Segment.Span and source-only declaration metadata
from the authoritative parser. No HTML/template segment scanner is maintained
here. Typed delimiter characters are escaped as entities and remain literal text;
raw template-hole insertion through Y.Text is refused.

`sourcecodec.State/Seed/Read` partitions normalized-once source into protected
source envelopes, literal Y.Text, protected logic/schema and authored prop slots
in the existing `design` map. Tuple keys name source file/PID/child/segment or
prop key. Directives/imports/style/SVG/token bytes stay in the source envelope.
No expanded instances or second authoritative IR are stored. `Seed` initializes
only an empty isolated document. `CheckTransition(before, after)` validates the
whole projected source and rejects changed protected/unknown entries; it must
run before persistence/broadcast on the raw-client path. It is not registered
in roomkind and cannot protect the current production socket by itself.

Required composition (#150): capture exact source and its context atomically;
verify explicit bases on existing format 1 and 2 read/publish/file/session paths;
compare and commit under one serialized service/hub operation; recheck current
permissions/epoch; reject protected raw updates before log/broadcast; return
consistent HTTP/MCP conflicts with retained drafts. Saved-version-only and
last-seen-cache fallbacks do not meet this contract. The existing format 1 codec,
published bytes and runtime admission remain unchanged by these modules.

`Inspect(FramePlan)` returns read-only `sourceUnits` data: the combined logic /
declaration digest, presence-sensitive authored prop digests for every declared
key, and ordered literal selectors with exact half-open UTF-8 source spans and
expected hashes. It never hashes resolved defaults/runtime state or issues a
source revision. Current format/version/live epoch and a write proof come from
the service's separately authorized atomic source snapshot. HTTP raw headers
carry only opaque revision proof; bounded unit metadata belongs in JSON reads.

`ReadBases` retains explicitly keyed signed snapshots under caller-selected
retention and entry bounds, with existing Design manifest/frame byte limits.
It returns the original signed saved version, while ordinary autosave does not
retire the live epoch. Missing/expired/evicted/restarted/retired entries fail;
there is no saved-version or account/client inference. `ForgetArtifact` retires
proofs on source authority/permission changes. Fresh signed read nonces prevent
an identical later read from reviving retired input. Shared service writes must
resolve `Load`; direct cryptographic `Verify` is not a cache-miss fallback.

The service transport names are distinct: `renderRevision` identifies a complete
bundle/dependency projection; `sourceBase` is an opaque per-file writable read
proof (`sourceBases` for multiple files); `expectedSHA256` compares one owned
unit. Inspect emits none of these proofs or execution capabilities. Read-only /
historical views may inspect units but receive no writable source base.

`sourcecodec.Write` plans validated source changes for the authorized conditional
service path. Unchanged literal Y.Text identities survive, and changed strings
receive code-point-safe UTF-16 deltas; a peer's pending literal coedit is retained.
Invalid planning returns no write and leaves the current document unchanged.
This is not a socket authorization path or production codec registration.
