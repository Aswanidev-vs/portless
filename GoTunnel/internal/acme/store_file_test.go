package acme

import (
	"context"
	"testing"
	"time"
)

func TestFileStorePersistsCertificateAndMetadata(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}
	meta := CertificateMetadata{
		Domain:      "app.example.com",
		State:       CertStateIssued,
		NotBefore:   time.Now().UTC(),
		NotAfter:    time.Now().UTC().Add(24 * time.Hour),
		NextRenewal: time.Now().UTC().Add(12 * time.Hour),
	}
	if err := store.SaveCertificate(context.Background(), meta.Domain, []byte("cert"), []byte("key")); err != nil {
		t.Fatalf("save cert: %v", err)
	}
	if err := store.SaveMetadata(context.Background(), meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	cert, key, err := store.LoadCertificate(context.Background(), meta.Domain)
	if err != nil {
		t.Fatalf("load cert: %v", err)
	}
	if string(cert) != "cert" || string(key) != "key" {
		t.Fatalf("unexpected certificate payloads")
	}
	loadedMeta, err := store.LoadMetadata(context.Background(), meta.Domain)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if loadedMeta.Domain != meta.Domain || loadedMeta.State != meta.State {
		t.Fatalf("unexpected metadata: %#v", loadedMeta)
	}
}
