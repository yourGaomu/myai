package agent

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

func (a *Agent) handleFileList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.FileListPayload](message)
	if err != nil {
		return fmt.Errorf("decode file list failed: %w", err)
	}

	result, err := a.fileService.List(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeFileListResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleFileRead(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.FileReadPayload](message)
	if err != nil {
		return fmt.Errorf("decode file read failed: %w", err)
	}

	result, err := a.fileService.Read(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeFileReadResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleChangesList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ChangesListPayload](message)
	if err != nil {
		return fmt.Errorf("decode changes list failed: %w", err)
	}

	result, err := a.changeService.List(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeChangesListResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleChangeDiff(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ChangeDiffPayload](message)
	if err != nil {
		return fmt.Errorf("decode change diff failed: %w", err)
	}

	result, err := a.changeService.Diff(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeChangeDiffResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleChangeRevert(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ChangeRevertPayload](message)
	if err != nil {
		return fmt.Errorf("decode change revert failed: %w", err)
	}

	result, err := a.changeService.Revert(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeChangeRevertResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleHistoryList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.HistoryListPayload](message)
	if err != nil {
		return fmt.Errorf("decode history list failed: %w", err)
	}

	result, err := a.changeService.History(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeHistoryListResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleHistoryDiff(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.HistoryDiffPayload](message)
	if err != nil {
		return fmt.Errorf("decode history diff failed: %w", err)
	}

	result, err := a.changeService.HistoryDiff(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeHistoryDiffResult, message.RequestID, message.SessionID, result)
}

func (a *Agent) handleHistoryRevert(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.HistoryRevertPayload](message)
	if err != nil {
		return fmt.Errorf("decode history revert failed: %w", err)
	}

	result, err := a.changeService.RevertCheckpoint(ctx, payload)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeHistoryRevertResult, message.RequestID, message.SessionID, result)
}
