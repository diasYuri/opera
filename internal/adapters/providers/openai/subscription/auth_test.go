package subscription

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/diasYuri/opera/internal/adapters/credentials"
	providercatalog "github.com/diasYuri/opera/internal/modules/providers/catalog"
)

func TestAuthenticatorStatusAndInvalidRecordOffline(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data", "credentials")
	store := credentials.FileStore{Directory: dir}
	record := Record{ClientID: "client", Subject: "subject", Email: "person@example.test", AccessToken: "private", ExpiresAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Set(ProviderID+".work.oauth", string(raw)); err != nil {
		t.Fatal(err)
	}
	handler := Authenticator()
	result, err := handler.Authenticate(context.Background(), "status", providercatalog.AuthIO{Store: store, Reference: "work", CredentialDir: dir, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }})
	if err != nil || result.Status != "expired" || result.Values["email"] != "person@example.test" {
		t.Fatalf("status = %#v, %v", result, err)
	}
	if _, err = handler.Authenticate(context.Background(), "unsupported", providercatalog.AuthIO{Store: store, Reference: "work", CredentialDir: dir}); err == nil {
		t.Fatal("unsupported action accepted")
	}
	if err = store.Set(ProviderID+".work.oauth", "{}"); err != nil {
		t.Fatal(err)
	}
	if _, err = handler.Authenticate(context.Background(), "status", providercatalog.AuthIO{Store: store, Reference: "work", CredentialDir: dir}); err == nil {
		t.Fatal("malformed credential accepted")
	}
}
