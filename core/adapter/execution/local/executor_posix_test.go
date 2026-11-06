//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package local

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func assertLocalStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		if state, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
			if end := strings.LastIndexByte(string(state), ')'); end >= 0 && len(state) > end+2 && state[end+2] == 'Z' {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("shell descendant remained alive after cancellation")
}
