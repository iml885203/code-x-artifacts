package mcptools

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

// asUser plays the web layer: the bearer value is the username, and the
// request carries the Caller built from it.
func asUser(svc *service.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		c := Caller{Viewer: svc.Viewer(user), BaseURL: "https://" + user + ".example", Client: user + "-client"}
		next.ServeHTTP(w, r.WithContext(WithCaller(r.Context(), c)))
	})
}

type bearer struct{ user string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.user)
	return http.DefaultTransport.RoundTrip(r)
}

func newEndpoint(t testing.TB, options ...func(*Config)) (*Endpoint, *httptest.Server) {
	t.Helper()
	st := storetest.Open(t)
	disk, err := assets.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.New(st, disk, false)
	cfg := Config{Service: svc, Logger: log}
	for _, option := range options {
		option(&cfg)
	}
	e := New(cfg)
	srv := httptest.NewServer(asUser(svc, e.Handler()))
	t.Cleanup(srv.Close)
	return e, srv
}

func connect(t testing.TB, srv *httptest.Server, user string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	s, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL, HTTPClient: &http.Client{Transport: bearer{user}},
	}, nil)
	if err != nil {
		t.Fatalf("connect %s: %v", user, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func call(t testing.TB, s *mcp.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("%s: err=%v result=%+v", tool, err, res)
	}
	out, _ := res.StructuredContent.(map[string]any)
	return out
}

func TestTwoCallersThroughOneEndpointOnlyEverSeeTheirOwn(t *testing.T) {
	t.Parallel()
	_, srv := newEndpoint(t)
	users := []string{"alice", "bob"}
	ids := map[string]string{}
	for _, u := range users {
		out := call(t, connect(t, srv, u), "publish", map[string]any{"title": "Page of " + u, "content": "<p>" + u + "</p>"})
		ids[u] = out["id"].(string)
	}

	// Few clients and rounds are enough: what matters is that calls of the two
	// callers are in flight together, so they all start at the barrier.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, u := range users {
		other := "bob"
		if u == "bob" {
			other = "alice"
		}
		for range 2 {
			wg.Add(1)
			s := connect(t, srv, u)
			go func() {
				defer wg.Done()
				<-start
				for range 3 {
					if me := call(t, s, "whoami", nil); me["username"] != u {
						t.Errorf("%s was told they are %v", u, me["username"])
					}
					read := call(t, s, "read", map[string]any{"id": ids[u]})
					if read["role"] != "author" || !strings.HasPrefix(read["url"].(string), "https://"+u+".example/") {
						t.Errorf("%s read their own page as %v at %v", u, read["role"], read["url"])
					}
					res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "read", Arguments: map[string]any{"id": ids[other]}})
					if err != nil || !res.IsError {
						t.Errorf("%s read %s's private page: %v %+v", u, other, err, res)
					}
					list := call(t, s, "list", nil)
					for _, a := range list["artifacts"].([]any) {
						if a.(map[string]any)["id"] != ids[u] {
							t.Errorf("%s was listed %v", u, a)
						}
					}
				}
			}()
		}
	}
	close(start)
	wg.Wait()
}

func TestARequestDoesNotRegisterTheToolsAgain(t *testing.T) {
	t.Parallel()
	e, srv := newEndpoint(t)
	s := connect(t, srv, "alice")
	for range 5 {
		call(t, s, "whoami", nil)
	}
	if got := e.builds.Load(); got != 1 {
		t.Errorf("the server was built %d times for 5 calls, want once for the endpoint", got)
	}
}

func BenchmarkAnMCPCall(b *testing.B) {
	_, srv := newEndpoint(b)
	s := connect(b, srv, "alice")
	b.ResetTimer()
	for b.Loop() {
		call(b, s, "whoami", nil)
	}
}

func TestAStandaloneMCPEndpointWritesItsOwnPublishAudit(t *testing.T) {
	t.Parallel()
	sink, err := os.CreateTemp(t.TempDir(), "operations-*.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	_, srv := newEndpoint(t, func(c *Config) { c.Logger = slog.New(slog.NewTextHandler(sink, nil)) })
	out := call(t, connect(t, srv, "alice"), "publish", map[string]any{"title": "Report", "content": "<p>hello"})
	if _, err := sink.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(sink)
	if err != nil {
		t.Fatal(err)
	}
	want := `level=INFO msg="artifact created" id=` + out["id"].(string) + ` user=alice`
	if strings.Count(string(raw), want) != 1 {
		t.Fatalf("standalone MCP lost or duplicated its operation audit: %s", raw)
	}
}
