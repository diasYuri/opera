package credentials

import (
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
)

// Opt-in native diagnostic. It only reads its own public canary under a unique
// reference and removes it afterward; it never inspects account credentials.
func TestLiveSystemKeyringLargeRecord(t *testing.T) {
	if os.Getenv("OPERA_KEYRING_LIVE_TEST") != "1" {
		t.Skip("set OPERA_KEYRING_LIVE_TEST=1 to test the native keyring")
	}
	store := systemStore{}
	name := "diagnostic-" + rand.Text()
	value := strings.Repeat("opera-public-diagnostic-", 400)
	t.Cleanup(func() {
		if err := store.Delete(name); err != nil && !errors.Is(err, ErrNotFound) {
			t.Error("diagnostic keyring cleanup failed", err)
		}
	})
	if err := store.Set(name, value); err != nil {
		t.Fatal("native large-record write failed", err)
	}
	got, err := store.Get(name)
	if err != nil {
		t.Fatal("native large-record read failed", err)
	}
	if got != value {
		t.Fatal("native large-record round trip changed the canary")
	}
}
