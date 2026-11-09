package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// withUsageReader names alice as the deployment's only usage reader.
func withUsageReader(cfg *Config) { cfg.UsageReaders = []string{"Alice"} }

// usage is the summary as alice's token reads it over HTTP.
func (h *harness) usage(query string) service.UsageSummary {
	h.t.Helper()
	res, body := h.do(nil, "GET", "/api/usage?"+query, "", map[string]string{"Authorization": "Bearer " + h.token("alice")})
	if res.StatusCode != http.StatusOK {
		h.t.Fatalf("usage: %d %s", res.StatusCode, body)
	}
	return decode[service.UsageSummary](h.t, body)
}

// usageTotal adds up the rows that match, so a test does not depend on which
// day or week it runs in.
func usageTotal(s service.UsageSummary, match func(service.UsageRow) bool) service.UsageRow {
	var t service.UsageRow
	for _, r := range s.Rows {
		if match == nil || match(r) {
			t.Opens += r.Opens
			t.AnonymousOpens += r.AnonymousOpens
			t.Publishes += r.Publishes
			t.WebPublishes += r.WebPublishes
			t.AgentPublishes += r.AgentPublishes
			t.ActivePeople += r.ActivePeople
			t.PublishingPeople += r.PublishingPeople
		}
	}
	return t
}

func TestOpeningTheSameArtifactThreeTimesInADayCountsThreeOpensAndOnePerson(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withUsageReader)
	a := h.publishShared(h.token("alice"), "Report", "<p>r")
	h.browser("alice", "pw-a")
	bob := h.browser("bob", "pw-b")
	for range 3 {
		if res, body := h.open(bob, a.ID); res.StatusCode != http.StatusOK {
			t.Fatalf("open: %d %s", res.StatusCode, body)
		}
	}

	got := usageTotal(h.usage("period=day&by=department"), func(r service.UsageRow) bool { return r.Department == service.UsageUnknownDepartment })
	if got.Opens != 3 || got.ActivePeople != 1 || got.Publishes != 0 {
		t.Errorf("bob's day = %+v, want 3 opens by 1 active person", got)
	}
}

func TestAnAnonymousOpenOfAPublicLinkIsCountedAsAnonymousAndKeepsNoName(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withUsageReader, withPublicLinks)
	a := h.publishAs(h.token("carol"), "Open", "<p>o", store.VisibilityPublic)
	if res, body := h.open(nil, a.ID); res.StatusCode != http.StatusOK {
		t.Fatalf("an anonymous open of a public link: %d %s", res.StatusCode, body)
	}
	private := h.publishAs(h.token("carol"), "Closed", "<p>c", store.VisibilityPrivate)
	if res, _ := h.open(nil, private.ID); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("an anonymous open of a private artifact: %d, want 401", res.StatusCode)
	}

	got := usageTotal(h.usage("by=department"), func(r service.UsageRow) bool { return r.Department == service.UsageAnonymous })
	if got.AnonymousOpens != 1 || got.Opens != 0 || got.ActivePeople != 0 {
		t.Errorf("anonymous = %+v, want 1 anonymous open and nobody active", got)
	}
	entries, err := h.st.Usage(t.Context(), "0000-01-01", "9999-12-31")
	if err != nil {
		t.Fatal(err)
	}
	opens := slices.DeleteFunc(entries, func(e store.UsageEntry) bool { return e.Action != store.UsageOpen })
	if len(opens) != 1 || opens[0].Username != "" || opens[0].Count != 1 {
		t.Errorf("the stored open = %+v, want one count with no username", opens)
	}
}

func TestADeletedArtifactsOpensAndPublishesStayInTheSummary(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withUsageReader)
	carol := h.browser("carol", "pw-c")
	a := h.publishShared(h.token("carol"), "Soon gone", "<p>g")
	h.open(carol, a.ID)
	before := usageTotal(h.usage(""), nil)
	if res, _ := h.do(carol, "DELETE", "/api/artifacts/"+a.ID, "", bridgeHeaders(h)); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", res.StatusCode)
	}

	if after := usageTotal(h.usage(""), nil); after != before || after.Opens != 1 || after.Publishes != 1 {
		t.Errorf("after the delete %+v, before %+v; want 1 open and 1 publish kept", after, before)
	}
}

