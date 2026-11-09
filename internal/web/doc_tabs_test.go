package web

import (
	"net/http"
	"strings"
	"testing"
)

// tabsOf is the tabs a read of the doc as blocks lists over HTTP: id, title, block count.
func (h *harness) tabsOf(tok, id string) []map[string]any {
	h.t.Helper()
	_, body := api(h, "GET", "/api/artifacts/"+id+"/blocks", "", tok)
	var out []map[string]any
	for _, tab := range decode[map[string]any](h.t, body)["tabs"].([]any) {
		out = append(out, tab.(map[string]any))
	}
	return out
}

func tabOpsFor(tab string) string {
	return `[{"op":"tab_add","title":"Notes"},{"op":"tab_rename","tab":"` + tab + `","title":"Plan"},{"op":"tab_move","tab":"` + tab + `","at":"end"}]`
}

func TestAReadListsEachTabWithItsIdAndTitleInBothViews(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	id := h.editableDoc(tok)

	read := agent.ok("read", map[string]any{"id": id})
	tabs, _ := read["tabs"].([]any)
	if len(tabs) != 1 {
		t.Fatalf("the Markdown read lists the tabs: %v", read["tabs"])
	}
	tab := tabs[0].(map[string]any)
	if tab["title"] != "Document" || tab["id"] == "" || tab["blocks"] != nil {
		t.Errorf("a tab in the Markdown read has an id and a title and no blocks: %v", tab)
	}
	blocks := agent.ok("read", map[string]any{"id": id, "view": "blocks"})
	if got := blocks["tabs"].([]any)[0].(map[string]any); got["id"] != tab["id"] || len(got["blocks"].([]any)) != 3 {
		t.Errorf("the blocks read: %v", got)
	}
}

func TestTabOpsGiveTheSameResultOverHTTPAndMCPAndTheNextReadHasTheNewOrder(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	viaHTTP, viaMCP := h.editableDoc(tok), h.editableDoc(tok)
	firstHTTP := h.tabsOf(tok, viaHTTP)[0]["id"].(string)
	firstMCP := h.tabsOf(tok, viaMCP)[0]["id"].(string)

	res, body := api(h, "POST", "/api/artifacts/"+viaHTTP+"/edit", `{"ops":`+tabOpsFor(firstHTTP)+`}`, tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("tab ops over HTTP: %d %s", res.StatusCode, body)
	}
	var ops []any
	if err := jsonUnmarshal(tabOpsFor(firstMCP), &ops); err != nil {
		t.Fatal(err)
	}
	want := agent.ok("doc_edit", map[string]any{"id": viaMCP, "ops": ops})
	// The two docs have their own tab ids, so what is compared is what does not depend on them.
	for name, got := range map[string]map[string]any{"http": decode[map[string]any](t, body), "mcp": want} {
		tabs := got["tabs"].([]any)
		if len(tabs) != 2 || tabs[0].(map[string]any)["title"] != "Notes" || tabs[1].(map[string]any)["title"] != "Plan" {
			t.Errorf("%s: the result lists the tabs in their new order: %v", name, got["tabs"])
		}
	}

	read := h.tabsOf(tok, viaMCP)
	if len(read) != 2 || read[0]["title"] != "Notes" || read[0]["id"] != want["tabs"].([]any)[0].(map[string]any)["id"] {
		t.Fatalf("the next read: %v", read)
	}
	if first := read[0]["blocks"].([]any); len(first) != 1 || first[0].(map[string]any)["type"] != "heading" {
		t.Errorf("a new tab starts with an empty heading: %v", first)
	}
}

func TestARefusedTabOpIsTheSameOverHTTPAndMCPAndStoresNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	viaHTTP, viaMCP := h.editableDoc(tok), h.editableDoc(tok)

	bad := `[{"op":"tab_delete","tab":"nope"}]`
	res, body := api(h, "POST", "/api/artifacts/"+viaHTTP+"/edit", `{"ops":`+bad+`}`, tok)
	var badValue []any
	_ = jsonUnmarshal(bad, &badValue)
	refusal := agent.refused("doc_edit", map[string]any{"id": viaMCP, "ops": badValue})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, `no tab \"nope\"`) || !strings.Contains(refusal, `no tab "nope"`) {
		t.Errorf("refusals: http %d %s, mcp %s", res.StatusCode, body, refusal)
	}

	// Adding a tab and then deleting every tab is all or none: the added tab is not kept.
	only := h.tabsOf(tok, viaHTTP)[0]["id"].(string)
	last := `[{"op":"tab_add","title":"Extra"},{"op":"tab_delete","tab":"` + only + `"},{"op":"tab_delete","tab":"nope2"}]`
	if res, body := api(h, "POST", "/api/artifacts/"+viaHTTP+"/edit", `{"ops":`+last+`}`, tok); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a call with a bad op: %d %s", res.StatusCode, body)
	}
	if tabs := h.tabsOf(tok, viaHTTP); len(tabs) != 1 || tabs[0]["id"] != only {
		t.Errorf("a refused call stored something: %v", tabs)
	}
	oneMore := `[{"op":"tab_delete","tab":"` + only + `"}]`
	if res, body := api(h, "POST", "/api/artifacts/"+viaHTTP+"/edit", `{"ops":`+oneMore+`}`, tok); res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "last tab") {
		t.Errorf("deleting the only tab: %d %s", res.StatusCode, body)
	}
}
