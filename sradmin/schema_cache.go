package sradmin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// registryCache keeps what is looked up by schema ID for one registry: the schema and the
// subjects using it. Schema IDs are immutable, so each ID is queried once: from memory during a
// session and from the cache directory across sessions. Only successful lookups are stored.
type registryCache struct {
	// file is the cache file of the registry, empty when nothing is persisted
	file string

	mu      sync.Mutex
	loaded  bool
	entries map[int]cachedSchema
}

type cachedSchema struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	// Subjects are the subjects using the schema, valid when SubjectsKnown is set
	Subjects      []string `json:"subjects,omitempty"`
	SubjectsKnown bool     `json:"subjectsKnown,omitempty"`
}

// newRegistryCache returns the cache of the registry at url, stored in dir.
func newRegistryCache(dir, url string) *registryCache {
	c := &registryCache{entries: map[int]cachedSchema{}}
	if dir != "" {
		sum := sha256.Sum256([]byte(url))
		c.file = filepath.Join(dir, hex.EncodeToString(sum[:])[:16]+".json")
	}
	return c
}

// load reads the cache file once. An unreadable file is treated as empty, the cache is best effort.
func (c *registryCache) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	if c.file == "" {
		return
	}
	data, err := os.ReadFile(c.file)
	if err != nil {
		return
	}
	var entries map[int]cachedSchema
	if err := json.Unmarshal(data, &entries); err == nil {
		c.entries = entries
	}
}

// save writes the cache file atomically. Failing to write only means the next session queries again.
func (c *registryCache) save() {
	if c.file == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.file), 0700); err != nil {
		return
	}
	data, err := json.Marshal(c.entries)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.file), ".tmp-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmp.Name(), c.file)
}

// schema returns the cached schema of the ID.
func (c *registryCache) schema(id int) (cachedSchema, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	entry, ok := c.entries[id]
	return entry, ok && entry.Schema != ""
}

// putSchema stores the schema of the ID, keeping subjects that are already known.
func (c *registryCache) putSchema(id int, schema string, version int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	entry := c.entries[id]
	entry.Schema, entry.Version = schema, version
	c.entries[id] = entry
	c.save()
}

// subjects returns the cached subjects of the schema ID.
func (c *registryCache) subjects(id int) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	entry, ok := c.entries[id]
	return entry.Subjects, ok && entry.SubjectsKnown
}

// putSubjects stores the subjects using the schema ID, keeping the schema when it is known.
func (c *registryCache) putSubjects(id int, subjects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	entry := c.entries[id]
	entry.Subjects, entry.SubjectsKnown = subjects, true
	c.entries[id] = entry
	c.save()
}
