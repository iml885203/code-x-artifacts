package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

const dbIntro = `Every artifact has a small shared database: collections of JSON documents that belong to that artifact and are
shared by everyone who can open it (the page itself reads and writes them through window.pictor.db). `

const dbGetDoc = dbIntro + `db_get reads one document by collection and doc_id. Returns its data, who wrote it last and when.
Anyone who can see the artifact can read it.`

const dbListDoc = dbIntro + `db_list returns every document in one collection (collection names are chosen by the page; read the page's
content to find them). Use it to look at what people have entered, such as votes or sign-ups.`

const dbVersionDoc = `Every document has a version: 1 when created, one more with each write. Name the version you built your change on as ifVersion
and the write is refused, changing nothing, when the document has moved on (ifVersion 0 means it must not exist yet). The refusal
carries the document as it is now, so merge and send again. Leave ifVersion out to write regardless.`

const dbQueryDoc = dbIntro + `db_query returns the documents of one collection that match a filter (same shape as db_list, with each document's
version), so you do not have to read everything. filter is a JSON object. Each key is a field of the document's data ("status", or
"owner.name" for a field inside a field) or "id" for the document id; its value is either a plain value (the field must equal it) or
an object of operators: {"in": [..]}, {"gt"|"gte"|"lt"|"lte": number or string}, {"prefix": "text"}, {"eq": any JSON, including an
object}. Everything given must hold. A missing field matches nothing. Examples: {"status": "open"}, {"score": {"gte": 3, "lt": 10}},
{"id": {"prefix": "2026-10-"}}, {"team": {"in": ["a", "b"]}}. Leave filter out for every document. limit caps how many come back
(oldest write first); 0 or absent is all. Anyone who can see the artifact can read it.`

const dbUpdateDoc = dbIntro + `db_update changes some fields of an existing document and leaves the rest: fields is a JSON object merged into the
document (objects merge field by field; arrays and values are replaced; a field set to null is removed). The document must exist and
be an object; use db_set to create one. Recorded as written by you, and anyone who can see the artifact may write. ` + dbVersionDoc

const dbBatchDoc = dbIntro + `db_batch applies several writes together: all of them or none. ops is a list (at most 100, 4 MB of data in all), each
{"op": "set" | "update" | "delete", "collection", "id" (the document id), "data" (for set: the whole value) or "fields" (for update: the
fields to merge, as in db_update), "ifVersion" (optional)}. They run in order and each sees the ones before it. If any is refused,
because its ifVersion is stale, an update or delete has no document, or something is invalid, nothing at all is changed and the
refusal names which operation (counting from 1). The result has one entry per operation, in order: the document as written with its
new version, or for a delete just its id. ` + dbVersionDoc

const dbSetDoc = dbIntro + `db_set creates or replaces one document, recorded as written by you. Anyone who can see the artifact may write,
not just its author, and it replaces the whole document, so read it first when you only mean to change a field (or use db_update).
data: any JSON value, at most 1 MB. Send the value itself (an object, array, string, number), not a string that
contains JSON. ` + dbVersionDoc

const dbDeleteDoc = dbIntro + `db_delete removes one document. It cannot be undone and takes effect for everyone at once, so use it only
when the person asked. ` + dbVersionDoc

type dbDocIn struct {
	ID         string `json:"id" jsonschema:"the artifact id"`
	Collection string `json:"collection" jsonschema:"collection name, chosen by the page"`
	Doc        string `json:"doc_id" jsonschema:"document id within the collection"`
}

type dbDeleteIn struct {
	dbDocIn
	IfVersion *int `json:"ifVersion,omitempty" jsonschema:"only delete if the document is at this version"`
}

type dbListIn struct {
	ID         string `json:"id" jsonschema:"the artifact id"`
	Collection string `json:"collection" jsonschema:"collection name, chosen by the page"`
}

type dbSetIn struct {
	ID         string `json:"id" jsonschema:"the artifact id"`
	Collection string `json:"collection" jsonschema:"collection name, chosen by the page"`
	Doc        string `json:"doc_id" jsonschema:"document id within the collection"`
	Data       any    `json:"data" jsonschema:"the JSON value to store, at most 1 MB"`
	IfVersion  *int   `json:"ifVersion,omitempty" jsonschema:"only write if the document is at this version (0: only if it does not exist)"`
}