func TestPublishesThroughAnAgentTokenAndThroughTheWebAreCountedApart(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withUsageReader)
	carol := h.browser("carol", "pw-c")
	if res, body := h.do(carol, "POST", "/api/artifacts", `{"title":"By hand","html":"<p>w"}`, bridgeHeaders(h)); res.StatusCode != http.StatusCreated {
		t.Fatalf("web publish: %d %s", res.StatusCode, body)
	}
	agent := h.agent(h.token("carol"))
	id := agent.ok("publish", map[string]any{"title": "By agent", "content": "<p>a"})["id"].(string)
	agent.ok("publish", map[string]any{"id": id, "title": "By agent", "content": "<p>b"})

	got := usageTotal(h.usage("by=department,kind"), func(r service.UsageRow) bool { return r.Department == "Operations" && r.Kind == "html" })
	if got.WebPublishes != 1 || got.AgentPublishes != 2 || got.Publishes != 3 || got.PublishingPeople != 1 {
		t.Errorf("carol's publishes = %+v, want 1 through the web and 2 through an agent, by 1 person", got)
	}
}

func TestTheSummaryHoldsCountsPerPeriodDepartmentAndKindAndNoAccountOrArtifact(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withUsageReader)
	h.browser("carol", "pw-c")
	a := h.publishShared(h.token("carol"), "Report", "<p>r")
	h.open(h.browser("bob", "pw-b"), a.ID)
	h.open(h.browser("alice", "pw-a"), a.ID)

	res, body := h.do(nil, "GET", "/api/usage?by=department,kind&period=day", "", map[string]string{"Authorization": "Bearer " + h.token("alice")})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("usage: %d %s", res.StatusCode, body)
	}
	for _, secret := range []string{"alice", "bob", "carol", a.ID, "Report"} {
		if strings.Contains(body, secret) {
			t.Errorf("the summary names %q: %s", secret, body)
		}
	}
	var departments []string
	for _, r := range decode[service.UsageSummary](t, body).Rows {
		if r.Period == "" || r.Kind != "html" {
			t.Errorf("a row without its period or kind: %+v", r)
		}
		departments = append(departments, r.Department)
	}
	slices.Sort(departments)
	if departments = slices.Compact(departments); !slices.Equal(departments, []string{"Engineering", "Operations", service.UsageUnknownDepartment}) {
		t.Errorf("departments = %v", departments)
	}
}

func TestASignInStoresTheDepartmentADReportsAndUnknownWhenItReportsNone(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withUsageReader)
	a := h.publishShared(h.token("carol"), "Report", "<p>r")
	h.open(h.browser("alice", "pw-a"), a.ID)
	h.open(h.browser("bob", "pw-b"), a.ID)

	opensBy := map[string]int{}
	for _, r := range h.usage("by=department").Rows {
		opensBy[r.Department] += r.Opens
	}
	if opensBy["Engineering"] != 1 || opensBy[service.UsageUnknownDepartment] != 1 {
		t.Errorf("opens by department = %v, want alice's under Engineering and bob's under unknown", opensBy)
	}

	h.dir.mu.Lock()
	h.dir.accounts["bob"].department = "Quality"
	h.dir.mu.Unlock()
	h.browser("bob", "pw-b")
	opensBy = map[string]int{}
	for _, r := range h.usage("by=department").Rows {
		opensBy[r.Department] += r.Opens
	}
	if opensBy["Quality"] != 1 || opensBy[service.UsageUnknownDepartment] != 0 {
		t.Errorf("after bob signs in with a department: %v, want his open under Quality", opensBy)
	}
}

func TestOnlyTheNamedUsageReadersMayReadTheSummaryAndBadQueriesSayWhatToSend(t *testing.T) {
	t.Parallel()
	nobody := newHarness(t)
	if res, _ := nobody.do(nil, "GET", "/api/usage", "", map[string]string{"Authorization": "Bearer " + nobody.token("alice")}); res.StatusCode != http.StatusForbidden {
		t.Errorf("with no readers named, an author's agent: %d, want 403", res.StatusCode)
	}

	h := newHarness(t, withUsageReader)
	if res, _ := h.do(h.browser("bob", "pw-b"), "GET", "/api/usage", "", bridgeHeaders(h)); res.StatusCode != http.StatusForbidden {
		t.Errorf("bob in his browser: %d, want 403", res.StatusCode)
	}
	if res, _ := h.do(h.browser("alice", "pw-a"), "GET", "/api/usage", "", bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Errorf("alice in her browser: %d, want 200", res.StatusCode)
	}
	if res, _ := h.do(nil, "GET", "/api/usage", "", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("nobody signed in: %d, want 401", res.StatusCode)
	}
	today := time.Now().UTC()
	for query, want := range map[string]string{
		"from=yesterday":                "YYYY-MM-DD",
		"period=month":                  "week",
		"by=author":                     "department",
		"from=2026-02-01&to=2026-01-01": "after",
		"from=" + today.AddDate(-2, 0, 0).Format(time.DateOnly): "400 days",
	} {
		res, body := h.do(nil, "GET", "/api/usage?"+query, "", map[string]string{"Authorization": "Bearer " + h.token("alice")})
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, want) {
			t.Errorf("%s: %d %s, want 400 saying %s", query, res.StatusCode, body, want)
		}
	}
}
