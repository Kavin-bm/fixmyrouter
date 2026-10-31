// Package store keeps fixmyrouter's local state as small JSON files in
// ~/.fixmyrouter (or $FIXMYROUTER_HOME). Nothing ever leaves the machine.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// ponytail: one in-process lock and atomic renames; two fixmyrouter
// processes writing at the same instant can still lose one write.
var mu sync.Mutex

// Dir returns the state directory.
func Dir() string {
	if d := os.Getenv("FIXMYROUTER_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".fixmyrouter"
	}
	return filepath.Join(home, ".fixmyrouter")
}

// Load reads name (e.g. "changes.json") into v. A missing file leaves v as is.
func Load(name string, v any) error {
	mu.Lock()
	defer mu.Unlock()
	b, err := os.ReadFile(filepath.Join(Dir(), name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Save writes v to name atomically (temp file + rename), readable only by you.
func Save(name string, v any) error {
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(Dir(), name+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(Dir(), name))
}