type dbQueryIn struct {
	ID         string `json:"id" jsonschema:"the artifact id"`
	Collection string `json:"collection" jsonschema:"collection name, chosen by the page"`
	Filter     any    `json:"filter,omitempty" jsonschema:"a JSON object, e.g. {\"status\": \"open\", \"score\": {\"gte\": 3}}; omit for every document"`
	Limit      int    `json:"limit,omitempty" jsonschema:"at most this many documents, oldest write first; 0 or absent is all"`
}

type dbUpdateIn struct {
	ID         string `json:"id" jsonschema:"the artifact id"`
	Collection string `json:"collection" jsonschema:"collection name, chosen by the page"`
	Doc        string `json:"doc_id" jsonschema:"document id within the collection"`
	Fields     any    `json:"fields" jsonschema:"a JSON object merged into the document; a field set to null is removed"`
	IfVersion  *int   `json:"ifVersion,omitempty" jsonschema:"only write if the document is at this version"`
}

type dbBatchOp struct {
	Op         string `json:"op" jsonschema:"set, update or delete"`
	Collection string `json:"collection" jsonschema:"collection name"`
	ID         string `json:"id" jsonschema:"the document id within the collection"`
	Data       any    `json:"data,omitempty" jsonschema:"set: the whole JSON value to store"`
	Fields     any    `json:"fields,omitempty" jsonschema:"update: a JSON object merged into the document; null removes a field"`
	IfVersion  *int   `json:"ifVersion,omitempty" jsonschema:"only apply if the document is at this version (0: only if it does not exist)"`
}

type dbBatchIn struct {
	ID  string      `json:"id" jsonschema:"the artifact id"`
	Ops []dbBatchOp `json:"ops" jsonschema:"the writes to apply together, in order; all or none"`
}

