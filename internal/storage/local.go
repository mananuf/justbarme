package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalDiskProvider writes objects to a local directory -- for
// development without real object-storage credentials. Unlike email's
// ConsoleProvider (which only logs and never persists anything), this
// genuinely writes the file: a logo upload needs to be retrievable to
// preview during development, not just acknowledged. Never used in
// production -- config.Load refuses to start there without real
// credentials for the configured provider.
type LocalDiskProvider struct {
	dir           string
	publicBaseURL string
}

func NewLocalDiskProvider(dir, publicBaseURL string) *LocalDiskProvider {
	return &LocalDiskProvider{dir: dir, publicBaseURL: strings.TrimSuffix(publicBaseURL, "/")}
}

func (p *LocalDiskProvider) Put(_ context.Context, key, _ string, data []byte) (string, error) {
	path := filepath.Join(p.dir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create upload directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write local object: %w", err)
	}
	return p.publicBaseURL + "/" + key, nil
}

func (p *LocalDiskProvider) URL(key string) string {
	return p.publicBaseURL + "/" + key
}

func (p *LocalDiskProvider) Delete(_ context.Context, key string) error {
	path := filepath.Join(p.dir, filepath.FromSlash(key))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete local object: %w", err)
	}
	return nil
}
