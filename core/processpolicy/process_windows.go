//go:build windows

package processpolicy

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processTree struct {
	job windows.Handle
}

func prepareTree(cmd *exec.Cmd) (*processTree, error) {
	attributes := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attributes = *cmd.SysProcAttr
	}
	if attributes.CreationFlags&(windows.CREATE_BREAKAWAY_FROM_JOB|windows.CREATE_SUSPENDED) != 0 {
		return nil, errors.New("caller-specified breakaway or suspended process is unsupported")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("set kill-on-close job limit: %w", err)
	}
	// No user code runs before AssignProcessToJobObject. Nested-job assignment
	// failures are fatal; silently running outside this job would leak children.
	attributes.CreationFlags |= windows.CREATE_SUSPENDED
	cmd.SysProcAttr = &attributes
	return &processTree{job: job}, nil
}

func (tree *processTree) attach(cmd *exec.Cmd) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("open suspended process: %w", err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(tree.job, process); err != nil {
		return fmt.Errorf("assign process to job (nested jobs must be permitted): %w", err)
	}
	return resumeInitialThread(uint32(cmd.Process.Pid))
}

func resumeInitialThread(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot suspended threads: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return fmt.Errorf("open initial thread: %w", err)
		}
		previous, resumeErr := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if resumeErr != nil {
			return fmt.Errorf("resume initial thread: %w", resumeErr)
		}
		if previous != 1 {
			return fmt.Errorf("unexpected initial thread suspend count: %d", previous)
		}
		return nil
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate initial thread: %w", err)
	}
	return errors.New("suspended process has no initial thread")
}

func (tree *processTree) kill(_ *exec.Cmd) error {
	if err := windows.TerminateJobObject(tree.job, 1); err != nil {
		return fmt.Errorf("terminate job: %w", err)
	}
	// TerminateJobObject is asynchronous. Wait for all descendants to leave the
	// job, while bounding shutdown independently of inherited output pipes.
	deadline := time.Now().Add(DefaultWaitDelay)
	for {
		var accounting struct {
			UserTime, KernelTime, PeriodUserTime, PeriodKernelTime  int64
			PageFaults, TotalProcesses, ActiveProcesses, Terminated uint32
		}
		if err := windows.QueryInformationJobObject(tree.job, windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return fmt.Errorf("query terminated job: %w", err)
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("job termination exceeded cleanup deadline")
		}
		time.Sleep(time.Millisecond)
	}
}

func (tree *processTree) close() error {
	return windows.CloseHandle(tree.job)
}
