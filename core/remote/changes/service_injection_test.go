package changes

import (
	"context"
	"testing"

	domainhistory "myai/core/domain/history"
	historyport "myai/core/port/history"
)

func TestNewWithStoreUsesInjectedHistoryStore(t *testing.T) {
	store := &fakeHistoryStore{hasBaseline: true}
	service, err := NewWithStore(t.TempDir(), store)
	if err != nil {
		t.Fatalf("NewWithStore() error = %v", err)
	}
	if store.loadBaselineCalls != 1 {
		t.Fatalf("LoadBaseline() calls = %d, want 1", store.loadBaselineCalls)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if store.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", store.closeCalls)
	}
}

func TestNewWithStoreFactoryUsesInjectedFactory(t *testing.T) {
	store := &fakeHistoryStore{hasBaseline: true}
	factory := &fakeHistoryStoreFactory{store: store}
	service, err := NewWithStoreFactory(t.TempDir(), "", factory)
	if err != nil {
		t.Fatalf("NewWithStoreFactory() error = %v", err)
	}
	defer service.Close()

	if factory.defaultPathCalls != 1 {
		t.Fatalf("DefaultPath() calls = %d, want 1", factory.defaultPathCalls)
	}
	if factory.openCalls != 1 {
		t.Fatalf("Open() calls = %d, want 1", factory.openCalls)
	}
}

type fakeHistoryStoreFactory struct {
	store            historyport.Store
	defaultPathCalls int
	openCalls        int
}

func (f *fakeHistoryStoreFactory) DefaultPath(string) (string, error) {
	f.defaultPathCalls++
	return "history.db", nil
}

func (f *fakeHistoryStoreFactory) Open(string) (historyport.Store, error) {
	f.openCalls++
	return f.store, nil
}

type fakeHistoryStore struct {
	hasBaseline       bool
	loadBaselineCalls int
	closeCalls        int
}

func (s *fakeHistoryStore) Close() error {
	s.closeCalls++
	return nil
}

func (s *fakeHistoryStore) HasBaseline(context.Context, string) (bool, error) {
	return s.hasBaseline, nil
}

func (s *fakeHistoryStore) LoadBaseline(context.Context, string) (map[string]domainhistory.FileSnapshot, error) {
	s.loadBaselineCalls++
	return map[string]domainhistory.FileSnapshot{}, nil
}

func (s *fakeHistoryStore) ReplaceBaseline(context.Context, string, map[string]domainhistory.FileSnapshot) error {
	s.hasBaseline = true
	return nil
}

func (*fakeHistoryStore) SaveCheckpoint(context.Context, domainhistory.Checkpoint, []domainhistory.FileChange) (string, error) {
	return "", nil
}

func (*fakeHistoryStore) ListCheckpoints(context.Context, string, int) ([]domainhistory.CheckpointSummary, error) {
	return nil, nil
}

func (*fakeHistoryStore) LoadCheckpointChanges(context.Context, string, string) ([]domainhistory.StoredFileChange, error) {
	return nil, nil
}
