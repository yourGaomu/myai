//go:build windows

package grpcprocessor

import (
	"testing"

	"golang.org/x/sys/windows"
)

func assertSupervisorStopped(t *testing.T, pid int) {
	t.Helper()
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	state, err := windows.WaitForSingleObject(process, 500)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("document worker descendant remained alive after cleanup")
	}
}
