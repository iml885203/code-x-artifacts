package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func previewMeta(t *testing.T, h *harness, c *http.Client, id string) previewIdentity {
	t.Helper()
	res, body := h.do(c, "GET", "/api/artifacts/"+id+"/preview?w=640&h=400", "", bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("preview metadata: %d %s", res.StatusCode, body)
	}
	return decode[previewIdentity](t, body)
}

func storePreview(t *testing.T, h *harness, c *http.Client, id string, meta previewIdentity, html string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"key": meta.Key, "html": html})
	if err != nil {
		t.Fatal(err)
	}
	res, out := h.do(c, "POST", "/api/artifacts/"+id+"/preview?w=640&h=400", string(body), bridgeHeaders(h))
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("save preview: %d %s", res.StatusCode, out)
	}
}

func TestReadyPreviewsArePrivateReusableAndInertWhileTheirDependenciesMatch(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Cached page", page)
	alice := h.browser("alice", "pw-a")
	bob := h.browser("bob", "pw-b")
	cold := previewMeta(t, h, alice, a.ID)
	if cold.Key == "" || cold.URL != "" || cold.Version != 1 {
		t.Fatalf("cold = %+v", cold)
	}
	snapshot := `<!doctype html><p>Already rendered</p><script>throw Error("must never run")</script>`
	storePreview(t, h, alice, a.ID, cold, snapshot)
	warm := previewMeta(t, h, alice, a.ID)
	if warm.Key != cold.Key || warm.URL == "" {
		t.Fatalf("warm = %+v", warm)
	}
	res, body := h.do(alice, "GET", warm.URL, "", nil)
	assertInertSnapshot(t, res, body, snapshot)
	other := previewMeta(t, h, bob, a.ID)
	if other.Key == warm.Key || other.URL != "" {
		t.Fatalf("other account shared cache: %+v", other)
	}
	res, _ = h.do(bob, "GET", warm.URL, "", nil)
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/raw/"+a.ID+"?preview" {
		t.Fatalf("foreign snapshot: %d", res.StatusCode)
	}
	res, _ = h.do(nil, "GET", warm.URL, "", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous snapshot: %d", res.StatusCode)
	}
}

func assertInertSnapshot(t *testing.T, res response, body, snapshot string) {
	t.Helper()
	if res.StatusCode != http.StatusOK || body != snapshot || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("snapshot response: %d %q", res.StatusCode, body)
	}
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'none'") || strings.Contains(csp, "allow-scripts") || strings.Contains(csp, "allow-same-origin") || !strings.HasPrefix(csp, "sandbox;") {
		t.Fatalf("snapshot CSP: %s", csp)
	}
}

func TestAssetChangesAndGrantRotationInvalidateUnchangedVersionPreviews(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	h := newHarness(t, func(cfg *Config) { cfg.Clock = clock })
	token := h.token("alice")
	a := h.publish(token, "Asset preview", page)
	alice := h.browser("alice", "pw-a")
	initial := previewMeta(t, h, alice, a.ID)
	storePreview(t, h, alice, a.ID, initial, "<p>Before asset</p>")
	h.mustUpload(token, a.ID, "picture.png", pngBytes)
	afterAsset := previewMeta(t, h, alice, a.ID)
	if afterAsset.Key == initial.Key || afterAsset.Version != initial.Version || afterAsset.URL != "" {
		t.Fatalf("asset did not invalidate: %+v", afterAsset)
	}
	storePreview(t, h, alice, a.ID, afterAsset, "<p>After asset</p>")
	h.mustUpload(token, a.ID, "picture.png", pngBytes+"replacement")
	replaced := previewMeta(t, h, alice, a.ID)
	if replaced.Key == afterAsset.Key || replaced.Version != initial.Version {
		t.Fatal("replacement did not invalidate unchanged version")
	}
	res, _ := h.do(nil, "DELETE", "/api/artifacts/"+a.ID+"/assets/picture.png", "", map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete asset: %d", res.StatusCode)
	}
	deleted := previewMeta(t, h, alice, a.ID)
	if deleted.Key == replaced.Key || deleted.Version != initial.Version {
		t.Fatal("asset deletion did not invalidate unchanged version")
	}
	storePreview(t, h, alice, a.ID, deleted, "<p>After deletion</p>")
	clock.Advance(time.Hour)
	afterHour := previewMeta(t, h, alice, a.ID)
	if afterHour.Key == afterAsset.Key || afterHour.URL != "" {
		t.Fatalf("grant rotation did not invalidate: %+v", afterHour)
	}
	res, _ = h.do(alice, "GET", "/preview/"+a.ID+"?w=640&h=400&key="+afterAsset.Key, "", nil)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("expired cached URL: %d", res.StatusCode)
	}
}

