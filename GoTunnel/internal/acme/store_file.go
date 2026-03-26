package acme

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileStore struct {
	baseDir string
}

func NewFileStore(baseDir string) (*FileStore, error) {
	baseDir = filepath.Clean(baseDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create acme dir: %w", err)
	}
	return &FileStore{baseDir: baseDir}, nil
}

func (s *FileStore) SaveCertificate(_ context.Context, domain string, cert []byte, key []byte) error {
	dir := s.domainDir(domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), cert, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "key.pem"), key, 0o600)
}

func (s *FileStore) LoadCertificate(_ context.Context, domain string) ([]byte, []byte, error) {
	dir := s.domainDir(domain)
	cert, err := os.ReadFile(filepath.Join(dir, "cert.pem"))
	if err != nil {
		return nil, nil, err
	}
	key, err := os.ReadFile(filepath.Join(dir, "key.pem"))
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func (s *FileStore) DeleteCertificate(_ context.Context, domain string) error {
	return os.RemoveAll(s.domainDir(domain))
}

func (s *FileStore) SaveMetadata(_ context.Context, meta CertificateMetadata) error {
	dir := s.domainDir(meta.Domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), payload, 0o600)
}

func (s *FileStore) LoadMetadata(_ context.Context, domain string) (CertificateMetadata, error) {
	payload, err := os.ReadFile(filepath.Join(s.domainDir(domain), "metadata.json"))
	if err != nil {
		return CertificateMetadata{}, err
	}
	var meta CertificateMetadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		return CertificateMetadata{}, err
	}
	return meta, nil
}

func (s *FileStore) ListCertificates(_ context.Context) ([]CertificateMetadata, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}
	metas := make([]CertificateMetadata, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.LoadMetadata(context.Background(), restoreDomainFromDirName(entry.Name()))
		if err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

func (s *FileStore) domainDir(domain string) string {
	replacer := strings.NewReplacer("*", "_wildcard_", ":", "_", "/", "_", "\\", "_")
	return filepath.Join(s.baseDir, replacer.Replace(domain))
}

func restoreDomainFromDirName(name string) string {
	return strings.ReplaceAll(name, "_wildcard_", "*")
}
