package credentials

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestFileStoreRejectsBroadWindowsACL(t *testing.T) {
	store := testStore(t)
	if err := store.Set("default", "secret"); err != nil {
		t.Fatal(err)
	}
	path, err := store.path("default")
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("default"); err == nil {
		t.Fatal("accepted credential readable by Everyone")
	}
	// Restore permissions so the test framework can remove its temporary files.
	if err := restrict(path, false); err != nil {
		t.Fatal(err)
	}
}