func TestPreviewWritesRefuseStaleKeysInvalidDimensionsAndNonBrowserRequests(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token := h.token("alice")
	a := h.publish(token, "Refused preview", page)
	alice := h.browser("alice", "pw-a")
	meta := previewMeta(t, h, alice, a.ID)
	path := "/api/artifacts/" + a.ID + "/preview?w=640&h=400"
	for _, tc := range []struct {
		path, body string
		client     *http.Client
		headers    map[string]string
		status     int
	}{
		{path, `{"key":"stale","html":"<p>x</p>"}`, alice, bridgeHeaders(h), 409},
		{"/api/artifacts/" + a.ID + "/preview?w=0&h=400", "{}", alice, bridgeHeaders(h), 400},
		{path, fmt.Sprintf(`{"key":%q,"html":""}`, meta.Key), alice, bridgeHeaders(h), 413},
		{path, "{}", nil, map[string]string{"Authorization": "Bearer " + token}, 401},
		{path, "{}", alice, map[string]string{"Origin": "null", "X-Pictor": "1"}, 403},
	} {
		res, _ := h.do(tc.client, "POST", tc.path, tc.body, tc.headers)
		if res.StatusCode != tc.status {
			t.Errorf("refused preview %s = %d, want %d", tc.path, res.StatusCode, tc.status)
		}
	}
}

