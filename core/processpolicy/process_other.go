//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package processpolicy

import (
	"errors"
	"os/exec"
)

type processTree struct{}

func prepareTree(_ *exec.Cmd) (*processTree, error) {
	return nil, errors.New("process tree containment is unsupported on this platform")
}
func (*processTree) attach(_ *exec.Cmd) error { return nil }
func (*processTree) kill(_ *exec.Cmd) error   { return nil }
func (*processTree) close() error             { return nil }
