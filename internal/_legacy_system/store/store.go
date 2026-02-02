package store

import (
	"os"
	"path/filepath"
)

type FS struct{ Root string }

func New(root string) (*FS, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &FS{Root: root}, nil
}
func (s *FS) JobDir(id string) string { return filepath.Join(s.Root, id) }
func (s *FS) MkJob(id string) (string, error) {
	j := s.JobDir(id)
	return j, os.MkdirAll(filepath.Join(j, "uploads"), 0o755)
}
