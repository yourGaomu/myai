//go:build windows

package trust

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestStoreRejectsLockWritableByOtherWindowsUsers(t *testing.T) {
	workspace, userDirectory := t.TempDir(), t.TempDir()
	store := NewStore(filepath.Join(userDirectory, "trust.json"))
	request := requestForTest(t, workspace, map[string]string{"command": "dummy"})
	name, err := windows.UTF16PtrFromString(store.Path() + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, &attributes, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	if err := store.Approve(request); err == nil {
		t.Fatal("world-writable Windows lock accepted")
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatal("unsafe lock allowed a trust store write")
	}
}