// docOut is the HTTP API's document, with the data as the JSON value it holds.
type docOut struct {
	ID        string    `json:"id"`
	Data      any       `json:"data"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
	Version   int       `json:"version"`
}

func docValue(d store.Doc) (docOut, error) {
	var data any
	if d.Data != nil {
		if err := json.Unmarshal(d.Data, &data); err != nil {
			return docOut{}, fmt.Errorf("stored document %s is not JSON: %w", d.ID, err)
		}
	}
	return docOut{ID: d.ID, Data: data, UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt, Version: d.Version}, nil
}

func (t *tools) doc(tool string, d store.Doc) (docOut, error) {
	out, err := docValue(d)
	if err != nil {
		return docOut{}, t.fail(tool, err)
	}
	return out, nil
}

type docsOut struct {
	Documents []docOut `json:"documents"`
}

type deletedOut struct {
	Deleted bool `json:"deleted"`
}

func (t *tools) dbGet(ctx context.Context, _ *mcp.CallToolRequest, in dbDocIn) (*mcp.CallToolResult, docOut, error) {
	d, err := t.e.svc.Doc(ctx, t.viewer(ctx), in.ID, in.Collection, in.Doc)
	if err != nil {
		return nil, docOut{}, t.fail("db_get", err)
	}
	out, err := t.doc("db_get", d)
	return nil, out, err
}

func (t *tools) dbList(ctx context.Context, _ *mcp.CallToolRequest, in dbListIn) (*mcp.CallToolResult, docsOut, error) {
	docs, err := t.e.svc.Docs(ctx, t.viewer(ctx), in.ID, in.Collection)
	if err != nil {
		return nil, docsOut{}, t.fail("db_list", err)
	}
	return t.docs("db_list", docs)
}

func (t *tools) dbSet(ctx context.Context, _ *mcp.CallToolRequest, in dbSetIn) (*mcp.CallToolResult, docOut, error) {
	data, err := json.Marshal(in.Data)
	if err != nil || strings.TrimSpace(string(data)) == "null" {
		return nil, docOut{}, errors.New("data is required: send the JSON value to store")
	}
	d, err := t.e.svc.PutDoc(ctx, t.viewer(ctx), in.ID, in.Collection, in.Doc, data, in.IfVersion)
	if err != nil {
		return nil, docOut{}, t.fail("db_set", err)
	}
	out, err := t.doc("db_set", d)
	return nil, out, err
}

func (t *tools) dbDelete(ctx context.Context, _ *mcp.CallToolRequest, in dbDeleteIn) (*mcp.CallToolResult, deletedOut, error) {
	if err := t.e.svc.DeleteDoc(ctx, t.viewer(ctx), in.ID, in.Collection, in.Doc, in.IfVersion); err != nil {
		return nil, deletedOut{}, t.fail("db_delete", err)
	}
	return nil, deletedOut{Deleted: true}, nil
}

// jsonArg is an agent's JSON argument as the bytes the service takes; an
// absent one is nil.
func jsonArg(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

func (t *tools) dbQuery(ctx context.Context, _ *mcp.CallToolRequest, in dbQueryIn) (*mcp.CallToolResult, docsOut, error) {
	filter, err := jsonArg(in.Filter)
	if err != nil {
		return nil, docsOut{}, errors.New("filter must be a JSON object")
	}
	docs, err := t.e.svc.QueryDocs(ctx, t.viewer(ctx), in.ID, in.Collection, filter, in.Limit)
	if err != nil {
		return nil, docsOut{}, t.fail("db_query", err)
	}
	return t.docs("db_query", docs)
}

func (t *tools) docs(tool string, docs []store.Doc) (*mcp.CallToolResult, docsOut, error) {
	out := docsOut{Documents: make([]docOut, 0, len(docs))}
	for _, d := range docs {
		o, err := t.doc(tool, d)
		if err != nil {
			return nil, docsOut{}, err
		}
		out.Documents = append(out.Documents, o)
	}
	return nil, out, nil
}

func (t *tools) dbUpdate(ctx context.Context, _ *mcp.CallToolRequest, in dbUpdateIn) (*mcp.CallToolResult, docOut, error) {
	fields, err := jsonArg(in.Fields)
	if err != nil || fields == nil {
		return nil, docOut{}, errors.New("fields is required: send a JSON object of the fields to change")
	}
	d, err := t.e.svc.UpdateDoc(ctx, t.viewer(ctx), in.ID, in.Collection, in.Doc, fields, in.IfVersion)
	if err != nil {
		return nil, docOut{}, t.fail("db_update", err)
	}
	out, err := t.doc("db_update", d)
	return nil, out, err
}

type batchOut struct {
	Results []docOut `json:"results"`
}

func (t *tools) dbBatch(ctx context.Context, _ *mcp.CallToolRequest, in dbBatchIn) (*mcp.CallToolResult, batchOut, error) {
	ops := make([]store.DocOp, len(in.Ops))
	for i, o := range in.Ops {
		raw, err := jsonArg(o.Data)
		if o.Op == store.OpUpdate {
			raw, err = jsonArg(o.Fields)
		}
		if err != nil {
			return nil, batchOut{}, fmt.Errorf("operation %d: data is not JSON", i+1)
		}
		ops[i] = store.DocOp{Kind: o.Op, Collection: o.Collection, ID: o.ID, Data: raw, IfVersion: o.IfVersion}
	}
	docs, err := t.e.svc.BatchDocs(ctx, t.viewer(ctx), in.ID, ops)
	if err != nil {
		return nil, batchOut{}, t.fail("db_batch", err)
	}
	_, list, err := t.docs("db_batch", docs)
	return nil, batchOut{Results: list.Documents}, err
}

// registerDB adds the tools of this feature to the server.
func (t *tools) registerDB(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "db_get", Description: dbGetDoc, Annotations: readOnly("Read a shared document")}, t.dbGet)
	mcp.AddTool(s, &mcp.Tool{Name: "db_list", Description: dbListDoc, Annotations: readOnly("List a shared collection")}, t.dbList)
	mcp.AddTool(s, &mcp.Tool{Name: "db_query", Description: dbQueryDoc, Annotations: readOnly("Query a shared collection")}, t.dbQuery)
	mcp.AddTool(s, &mcp.Tool{Name: "db_update", Description: dbUpdateDoc, Annotations: &mcp.ToolAnnotations{Title: "Change fields of a shared document", DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}}, t.dbUpdate)
	mcp.AddTool(s, &mcp.Tool{Name: "db_batch", Description: dbBatchDoc, Annotations: &mcp.ToolAnnotations{Title: "Write several shared documents together", DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}}, t.dbBatch)
	mcp.AddTool(s, &mcp.Tool{Name: "db_set", Description: dbSetDoc, Annotations: &mcp.ToolAnnotations{Title: "Write a shared document", DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)}}, t.dbSet)
	mcp.AddTool(s, &mcp.Tool{Name: "db_delete", Description: dbDeleteDoc, Annotations: &mcp.ToolAnnotations{Title: "Delete a shared document", DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)}}, t.dbDelete)
}
