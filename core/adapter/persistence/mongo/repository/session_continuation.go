package repository

import (
	"context"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"myai/core/adapter/persistence/mongo/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	repository "myai/core/port/repository"
)

const continuationControlsCollection = "session_continuation_controls"

var _ repository.SessionContinuationStore = (*Store)(nil)

func (m *Store) ContinuationAllowed(ctx context.Context, sessionID string) (bool, error) {
	// GetSession excludes soft-deleted sessions. Never resurrect them through
	// a cached in-memory session or a late child completion.
	if _, err := m.GetSession(ctx, sessionID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var control po.ContinuationControl
	err := m.template.FindOne(ctx, continuationControlsCollection, bson.M{"_id": sessionID}, &control)
	if errors.Is(err, mongotemplate.ErrNotFound) {
		return true, nil
	}
	return !control.Paused && err == nil, err
}

func (m *Store) SetContinuationPaused(ctx context.Context, sessionID string, paused bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	_, err := m.template.UpdateOne(ctx, continuationControlsCollection, bson.M{"_id": sessionID},
		bson.M{"$set": bson.M{"paused": paused}}, options.UpdateOne().SetUpsert(true))
	return err
}
