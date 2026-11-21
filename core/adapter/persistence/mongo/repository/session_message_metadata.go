package repository

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mongomapper "myai/core/adapter/persistence/mongo/mapper"
	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
	"myai/core/session"
)

var (
	_ repository.SessionMessageMetadataStore = (*Store)(nil)
	_ repository.SessionPlanStore            = (*Store)(nil)
)

func (m *Store) TouchSessionMessages(ctx context.Context, record repository.SessionRecord, sequence int64, includeUsage bool) error {
	if strings.TrimSpace(record.ID) == "" {
		return errors.New("session id is empty")
	}
	if includeUsage && (record.Usage != nil || record.LastUsage != nil) && sequence <= 0 {
		return errors.New("session message usage sequence must be positive")
	}

	// A message snapshot may initialize defaults, but it cannot initialize a
	// plan or opt a missing session into plan mode. Plans have their own CAS.
	initial := record
	initial.CurrentPlan = nil
	initial.AgentMode = string(session.AgentModeChat)
	initial.Usage, initial.LastUsage = nil, nil
	encoded, err := bson.Marshal(mongomapper.SessionDocumentFromRecord(initial))
	if err != nil {
		return err
	}
	insert := bson.M{}
	if err := bson.Unmarshal(encoded, &insert); err != nil {
		return err
	}
	delete(insert, "updated_at")
	// Only identity belongs in the upsert filter: a title or sequence mismatch
	// must never turn a conditional update into a duplicate session insert.
	result, err := m.template.UpdateOne(ctx, sessionsCollection, bson.M{"_id": record.ID}, bson.M{
		"$setOnInsert": insert,
		"$max":         bson.M{"updated_at": record.UpdatedAt},
	}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return err
	}
	if result == nil || (result.MatchedCount == 0 && result.UpsertedCount == 0) {
		return errors.New("mongo session metadata initialization returned no match or insert")
	}

	if title := strings.TrimSpace(record.Title); title != "" && title != "New chat" {
		result, err = m.template.UpdateOne(ctx, sessionsCollection, bson.M{"_id": record.ID, "$or": bson.A{
			bson.M{"title": "New chat"}, bson.M{"title": ""}, bson.M{"title": bson.M{"$exists": false}},
		}}, bson.M{"$set": bson.M{"title": record.Title}})
		if err != nil {
			return err
		}
		if result == nil {
			return errors.New("mongo session title update returned no result")
		}
	}
	if !includeUsage || (record.Usage == nil && record.LastUsage == nil) {
		return nil
	}

	if record.Usage != nil {
		// Older full-session writers stored an absent usage as BSON null. Remove
		// only that null before writing nested counters; an object never matches.
		result, err = m.template.UpdateOne(ctx, sessionsCollection, bson.M{"_id": record.ID, "usage": nil}, bson.M{"$unset": bson.M{"usage": ""}})
		if err != nil {
			return err
		}
		if result == nil {
			return errors.New("mongo session usage normalization returned no result")
		}
	}
	set := bson.M{"message_usage_sequence": sequence}
	if record.LastUsage != nil {
		set["last_usage"] = mongomapper.TokenUsageDocumentFromRecord(record.LastUsage)
	}
	update := bson.M{"$set": set}
	if usage := record.Usage; usage != nil {
		// Usage is a cumulative snapshot, never a delta. $max also protects legacy
		// sessions without a sequence and newer snapshots built from stale state.
		update["$max"] = bson.M{
			"usage.prompt_tokens":           usage.PromptTokens,
			"usage.completion_tokens":       usage.CompletionTokens,
			"usage.total_tokens":            usage.TotalTokens,
			"usage.reasoning_tokens":        usage.ReasoningTokens,
			"usage.prompt_cached_tokens":    usage.PromptCachedTokens,
			"usage.prompt_cache_reported":   usage.PromptCacheReported,
			"usage.prompt_cache_incomplete": usage.PromptCacheIncomplete,
			"usage.available":               usage.Available,
		}
	}
	result, err = m.template.UpdateOne(ctx, sessionsCollection, bson.M{"_id": record.ID, "$or": bson.A{
		bson.M{"message_usage_sequence": bson.M{"$lt": sequence}},
		bson.M{"message_usage_sequence": bson.M{"$exists": false}},
	}}, update)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("mongo session usage update returned no result")
	}
	// A zero match means that this snapshot was already committed or superseded.
	return nil
}

func (m *Store) SaveSessionPlan(ctx context.Context, sessionID, expectedID string, expectedRevision int64, next *agentplan.Plan) error {
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("session id is empty")
	}
	if expectedRevision < 0 || (expectedID == "" && expectedRevision != 0) {
		return repository.ErrConflict
	}
	if next != nil {
		if next.ID == "" || next.SessionID != sessionID || next.Revision <= 0 {
			return repository.ErrConflict
		}
		if next.ID != expectedID {
			// A new draft may explicitly replace a plan, but only while the
			// expected previous identity/revision below still matches.
			if next.Revision != 1 {
				return repository.ErrConflict
			}
		} else if next.Revision <= expectedRevision || next.Revision-expectedRevision != 1 {
			return repository.ErrConflict
		}
	}
	filter := bson.M{"_id": sessionID}
	if expectedID == "" {
		filter["current_plan"] = nil
	} else {
		filter["current_plan.id"] = expectedID
		if expectedRevision == 0 {
			filter["$or"] = bson.A{bson.M{"current_plan.revision": int64(0)}, bson.M{"current_plan.revision": bson.M{"$exists": false}}}
		} else {
			filter["current_plan.revision"] = expectedRevision
		}
	}
	document := mongomapper.PlanDocumentFromDomain(next)
	result, err := m.template.UpdateOne(ctx, sessionsCollection, filter, bson.M{
		"$set": bson.M{"current_plan": document},
		"$max": bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("mongo session plan update returned no result")
	}
	if result.MatchedCount > 0 {
		return nil
	}

	// A prior attempt may have committed before its transport failed. Accept
	// only the same persisted payload, not another update reusing ID/revision.
	current, err := m.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.ErrConflict
		}
		return err
	}
	persisted, err := bson.Marshal(bson.M{"plan": mongomapper.PlanDocumentFromDomain(current.CurrentPlan)})
	if err != nil {
		return err
	}
	wanted, err := bson.Marshal(bson.M{"plan": document})
	if err != nil {
		return err
	}
	if bytes.Equal(persisted, wanted) {
		return nil
	}
	return repository.ErrConflict
}
