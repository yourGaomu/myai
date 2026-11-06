//go:build windows

package local

import (
	"testing"

	"golang.org/x/sys/windows"
)

func assertLocalStopped(t *testing.T, pid int) {
	t.Helper()
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	state, err := windows.WaitForSingleObject(process, 500)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("shell descendant remained alive after cancellation")
	}
}
