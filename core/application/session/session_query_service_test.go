package sessionapp

import (
	"context"
	"testing"

	repository "myai/core/port/repository"
)

func TestSessionQueryServiceListSessions(t *testing.T) {
	store := &fakeSessionQueryStore{
		sessions: []repository.SessionRecord{{ID: "session-1", Deleted: true}},
	}

	records, err := (SessionQueryService{Sessions: store}).ListSessions(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}

	if !store.includeDeleted || len(records) != 1 || records[0].ID != "session-1" {
		t.Fatalf("expected deleted sessions to be listed, include=%v records=%#v", store.includeDeleted, records)
	}
}

func TestSessionQueryServiceListSessionsWithoutStore(t *testing.T) {
	records, err := (SessionQueryService{}).ListSessions(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Fatalf("expected nil records without store, got %#v", records)
	}
}

func TestSessionQueryServiceListAssets(t *testing.T) {
	store := &fakeSessionQueryStore{
		assets: []repository.AssetRecord{{ID: "asset-1", SessionID: "session-1"}},
	}

	assets, err := (SessionQueryService{Assets: store}).ListAssets(context.Background(), ListAssetsCommand{
		SessionID: " session-1 ",
		Limit:     20,
	})
	if err != nil {
		t.Fatal(err)
	}

	if store.assetSessionID != "session-1" || store.assetLimit != 20 || len(assets) != 1 {
		t.Fatalf("expected asset query to be delegated, store=%#v assets=%#v", store, assets)
	}
}

func TestSessionQueryServiceListAssetsRequiresSessionID(t *testing.T) {
	_, err := (SessionQueryService{}).ListAssets(context.Background(), ListAssetsCommand{})
	if err == nil || err.Error() != "session id is empty" {
		t.Fatalf("expected empty session id error, got %v", err)
	}
}

type fakeSessionQueryStore struct {
	includeDeleted bool
	sessions       []repository.SessionRecord
	assets         []repository.AssetRecord
	assetSessionID string
	assetLimit     int
}

func (s *fakeSessionQueryStore) ListSessionsWithDeleted(ctx context.Context, includeDeleted bool) ([]repository.SessionRecord, error) {
	s.includeDeleted = includeDeleted
	return s.sessions, nil
}

func (s *fakeSessionQueryStore) ListAssets(ctx context.Context, sessionID string, limit int) ([]repository.AssetRecord, error) {
	s.assetSessionID = sessionID
	s.assetLimit = limit
	return s.assets, nil
}
