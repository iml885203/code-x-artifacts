package mcptools

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type activityIn struct {
	TokenID string `json:"tokenID,omitempty" jsonschema:"Filter by one of your stable nonsecret token IDs; omit for all your tokens"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 100; defaults to 50"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"Opaque nextCursor returned by a previous page with the same token filter"`
}

func (t *tools) registerActivity(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "agent_activity", Description: `Read your own recent agent connections and successful content edits on Connect. History is limited to 30 days and 1000 rows per account. Client names and versions are self-reported, not verified identities; a stateless unnamed request stays unknown. Last connected means a successful MCP initialize; last used can include HTTP requests and is throttled. An unavailable or deleted artifact has no target metadata. Reading this tool creates no activity or use summary. No credential or token hash is returned.`, Annotations: readOnly("Read my agent activity")}, func(ctx context.Context, _ *mcp.CallToolRequest, in activityIn) (*mcp.CallToolResult, service.AgentActivityPage, error) {
		out, err := t.e.svc.AgentActivity(ctx, t.viewer(ctx), service.ActivityQuery{TokenID: in.TokenID, Limit: in.Limit, Cursor: in.Cursor})
		if err != nil {
			return nil, out, t.fail("agent_activity", err)
		}
		return nil, out, nil
	})
}

// observeClient runs after the SDK has parsed a protocol request; payload text cannot impersonate client metadata.
func (e *Endpoint) observeClient(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		c, _ := ctx.Value(callerKey{}).(Caller)
		p := c.Provenance
		params := presentParams(req.GetParams())
		info := requestClient(method, params)
		p.Client = ClientName(info.Name)
		p.ClientVersion = ""
		if p.Client != "" {
			p.ClientVersion = store.BoundedLabel(info.Version, 40)
		}
		c.Client, c.Provenance = p.Client, p
		ctx = WithCaller(service.WithTokenProvenance(ctx, p), c)
		result, err := next(ctx, method, req)
		if shouldRecordActivity(params, result, err) && p.TokenID != "" {
			if saveErr := e.recordClientUse(ctx, method, p); saveErr != nil {
				e.log.Error("record agent activity", "err", saveErr)
			}
		}
		return result, err
	}
}

// presentParams turns the typed-nil params the SDK passes for an omitted or null params object into a nil
// interface, so readers below never call a method on a nil pointer.
func presentParams(params mcp.Params) mcp.Params {
	if params == nil {
		return nil
	}
	if v := reflect.ValueOf(params); v.Kind() == reflect.Pointer && v.IsNil() {
		return nil
	}
	return params
}

type reportedClient struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func requestClient(method string, params mcp.Params) reportedClient {
	if params == nil {
		return reportedClient{}
	}
	if method == "initialize" {
		var in struct {
			ClientInfo reportedClient `json:"clientInfo"`
		}
		if data, err := json.Marshal(params); err == nil && json.Unmarshal(data, &in) == nil {
			return in.ClientInfo
		}
		return reportedClient{}
	}
	if method != "tools/call" && method != "tools/list" {
		return reportedClient{}
	}
	raw, _ := params.GetMeta()["io.modelcontextprotocol/clientInfo"].(map[string]any)
	name, _ := raw["name"].(string)
	version, _ := raw["version"].(string)
	return reportedClient{Name: name, Version: version}
}

func shouldRecordActivity(params mcp.Params, result mcp.Result, err error) bool {
	if err != nil {
		return false
	}
	if tool, ok := result.(*mcp.CallToolResult); ok && tool.IsError {
		return false
	}
	if tool, ok := params.(*mcp.CallToolParamsRaw); ok && tool.Name == "agent_activity" {
		return false
	}
	return true
}

func (e *Endpoint) recordClientUse(ctx context.Context, method string, p store.TokenProvenance) error {
	switch method {
	case "initialize":
		return e.svc.RecordAgentConnection(ctx, p, time.Now())
	case "tools/call", "tools/list":
		return e.svc.RecordAgentUse(ctx, p, time.Now())
	default:
		return nil
	}
}
