package web

import (
	"container/list"
	"sync"
	"time"
)

const (
	maxPreviewBytes      = 2 << 20
	maxPreviewCacheBytes = 32 << 20
	maxPreviewEntries    = 128
)

type previewEntry struct {
	key     string
	html    string
	expires time.Time
}

// previewCache is disposable, process-local and bounded. Keys already contain
// account and authorization/render identity; no snapshot is shared by accounts.
type previewCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	bytes   int
	now     func() time.Time
}

func newPreviewCache(now func() time.Time) *previewCache {
	if now == nil {
		now = time.Now
	}
	return &previewCache{entries: make(map[string]*list.Element), order: list.New(), now: now}
}

func (c *previewCache) remove(e *list.Element) {
	entry := e.Value.(previewEntry)
	delete(c.entries, entry.key)
	c.bytes -= len(entry.html)
	c.order.Remove(e)
}

func (c *previewCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[key]
	if e == nil {
		return "", false
	}
	entry := e.Value.(previewEntry)
	if !c.now().Before(entry.expires) {
		c.remove(e)
		return "", false
	}
	c.order.MoveToFront(e)
	return entry.html, true
}

func (c *previewCache) put(key, html string, expires time.Time) bool {
	if len(html) == 0 || len(html) > maxPreviewBytes {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.now().Before(expires) {
		return false
	}
	if e := c.entries[key]; e != nil {
		c.remove(e)
	}
	for e := c.order.Back(); e != nil; {
		previous := e.Prev()
		if !c.now().Before(e.Value.(previewEntry).expires) {
			c.remove(e)
		}
		e = previous
	}
	for c.order.Len() >= maxPreviewEntries || c.bytes+len(html) > maxPreviewCacheBytes {
		c.remove(c.order.Back())
	}
	e := c.order.PushFront(previewEntry{key: key, html: html, expires: expires})
	c.entries[key] = e
	c.bytes += len(html)
	return true
}
