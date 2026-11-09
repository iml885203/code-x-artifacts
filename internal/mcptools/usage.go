package mcptools

import (
	"context"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const usageDoc = `usage_summary counts how often artifacts were opened in the viewer and published, per week or per day, optionally per
department and per artifact kind, so the person running this deployment can show how much it is used. Only the accounts
the deployment names as usage readers may call it; everyone else is refused. It returns counts only, never an account
or an artifact. Each row has: period (its first day; a Monday for a week), department and kind when grouped by them,
opens (by people signed in), anonymousOpens (public-link readers who were not signed in), publishes (new versions, split
into webPublishes and agentPublishes), activePeople (distinct people who opened or published) and publishingPeople.
The department is the one Active Directory gave the person at their latest sign-in: "unknown" when it gave none, and
"anonymous" groups the anonymous opens. Days are UTC and counts are kept for 400 days. To share the numbers, publish
them as an artifact.`

type usageIn struct {
	From   string   `json:"from,omitempty" jsonschema:"First day to count, YYYY-MM-DD (UTC); defaults to 12 weeks before to"`
	To     string   `json:"to,omitempty" jsonschema:"Last day to count, YYYY-MM-DD (UTC), included; defaults to today. At most 400 days after from"`
	Period string   `json:"period,omitempty" jsonschema:"week (the default; weeks start on Monday) or day"`
	By     []string `json:"by,omitempty" jsonschema:"Group each period by department, kind, both, or neither (omit) for one total per period"`
}

func (t *tools) registerUsage(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "usage_summary", Description: usageDoc, Annotations: readOnly("Read usage counts")}, func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, service.UsageSummary, error) {
		out, err := t.e.svc.Usage(ctx, t.viewer(ctx), service.UsageQuery(in))
		if err != nil {
			return nil, out, t.fail("usage_summary", err)
		}
		return nil, out, nil
	})
}
