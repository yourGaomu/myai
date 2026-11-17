//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package processpolicy

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

func processAlive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	// Linux containers do not always reap orphan zombies promptly. A zombie
	// cannot execute or keep output pipes open and is already terminated.
	if state, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		if end := strings.LastIndexByte(string(state), ')'); end >= 0 && len(state) > end+2 && state[end+2] == 'Z' {
			return false
		}
	}
	return true
}
