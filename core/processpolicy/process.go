package processpolicy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// DefaultWaitDelay bounds inherited-pipe draining after exit or cancellation.
const DefaultWaitDelay = 500 * time.Millisecond

// Process owns a command and its platform-specific cleanup scope: a Job Object
// containing descendants on Windows, or the child's initial process group on
// POSIX. POSIX descendants can escape that group with setsid/setpgid; cleanup is
// best-effort for those descendants and does not provide OS containment.
// Start reaps the command in the background; callers must use Process.Wait
// rather than Cmd.Wait. Root exit also cleans up the job or original group;
// this API does not manage a separate background-process lifetime.
type Process struct {
	cmd      *exec.Cmd
	tree     *processTree
	mu       sync.Mutex
	finished bool
	done     chan struct{}
	err      error
}

// Start applies a minimal environment when Env is nil, a bounded WaitDelay, and
// cancellation of the Windows job or POSIX process group for exec.CommandContext.
// Failure to establish that platform's cleanup scope is a startup error. A POSIX
// process group cannot contain descendants that change their session or group.
func Start(cmd *exec.Cmd) (*Process, error) {
	if cmd == nil {
		return nil, errors.New("processpolicy: command is nil")
	}
	if cmd.Process != nil {
		return nil, errors.New("processpolicy: command was already started")
	}
	if cmd.Env == nil {
		cmd.Env = Environment(nil)
	}
	if cmd.WaitDelay <= 0 || cmd.WaitDelay > DefaultWaitDelay {
		cmd.WaitDelay = DefaultWaitDelay
	}
	tree, err := prepareTree(cmd)
	if err != nil {
		return nil, fmt.Errorf("processpolicy: prepare process tree: %w", err)
	}
	process := &Process{cmd: cmd, tree: tree, done: make(chan struct{})}
	if cmd.Cancel != nil {
		cmd.Cancel = process.Kill
	}
	// Cancellation may race with Start. Keep cleanup locked until the child's
	// platform-specific scope is established (and, on Windows, it is resumed).
	process.mu.Lock()
	if err := cmd.Start(); err != nil {
		process.finished = true
		_ = tree.close()
		process.mu.Unlock()
		return nil, err
	}
	if err := tree.attach(cmd); err != nil {
		// Windows starts suspended, so no child code has run on this path.
		_ = cmd.Process.Kill()
		_ = tree.kill(cmd)
		process.finished = true
		_ = tree.close()
		process.mu.Unlock()
		_ = cmd.Wait()
		return nil, fmt.Errorf("processpolicy: contain process tree: %w", err)
	}
	process.mu.Unlock()
	go func() {
		err := cmd.Wait()
		process.mu.Lock()
		cleanupErr := tree.kill(cmd)
		closeErr := tree.close()
		process.finished = true
		process.mu.Unlock()
		process.err = errors.Join(err, cleanupErr, closeErr)
		close(process.done)
	}()
	return process, nil
}

// Run starts a command and waits for bounded I/O draining and platform-specific
// cleanup. Successful return does not imply escaped POSIX descendants were killed.
func Run(cmd *exec.Cmd) error {
	process, err := Start(cmd)
	if err != nil {
		return err
	}
	return process.Wait()
}

// Kill forcefully terminates the Windows job or original POSIX process group.
// POSIX descendants that changed their session/group are outside that scope.
// Kill is safe to call concurrently with Wait or context cancellation. Wait
// must still be used to await cleanup within the platform's supported scope.
func (process *Process) Kill() error {
	process.mu.Lock()
	defer process.mu.Unlock()
	if process.finished {
		return os.ErrProcessDone
	}
	return process.tree.kill(process.cmd)
}

func (process *Process) Wait() error {
	<-process.done
	return process.err
}

func (process *Process) Done() <-chan struct{} { return process.done }
