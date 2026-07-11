package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	generationcommand "myai/core/application/chat/generation/command"
	domaintool "myai/core/domain/tool"
	repository "myai/core/port/repository"
)

func TestRecorderSavesToolExecutionRecords(t *testing.T) {
	persistence := &fakePersistence{}
	recorder := Recorder{
		Persistence: persistence,
		IDs:         &sequentialIDs{},
		RunAsync: func(task func()) {
			task()
		},
	}

	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		Entries: []domaintool.ExecutionEntry{{
			Kind:       domaintool.ExecutionEntryToolCall,
			SessionID:  "session-1",
			ToolCallID: "call-1",
		}},
		Assets: []domaintool.SharedAsset{{SessionID: "session-1", ShortCode: "asset-code"}},
	})

	if len(persistence.messages) != 1 || persistence.messages[0].ID != "id-1" || persistence.messages[0].Role != repository.RoleToolCall {
		t.Fatalf("expected message record to be saved, got %#v", persistence.messages)
	}
	if len(persistence.assets) != 1 || persistence.assets[0].ID != "id-2" || persistence.assets[0].ShortCode != "asset-code" {
		t.Fatalf("expected asset record to be saved, got %#v", persistence.assets)
	}
}

func TestRecorderReportsPersistenceErrors(t *testing.T) {
	persistence := &fakePersistence{
		messageErr: errors.New("message failed"),
		assetErr:   errors.New("asset failed"),
	}
	var reported error
	recorder := Recorder{
		Persistence: persistence,
		IDs:         &sequentialIDs{},
		RunAsync: func(task func()) {
			task()
		},
		OnError: func(err error) {
			reported = err
		},
	}

	recorder.RecordToolExecution(context.Background(), generationcommand.ToolExecutionRecord{
		Entries: []domaintool.ExecutionEntry{{Kind: domaintool.ExecutionEntryToolResult}},
		Assets:  []domaintool.SharedAsset{{}},
	})

	if reported == nil {
		t.Fatal("expected persistence error to be reported")
	}
	if text := reported.Error(); !strings.Contains(text, "message failed") || !strings.Contains(text, "asset failed") {
		t.Fatalf("expected combined persistence error, got %q", text)
	}
}

type sequentialIDs struct {
	next int
}

func (g *sequentialIDs) NewID() string {
	g.next++
	return fmt.Sprintf("id-%d", g.next)
}

type fakePersistence struct {
	messages   []repository.MessageRecord
	assets     []repository.AssetRecord
	messageErr error
	assetErr   error
}

func (p *fakePersistence) SaveMessage(ctx context.Context, record repository.MessageRecord) error {
	p.messages = append(p.messages, record)
	return p.messageErr
}

func (p *fakePersistence) SaveAsset(ctx context.Context, record repository.AssetRecord) error {
	p.assets = append(p.assets, record)
	return p.assetErr
}
