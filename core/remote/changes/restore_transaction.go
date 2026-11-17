package changes

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/google/uuid"

	domainhistory "myai/core/domain/history"
)

type restorePlan struct {
	path    string
	current *domainhistory.FileSnapshot
	desired *domainhistory.FileSnapshot
}

type stagedRestore struct {
	restorePlan
	parent      *os.Root
	leaf        string
	backup      string
	replacement string
	applied     bool
	retain      bool
}

func filesystemPathKey(value string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(value)
	}
	return value
}

// restore keeps backups until both the file batch and its inverse history commit.
// All paths, including temporary files and compensation, stay under os.Root.
func (s *Service) restore(ctx context.Context, plans []restorePlan, source string) (string, error) {
	for i, plan := range plans {
		for _, other := range plans[:i] {
			a, b := filesystemPathKey(plan.path), filesystemPathKey(other.path)
			if a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
				return "", errors.New("checkpoint contains overlapping file paths")
			}
		}
		if err := validateRestorable(plan.desired); err != nil {
			return "", err
		}
		if err := s.checkCurrent(plan); err != nil {
			return "", err
		}
	}

	staged := make([]*stagedRestore, 0, len(plans))
	var createdDirectories []string
	cleanup := func(removeDirectories bool) {
		for _, item := range staged {
			if !item.retain && item.backup != "" {
				_ = item.parent.Remove(item.backup)
			}
			if item.replacement != "" {
				_ = item.parent.Remove(item.replacement)
			}
			_ = item.parent.Close()
		}
		if removeDirectories {
			for i := len(createdDirectories) - 1; i >= 0; i-- {
				_ = s.files.Remove(createdDirectories[i]) // Only empty directories.
			}
		}
	}
	fail := func(cause error) (string, error) {
		compensation := s.compensate(staged)
		cleanup(true)
		return "", errors.Join(cause, compensation)
	}

	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if err := s.ensureParents(path.Dir(plan.path), &createdDirectories); err != nil {
			return fail(err)
		}
		parent, err := s.openParent(plan.path)
		if err != nil {
			return fail(err)
		}
		item := &stagedRestore{restorePlan: plan, parent: parent, leaf: path.Base(plan.path)}
		staged = append(staged, item)
		if plan.current != nil {
			item.backup = ".myai-revert-before-" + uuid.NewString()
			if err := item.backupCurrent(); err != nil {
				return fail(err)
			}
		}
		if plan.desired != nil {
			item.replacement = ".myai-revert-after-" + uuid.NewString()
			if err := item.stageReplacement(); err != nil {
				return fail(err)
			}
		}
	}

	// Staging may take time. Recheck the entire batch before the first replacement.
	for _, item := range staged {
		if err := s.checkCurrent(item.restorePlan); err != nil {
			return fail(err)
		}
	}
	for _, item := range staged {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if err := s.checkCurrent(item.restorePlan); err != nil {
			return fail(err)
		}
		if err := validateRootPath(item.parent, item.leaf); err != nil {
			return fail(err)
		}
		var err error
		if item.desired == nil {
			err = item.parent.Remove(item.leaf)
		} else {
			err = s.rename(item.parent, item.replacement, item.leaf)
		}
		if err != nil {
			return fail(err)
		}
		item.applied = true
	}
	for _, item := range staged {
		now, exists, err := s.snapshotPath(item.path)
		if err != nil {
			return fail(err)
		}
		if !snapshotMatches(snapshotPtr(now, exists), item.desired) {
			return fail(fmt.Errorf("workspace conflict after restore: %s", item.path))
		}
	}

	inverse := make([]domainhistory.FileChange, 0, len(plans))
	for _, plan := range plans {
		kind := "modified"
		if plan.current == nil {
			kind = "added"
		} else if plan.desired == nil {
			kind = "deleted"
		}
		inverse = append(inverse, domainhistory.FileChange{
			Path: plan.path, ChangeType: kind, Before: plan.current, After: plan.desired,
		})
	}
	id, err := s.store.SaveCheckpoint(ctx, domainhistory.Checkpoint{
		Workspace: s.root, Title: "Revert " + source, Reason: "revert " + source,
	}, inverse)
	if err != nil {
		return fail(fmt.Errorf("record inverse checkpoint: %w", err))
	}
	if id == "" {
		return fail(errors.New("inverse checkpoint was not recorded"))
	}
	cleanup(false)
	return id, nil
}

