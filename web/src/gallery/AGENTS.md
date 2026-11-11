# Library

`Gallery.tsx` owns filtering, grouping and the grid/list choice; `Nav.tsx` links only existing destinations. The account menu belongs to `shell/AccountMenu.tsx`; the rail uses one account trigger for the avatar and available profile name, with the account identifier retained in its menu. User navigation offers Connect your agent and does not advertise Kit; direct Kit routes remain available.

## Invariants

- The navigation has one connection/guide destination. Connect reuses this rail and marks only its own link current; its links back to the library use normal server navigation. The empty-library action uses the same translated Connect your agent label. On phones it is one compact row, with the existing links behind the Menu button and the account menu still reachable. Search follows immediately; the viewport's spare height belongs to content, not the navigation row.
  Tests: Gallery.test.tsx (opens the rail with one agent connection destination…); browser capture matrix at 1440px and 390px in both languages and themes.
- A card has one outer edge and an inset, clipped, faded image preview. Warm cards load JPEG images without artifact documents or fonts. A near-viewport miss queues at most two hidden 560×360 opaque-origin frames (1120×720 output), with no viewer bridge; successful image upload removes its frame. Failure leaves a lightweight placeholder, never a broken image or live fallback. Theme/language variants are bounded; full viewers retain normal scrolling. Tests: Preview.test.tsx, gallery-cards.spec.ts.

- Cards and list rows use the same action registry and decorative, hidden-from-assistive-tech icons. Icons do not alter action labels, ordering, permission visibility or P/C/R keyboard shortcuts.
  Tests: Gallery.test.tsx (gives a row the same menu…, runs an action from its letter…); artifactActionList.test.ts; e2e/tests/gallery-cards.spec.ts.
- "Recently opened" (`/recent`, `isRecentPath`) is the same wall over the artifacts the person opened in the viewer, newest open first (`recentlyOpened`, the order of `GET /api/artifacts?order=opened`): the same cards, rows, menus, search and grid/list choice, without the filter tabs or grouping. The viewer records each opening once (`viewer/useRecordOpen.ts`), also for a signed-out reader of a public link, which only counts in the usage (docs/usage.md); for a signed-in person it puts the answer into the shared list.
  Tests: Recent.test.tsx (opens from the rail item…, says what will show up…); viewer/RecordOpen.test.tsx; e2e/tests/gallery-cards.spec.ts (…finds bob's page first under Recently opened once she opened it).

- Author labels, initials and owner-group headings use `authorDisplayName` with account fallback. Search accepts both names and account IDs; ownership filters and group membership keep stable account identity. Identical display names never merge accounts. List rows keep their existing content.
  Tests: Gallery.test.tsx, filter.test.ts, group.test.ts; HTTP profile/ownership integration coverage.

- Legacy HTML snapshot routes remain for older clients with their original account/access/expiry boundaries; current cards use persistent image thumbnails and never a live fallback. Tests: internal/web/preview_cache_test.go, thumbnail_test.go.
