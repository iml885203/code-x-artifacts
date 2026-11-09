# Design source read bases

Current live Design format 1 reads and whole-file publishes use an explicit
per-file baseline. Internal format-2 read-base machinery does not admit that
format: executable routes and the source-room codec are not registered. The
runtime/source expansion described in #148 and #150 is outside the authorized
beta scope, not a prerequisite for current-format behavior.

## Read and publish

MCP `read {id}` returns current content. For Design only, HTTP
`GET /api/artifacts/{id}` returns artifact metadata and `content` containing the
exact current manifest bytes from the same snapshot as its proof. Readers get
the same manifest without write proofs. Non-Design HTTP artifact reads retain
the metadata-only response and do not read the live room. An editor of a live
Design receives `sourceBases: {"design.json": "<opaque proof>"}`.
MCP `read {id, file: "frames/hero.html"}` returns `file.text`, `file.present`,
`file.sourceBase` and the same proof in `sourceBases` under the frame path.
The HTTP file API returns plain text and `X-Pictor-Source-Base`. These responses
use `Cache-Control: no-store`; callers must not share, log or persist the proof.

Keep the proof with the exact source read used to prepare the draft. Send
`sourceBases` with `publish` or HTTP `PUT /api/artifacts/{id}`, alongside the
usual `baseVersion`, content and changed files. Only the manifest and files
being changed need proofs. A second read, even from the same account and client,
does not replace the first proof. Proofs are bound to the authenticated account,
artifact, file, format and live epoch; they never grant edit permission.

An editor can read a valid absent current frame path. MCP returns
`file.present: false`; the HTTP file API returns JSON
`{path, present: false, sourceBase}`. Use that absence proof to add the file;
it cannot replace a file another publisher has since created. A present empty
file and an absent file are distinct. Legacy service `ReadFile` still refuses
missing files. Reader and explicit saved-version reads never return write bases.
Explicit file versions read saved bytes even when the number is the latest.

A changed live frame requires its own proof. Changed manifest content requires
its manifest proof. Metadata-only publishes may repeat saved manifest content
without a proof; identical-current files and omitted files need no proof.
Static artifacts retain their ordinary version contract. A truly unopened Design
read issues no proofs; if a publish supplies them anyway, retain and reconcile
the draft, reread, then publish without `sourceBases` using the returned `version`
as `baseVersion`. Idle room unloading does not make an opened Design non-live: its
durable room log remains the working copy. Every supplied proof
is checked, including on a no-op; malformed, unrelated or retired input is not
silently ignored.

## Refusals and retention

Missing, expired, evicted, retired, wrong-scope or pre-restart proofs produce a
recoverable conflict before any source merge, metadata update or activity row.
Retain the draft, read current source, reconcile changes and retry with its
new proofs. The server never substitutes saved content, a last-read pointer,
another read or a client-supplied baseline body. Overlapping edits still use
the existing Design merge conflict rules; independent changes are retained.

The registry retains at most 500 file reads for 30 minutes per service lifetime.
Access changes and restore retire all reads for the artifact. Ordinary snapshots
do not retire the live epoch; `baseVersion` remains independently checked.

Large publishes send only hashes/sizes at `begin_publish`, upload each body once,
and pass `sourceBases` with the session ID at `commit_publish` (or in the HTTP
commit JSON body). No baseline source body is duplicated or stored in the session.
A refused commit retains its uploaded draft until the existing session expiry.

## Business admission and source comparisons

One process registry admits source operations before authorization reads and
Snapshotter/Hub/room locks. Source reads and writes hold shared admission;
sharing, visibility, deletion and restore hold exclusive admission. A publish
that names visibility takes exclusive admission for its entire operation before
reading current authority. Shared admission permits concurrent source writers.
The registry retains entries only while an operation holds or awaits them.

Socket admission holds the same shared guard across current rights and Join;
each raw peer message holds it before the room lock and checks closed state
inside that lock. Access-change watchers execute within exclusive admission,
recheck rights using unguarded service reads, and disconnect stale peers before
releasing it. They must not call guarded reads or mutators. Public service
wrappers acquire once; publish metadata uses private already-admitted helpers.

The lock order is artifact admission, Snapshotter mutex when needed, Hub mutex,
then room mutex. Update callbacks run after the room lock is released and never
acquire artifact admission. Quiet snapshots stay guard-free: they save versions
without changing authority or live source, with existing stale-version checks.
Native Hub reads/edits assume caller admission; native epoch replacement requires
exclusive caller admission when used with business authority. No production
source reset route is introduced.

Epoch, authoritative absence and whole-frame deletion expectations are checked
inside the actual Hub mutation lock, before the three-way merge, append or
broadcast. A concurrent peer edit cannot be erased by a previously resolved
whole-frame removal; a concurrent creation cannot be overwritten by an absent
baseline. Ordinary text edits retain the existing independent-change merge.
Refusals retain the caller draft and require a fresh reconciled source read.

## Remaining source-authority boundary

This is current-format-1 business ACL admission, not a protected format-2 raw
Yjs policy. Format-2 logic/declaration and per-prop authority, source-op transports,
style/tree/token operations, codec dispatch and repeated-instance runtime/UI
integration are not admitted capabilities or current beta prerequisites.
No executable route, CSP change or alternate executor is enabled here. Scope
cancellation is not validation or security approval; any future authorized
activation still requires the applicable architectural and security invariants
and actual verification, as described in [goal.md](artifact-types.md#design-範圍與需求來源).
A successful live merge and a later metadata/storage failure are not one new
transaction; this admission guard does not claim whole metadata atomicity.
