//go:build windows

package mcp

import (
	"testing"

	"golang.org/x/sys/windows"
)

func assertMCPStopped(t *testing.T, pid int) {
	t.Helper()
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	state, err := windows.WaitForSingleObject(process, 500)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("MCP descendant remained alive after cleanup")
	}
}
