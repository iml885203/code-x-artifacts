# Artifact admission

This leaf package has no business, room, store or transport imports. The registry
map mutex is released before waiting on an entry. An entry remains while any
operation holds or awaits it and retires after its last release.

- Shared source operations coexist; an exclusive operation on one artifact does
  not block another artifact. Tests:
  `TestSharedAdmissionCoexistsAndOtherArtifactsDoNotWaitForAnExclusiveHolder`.
- Waiters retain the same entry as its current holder; all entries retire when
  every holder and waiter leaves. Tests:
  `TestWaitingAdmissionsKeepOneEntryUntilEveryHolderAndWaiterLeaves`.

Wire one registry through service, peer receives and socket admission before use.
Acquire admission before Snapshotter/Hub/room locks. It is not reentrant: public
wrappers acquire once, while their private helpers and callbacks never reacquire.
