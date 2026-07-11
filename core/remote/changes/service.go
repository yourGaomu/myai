package changes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	historyport "myai/core/port/history"
)

type Service struct {
	// Service 以 SQLite baseline 为参照提供 Changes 视图，不要求 workspace 必须是 Git 仓库。
	root     string
	store    historyport.Store
	mu       sync.RWMutex
	baseline map[string]snapshotEntry
}

func NewWithStoreFactory(root string, historyPath string, factory historyport.StoreFactory) (*Service, error) {
	if factory == nil {
		return nil, errors.New("history store factory is nil")
	}

	root, err := normalizeRoot(root)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(historyPath) == "" {
		historyPath, err = factory.DefaultPath(root)
		if err != nil {
			return nil, err
		}
	}
	store, err := factory.Open(historyPath)
	if err != nil {
		return nil, err
	}

	service, err := newService(root, store)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return service, nil
}

func NewWithStore(root string, store historyport.Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("history store is nil")
	}
	root, err := normalizeRoot(root)
	if err != nil {
		return nil, err
	}
	return newService(root, store)
}

func newService(root string, store historyport.Store) (*Service, error) {
	service := &Service{
		root:     root,
		store:    store,
		baseline: make(map[string]snapshotEntry),
	}
	if err := service.loadOrCreateBaseline(context.Background()); err != nil {
		return nil, err
	}
	return service, nil
}

func normalizeRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", abs)
	}
	return abs, nil
}

func (s *Service) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

func (s *Service) Reset(ctx context.Context) error {
	snapshot, err := s.scan(ctx)
	if err != nil {
		return err
	}
	if err := s.store.ReplaceBaseline(ctx, s.root, snapshotToHistory(snapshot)); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseline = snapshot
	return nil
}

func (s *Service) loadOrCreateBaseline(ctx context.Context) error {
	// 首次打开时保存 baseline；后续进程重启继续复用，不会把现有改动误当成新基线。
	exists, err := s.store.HasBaseline(ctx, s.root)
	if err != nil {
		return err
	}
	if !exists {
		return s.Reset(ctx)
	}

	files, err := s.store.LoadBaseline(ctx, s.root)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseline = historyToSnapshot(files)
	return nil
}
