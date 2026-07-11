package service

import (
	"context"
	"errors"
	"strings"

	queryapi "myai/core/application/session/query/api"
	querycommand "myai/core/application/session/query/command"
	queryport "myai/core/application/session/query/port"
	sessionresult "myai/core/application/session/result"
)

type SessionQueryService struct {
	// 查询服务只做仓库 Record 到应用 Result 的映射，不改变 Session 运行态。
	Sessions queryport.SessionListRepository
	Assets   queryport.SessionAssetRepository
}

var _ queryapi.SessionQueryService = SessionQueryService{}

func (s SessionQueryService) ListSessions(ctx context.Context, includeDeleted bool) ([]sessionresult.SessionListItem, error) {
	if s.Sessions == nil {
		return nil, nil
	}
	records, err := s.Sessions.ListSessionsWithDeleted(ctx, includeDeleted)
	if err != nil {
		return nil, err
	}
	return SessionListItems(records), nil
}

func (s SessionQueryService) ListAssets(ctx context.Context, command querycommand.ListAssets) ([]sessionresult.AssetListItem, error) {
	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	if s.Assets == nil {
		return nil, nil
	}
	records, err := s.Assets.ListAssets(ctx, sessionID, command.Limit)
	if err != nil {
		return nil, err
	}
	return AssetListItems(records), nil
}
