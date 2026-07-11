package agent

import (
	"context"

	"myai/core/remote/protocol"
)

type WorkspaceFileFacade interface {
	Root() string
	List(ctx context.Context, payload protocol.FileListPayload) (protocol.FileListResultPayload, error)
	Read(ctx context.Context, payload protocol.FileReadPayload) (protocol.FileReadResultPayload, error)
}

type WorkspaceChangeFacade interface {
	Close() error
	List(ctx context.Context, payload protocol.ChangesListPayload) (protocol.ChangesListResultPayload, error)
	Diff(ctx context.Context, payload protocol.ChangeDiffPayload) (protocol.ChangeDiffResultPayload, error)
	Revert(ctx context.Context, payload protocol.ChangeRevertPayload) (protocol.ChangeRevertResultPayload, error)
	History(ctx context.Context, payload protocol.HistoryListPayload) (protocol.HistoryListResultPayload, error)
	HistoryDiff(ctx context.Context, payload protocol.HistoryDiffPayload) (protocol.HistoryDiffResultPayload, error)
	RevertCheckpoint(ctx context.Context, payload protocol.HistoryRevertPayload) (protocol.HistoryRevertResultPayload, error)
}
