package web

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	thumbnailWidth             = 1120
	thumbnailHeight            = 720
	maxThumbnailBytes          = 1 << 20
	defaultThumbnailCacheBytes = 200 << 20
	maxThumbnailEntries        = 10000
)

type thumbnailFile struct {
	name string
	size int64
	used time.Time
}

// One disposable image per account/artifact/theme/language, with a bounded disk LRU.
// The index contains no artifact content; names are hashes, never request paths.
type thumbnailCache struct {
	mu    sync.Mutex
	dir   string
	limit int64
	files map[string]thumbnailFile
	bytes int64
}

func openThumbnailCache(dir string, limit int64) (*thumbnailCache, error) {
	if limit == 0 {
		limit = defaultThumbnailCacheBytes
	}
	c := &thumbnailCache{dir: dir, limit: limit, files: map[string]thumbnailFile{}}
	if dir == "" {
		return c, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".partial") {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		if !thumbnailName(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		c.files[name] = thumbnailFile{name, info.Size(), info.ModTime()}
		c.bytes += info.Size()
	}
	if err := c.trim(0); err != nil {
		return nil, err
	}
	return c, nil
}

func thumbnailName(name string) bool {
	if len(name) != 133 || name[64] != '-' || name[129:] != ".jpg" {
		return false
	}
	return hashName(name[:64]) && hashName(name[65:129])
}

func hashName(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, r := range v {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (c *thumbnailCache) remove(name string) error {
	if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	c.bytes -= c.files[name].size
	delete(c.files, name)
	return nil
}

func (c *thumbnailCache) trim(extra int64) error {
	files := make([]thumbnailFile, 0, len(c.files))
	for _, f := range c.files {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	for _, f := range files {
		if c.bytes+extra <= c.limit && len(c.files) < maxThumbnailEntries {
			return nil
		}
		if err := c.remove(f.name); err != nil {
			return err
		}
	}
	return nil
}

// Callers hold mu while checking identity and accessing the index.
func (c *thumbnailCache) get(name string) ([]byte, bool) {
	if c == nil || c.dir == "" {
		return nil, false
	}
	f, ok := c.files[name]
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(c.dir, name)) //nolint:gosec // G304: index names come only from thumbnailName-validated disk entries or put().
	if err != nil {
		return nil, false
	}
	f.used = time.Now()
	c.files[name] = f
	_ = os.Chtimes(filepath.Join(c.dir, name), f.used, f.used)
	return data, true
}

func (c *thumbnailCache) has(name string) bool {
	if c == nil {
		return false
	}
	_, ok := c.files[name]
	if ok {
		if _, err := os.Stat(filepath.Join(c.dir, name)); err != nil {
			c.bytes -= c.files[name].size
			delete(c.files, name)
			return false
		}
	}
	return ok
}

func (c *thumbnailCache) put(name string, data []byte) error {
	if c.dir == "" {
		return errors.New("thumbnail storage unavailable")
	}
	if !thumbnailName(name) || len(data) == 0 || len(data) > maxThumbnailBytes || int64(len(data)) > c.limit {
		return errors.New("thumbnail exceeds cache limits")
	}
	if err := c.trim(int64(len(data))); err != nil {
		return err
	}
	if c.has(name) {
		return nil
	}
	path := filepath.Join(c.dir, name)
	if err := os.WriteFile(path+".partial", data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(path+".partial", path); err != nil {
		_ = os.Remove(path + ".partial")
		return err
	}
	// A newer identity replaces this variant's previous revision, never other accounts.
	for old := range c.files {
		if strings.HasPrefix(old, name[:65]) {
			if old == name {
				c.bytes -= c.files[old].size
				delete(c.files, old)
			} else if err := c.remove(old); err != nil {
				return err
			}
		}
	}
	c.files[name] = thumbnailFile{name, int64(len(data)), time.Now()}
	c.bytes += int64(len(data))
	return nil
}
