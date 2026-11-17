//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package processpolicy

import (
	"errors"
	"os/exec"
	"syscall"
)

type processTree struct{}

func prepareTree(cmd *exec.Cmd) (*processTree, error) {
	attributes := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attributes = *cmd.SysProcAttr
	}
	if attributes.Setsid || attributes.Foreground || attributes.Pgid != 0 {
		return nil, errors.New("caller-specified session or process group is unsupported")
	}
	attributes.Setpgid = true
	cmd.SysProcAttr = &attributes
	return &processTree{}, nil
}

func (*processTree) attach(_ *exec.Cmd) error { return nil }

func (*processTree) kill(cmd *exec.Cmd) error {
	// The child's PID is also its initial process group ID. SIGKILL reaches
	// ordinary descendants that remain in that group without shell cooperation.
	// setsid/setpgid can move descendants outside it; this is best-effort cleanup,
	// not an OS containment boundary or a guarantee to terminate escaped children.
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (*processTree) close() error { return nil }
