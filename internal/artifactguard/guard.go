// Package artifactguard orders per-artifact business admission before room locks.
package artifactguard

import "sync"

// Admission is shared by service, socket admission and per-message receive.
// Register the same registry before use; callbacks inside it must not reenter.
type Admission interface {
	Shared(string) func()
	Exclusive(string) func()
}

type entry struct {
	rw   sync.RWMutex
	refs int
}

// Registry retains an entry only while an operation holds or awaits it.
// Its leaf mutex is never held while waiting on an artifact lock.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*entry
}

func (r *Registry) retain(id string) *entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = map[string]*entry{}
	}
	e := r.entries[id]
	if e == nil {
		e = &entry{}
		r.entries[id] = e
	}
	e.refs++
	return e
}

func (r *Registry) release(id string, e *entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e.refs--
	if e.refs == 0 {
		delete(r.entries, id)
	}
}

// Shared admits concurrent source operations while excluding authority changes.
func (r *Registry) Shared(id string) func() {
	e := r.retain(id)
	e.rw.RLock()
	return func() { e.rw.RUnlock(); r.release(id, e) }
}

// Exclusive orders authority changes against all admitted operations.
func (r *Registry) Exclusive(id string) func() {
	e := r.retain(id)
	e.rw.Lock()
	return func() { e.rw.Unlock(); r.release(id, e) }
}