func TestPreviewSnapshotsCannotOutliveDeletionOrLossOfAccess(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token := h.token("alice")
	a := h.publishShared(token, "Revocable preview", page)
	alice := h.browser("alice", "pw-a")
	bob := h.browser("bob", "pw-b")
	meta := previewMeta(t, h, bob, a.ID)
	storePreview(t, h, bob, a.ID, meta, "<p>Private to bob cache</p>")
	url := previewMeta(t, h, bob, a.ID).URL
	res, _ := h.do(alice, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"private"}`, bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set private = %d", res.StatusCode)
	}
	res, _ = h.do(bob, "GET", url, "", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked snapshot = %d", res.StatusCode)
	}
	res, _ = h.do(alice, "DELETE", "/api/artifacts/"+a.ID, "", bridgeHeaders(h))
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d", res.StatusCode)
	}
	res, _ = h.do(bob, "GET", url, "", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted snapshot = %d", res.StatusCode)
	}
}

func TestPreviewCacheEvictsOldestEntriesAndRejectsOversizedSnapshots(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 11, 12, 5, 0, 0, time.UTC)
	cache := newPreviewCache(func() time.Time { return now })
	for i := range maxPreviewEntries {
		cache.put(strconv.Itoa(i), "<p>ready</p>", now.Add(time.Hour))
	}
	if _, ok := cache.get("0"); !ok {
		t.Fatal("recently used entry missing")
	}
	cache.put("new", "<p>ready</p>", now.Add(time.Hour))
	if _, ok := cache.get("1"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if _, ok := cache.get("0"); !ok {
		t.Fatal("recently used entry was evicted")
	}
	if cache.put("oversized", strings.Repeat("x", maxPreviewBytes+1), now.Add(time.Hour)) {
		t.Fatal("oversized entry accepted")
	}
	now = now.Add(time.Hour)
	if _, ok := cache.get("0"); ok {
		t.Fatal("expired entry returned")
	}
}

func TestPublishedVersionsAndBrowserLogoutCannotReuseOldPreviewSnapshots(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token := h.token("alice")
	a := h.publish(token, "First title", page)
	alice := h.browser("alice", "pw-a")
	initial := previewMeta(t, h, alice, a.ID)
	storePreview(t, h, alice, a.ID, initial, "<p>First preview</p>")
	oldURL := previewMeta(t, h, alice, a.ID).URL
	res, _ := h.do(alice, "PUT", "/api/artifacts/"+a.ID+"/title", `{"title":"Second title"}`, bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("rename = %d", res.StatusCode)
	}
	updated := previewMeta(t, h, alice, a.ID)
	if updated.Key == initial.Key || updated.Version != initial.Version+1 || updated.URL != "" {
		t.Fatalf("version key not refreshed: %+v", updated)
	}
	res, _ = h.do(alice, "GET", oldURL, "", nil)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("old version delivery = %d", res.StatusCode)
	}
	body, _ := json.Marshal(map[string]string{"key": initial.Key, "html": "<p>Stale upload</p>"})
	res, _ = h.do(alice, "POST", "/api/artifacts/"+a.ID+"/preview?w=640&h=400", string(body), bridgeHeaders(h))
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("stale upload = %d", res.StatusCode)
	}
	res, _ = h.do(alice, "POST", "/logout", "", bridgeHeaders(h))
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout = %d", res.StatusCode)
	}
	res, _ = h.do(alice, "GET", oldURL, "", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logged-out snapshot = %d", res.StatusCode)
	}
}

func TestPreviewCacheEvictsAtItsTotalByteBudget(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cache := newPreviewCache(func() time.Time { return now })
	large := strings.Repeat("x", maxPreviewBytes)
	for i := range maxPreviewCacheBytes / maxPreviewBytes {
		cache.put(strconv.Itoa(i), large, now.Add(time.Hour))
	}
	cache.put("next", large, now.Add(time.Hour))
	if _, ok := cache.get("0"); ok {
		t.Fatal("oldest entry survived byte-budget eviction")
	}
	if _, ok := cache.get("next"); !ok {
		t.Fatal("new entry missing")
	}
}

func TestPreviewEnvironmentHintsAndOriginSelectIndependentCacheEntries(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publish(h.token("alice"), "Environment preview", page)
	alice := h.browser("alice", "pw-a")
	path := "/api/artifacts/" + a.ID + "/preview?w=640&h=400"
	contexts := []string{
		"&scheme=light&motion=no-preference&lang=en",
		"&scheme=dark&motion=no-preference&lang=en",
		"&scheme=light&motion=reduce&lang=en",
		"&scheme=light&motion=no-preference&lang=zh-Hant",
		"&scheme=light&motion=no-preference&lang=en&dpr=2",
	}
	keys := make(map[string]bool)
	for _, context := range contexts {
		res, body := h.do(alice, "GET", path+context, "", bridgeHeaders(h))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("context metadata = %d", res.StatusCode)
		}
		key := decode[previewIdentity](t, body).Key
		if keys[key] {
			t.Fatal("environment contexts reused a cache identity")
		}
		keys[key] = true
	}
	otherOrigin := newHarnessOn(t, h.st)
	res, body := otherOrigin.do(alice, "GET", path+contexts[0], "", bridgeHeaders(otherOrigin))
	if res.StatusCode != http.StatusOK || keys[decode[previewIdentity](t, body).Key] {
		t.Fatalf("origin reused cache identity: %d", res.StatusCode)
	}
}

func TestPreviewEnvironmentHintsRejectUnsupportedOrUnboundedValues(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publish(h.token("alice"), "Bounded context", page)
	alice := h.browser("alice", "pw-a")
	for _, context := range []string{"&dpr=", "&dpr=0.1", "&dpr=5", "&dpr=NaN", "&dpr=+Inf", "&scheme=", "&motion=", "&lang=", "&scheme=auto", "&motion=fast", "&lang=en%2FUS", "&lang=" + strings.Repeat("a", 65)} {
		res, _ := h.do(alice, "GET", "/api/artifacts/"+a.ID+"/preview?w=640&h=400"+context, "", bridgeHeaders(h))
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid context %q = %d", context, res.StatusCode)
		}
	}
}
