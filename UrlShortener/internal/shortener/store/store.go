package store

import (
	"context"
	"errors"
	"myai-url-shortener/internal/shortener"
)

var (
	ErrLinkNotFound    = errors.New("link not found")
	ErrCodeExists      = errors.New("code already exists")
	ErrLinkExpired     = errors.New("link is expired")
	ErrVisitsExhausted = errors.New("link max visits exhausted")
)

type Store interface {
	Create(ctx context.Context, link shortener.Link) error
	Get(ctx context.Context, code string) (shortener.Link, error)
	IncrementVisits(ctx context.Context, code string) (shortener.Link, error)
	List(ctx context.Context) ([]shortener.Link, error)
	Delete(ctx context.Context, code string) (shortener.Link, error)
}
