// Package jsruntime holds the libraries pictor serves to artifact pages
// (React, React DOM, Recharts, lucide-react, the component kit, Tailwind's
// browser build) as versioned ESM, embedded in the binary.
//
// The files are built by runtime/build.mjs from runtime/package-lock.json and
// committed under v1/. Node is a build-time tool only; `make verify-runtime`
// fails when the committed files differ from a fresh build. A runtime version
// is immutable once artifacts depend on it: changing what v1 contains means
// adding v2 beside it (docs/runtime.md).
package jsruntime

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sync"
)

//go:embed all:v1
var files embed.FS

// Version is the name of a runtime: the folder it is embedded under and the
// path segment it is served at (/vendor/v1/).
type Version string

// V1 is the first runtime.
const V1 Version = "v1"

// Module is one import specifier a page may use.
type Module struct {
	// File is the content-hashed name of the ESM file, relative to the
	// runtime's root.
	File string `json:"file"`
	// Exports is every name the module exports, "default" included when it has one.
	Exports []string `json:"exports"`
}

// Manifest describes one runtime build.
type Manifest struct {
	Runtime Version `json:"runtime"`
	// Specifiers maps what a page writes in `import ... from "<specifier>"`
	// to the file that provides it.
	Specifiers map[string]Module `json:"specifiers"`
	// Scripts are classic scripts (not modules) the page loads, by role.
	Scripts map[string]string `json:"scripts"`
	// Notices is the file holding the license texts of the bundled packages.
	Notices string `json:"notices"`
	// Files maps every file of the runtime, the manifest excepted, to its SHA-256.
	Files map[string]string `json:"files"`
}

// Runtime is one embedded runtime: its files and what they contain.
type Runtime struct {
	FS       fs.FS
	Manifest Manifest
}

// Load returns the runtime embedded under version. A version this binary does
// not carry is an error, so an artifact that asks for one fails visibly.
func Load(version Version) (*Runtime, error) {
	loaded.Lock()
	defer loaded.Unlock()
	if rt, ok := loaded.byVersion[version]; ok {
		return rt, nil
	}
	rt, err := load(version)
	if err != nil {
		return nil, err
	}
	loaded.byVersion[version] = rt
	return rt, nil
}

// loaded memoizes Load: the files are embedded and never change, and a page
// view asks for the runtime on every request.
var loaded = struct {
	sync.Mutex
	byVersion map[Version]*Runtime
}{byVersion: map[Version]*Runtime{}}

func load(version Version) (*Runtime, error) {
	root, err := fs.Sub(files, string(version))
	if err != nil {
		return nil, fmt.Errorf("embedded runtime %s: %w", version, err)
	}
	raw, err := fs.ReadFile(root, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("runtime %s has no manifest (run `make runtime`): %w", version, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest of runtime %s: %w", version, err)
	}
	if m.Runtime != version {
		return nil, fmt.Errorf("runtime %s embeds a manifest for %q", version, m.Runtime)
	}
	return &Runtime{FS: root, Manifest: m}, nil
}