func (s *Service) checkCurrent(plan restorePlan) error {
	now, exists, err := s.snapshotPath(plan.path)
	if err != nil {
		return err
	}
	if !snapshotMatches(snapshotPtr(now, exists), plan.current) {
		return fmt.Errorf("workspace conflict: %s changed before restore", plan.path)
	}
	return nil
}

func (s *Service) ensureParents(dir string, created *[]string) error {
	if dir == "." {
		return nil
	}
	prefix := ""
	for _, component := range strings.Split(dir, "/") {
		prefix = path.Join(prefix, component)
		if err := s.validatePath(prefix); err != nil {
			return err
		}
		info, err := s.files.Lstat(prefix)
		if errors.Is(err, os.ErrNotExist) {
			if err := s.files.Mkdir(prefix, 0o755); err != nil {
				return err
			}
			*created = append(*created, prefix)
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return unsafePath(prefix)
		}
	}
	return nil
}

func (s *Service) openParent(rel string) (*os.Root, error) {
	if err := s.validatePath(rel); err != nil {
		return nil, err
	}
	dir := path.Dir(rel)
	before, err := s.files.Lstat(dir)
	if err != nil {
		return nil, err
	}
	parent, err := s.files.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	after, err := parent.Stat(".")
	if err == nil && (!before.IsDir() || !os.SameFile(before, after)) {
		err = unsafePath(rel)
	}
	if err == nil {
		err = s.validatePath(rel)
	}
	if err != nil {
		_ = parent.Close()
		return nil, err
	}
	return parent, nil
}

func (item *stagedRestore) backupCurrent() error {
	if err := validateRootPath(item.parent, item.leaf); err != nil {
		return err
	}
	source, err := item.parent.Open(item.leaf)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return unsafePath(item.path)
	}
	backup, err := item.parent.OpenFile(item.backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		item.backup = "" // Do not remove an entry we did not create.
		return err
	}
	defer backup.Close()
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(backup, hasher), source)
	if err != nil {
		return err
	}
	if size != item.current.Size || hashSum(hasher) != item.current.Hash {
		return fmt.Errorf("workspace conflict while backing up: %s", item.path)
	}
	if err := backup.Chmod(item.current.Mode.Perm()); err != nil {
		return err
	}
	if err := backup.Sync(); err != nil {
		return err
	}
	return backup.Close()
}

func (item *stagedRestore) stageReplacement() error {
	file, err := item.parent.OpenFile(item.replacement, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		item.replacement = ""
		return err
	}
	defer file.Close()
	if _, err := file.Write(item.desired.Content); err != nil {
		return err
	}
	if err := file.Chmod(item.desired.Mode.Perm()); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return file.Close()
}

func (s *Service) rename(root *os.Root, from, to string) error {
	if s.renameFile != nil {
		return s.renameFile(root, from, to)
	}
	return root.Rename(from, to)
}

func (s *Service) compensate(staged []*stagedRestore) error {
	var compensation error
	for i := len(staged) - 1; i >= 0; i-- {
		item := staged[i]
		if !item.applied {
			continue
		}
		now, exists, err := s.snapshotPath(item.path)
		if err == nil && !snapshotMatches(snapshotPtr(now, exists), item.desired) {
			err = fmt.Errorf("workspace changed during compensation: %s", item.path)
		}
		if err == nil {
			if item.current == nil {
				err = item.parent.Remove(item.leaf)
			} else {
				err = s.rename(item.parent, item.backup, item.leaf)
			}
		}
		if err != nil {
			item.retain = true
			compensation = errors.Join(compensation, fmt.Errorf("restore compensation failed for %s; backup retained as %s: %w", item.path, item.backup, err))
		}
	}
	return compensation
}
