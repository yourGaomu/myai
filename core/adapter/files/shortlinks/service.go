package shortlinks

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"myai/core/mcp"
	filesport "myai/core/port/files"
)

const serverName = "short_links"

// Service transfers files only through the configured, managed short_links MCP.
// The provider is evaluated per operation because the application initializes
// the manager after constructing its file service.
type Service struct {
	provider    func() *mcp.Manager
	mu          sync.Mutex
	receipts    map[string]*uploadReceipt
	uploadSlots chan struct{}
	lifecycle   context.Context
	shutdown    context.CancelFunc
	closed      bool
	closeDone   chan struct{}
}

var _ filesport.Service = (*Service)(nil)

func New(provider func() *mcp.Manager) *Service { return &Service{provider: provider} }

func (s *Service) uploadSlot(ctx context.Context) (func(), error) {
	if s == nil {
		return nil, errors.New("short_links file service is unavailable")
	}
	s.mu.Lock()
	if s.uploadSlots == nil {
		s.uploadSlots = make(chan struct{}, 4)
	}
	slots := s.uploadSlots
	s.mu.Unlock()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) acquire(ctx context.Context, tools ...string) (*mcp.ServerLease, error) {
	if s == nil || s.provider == nil {
		return nil, errors.New("short_links file service is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, errors.New("short_links file service is closed")
	}
	manager := s.provider()
	if manager == nil {
		return nil, errors.New("short_links MCP manager is unavailable")
	}
	lease, err := manager.AcquireServer(ctx, serverName)
	if err != nil {
		return nil, safeError("acquire short_links MCP", err)
	}
	for _, name := range tools {
		if !lease.HasTool(name) {
			_ = lease.Close()
			return nil, errors.New("short_links MCP is missing a required file tool")
		}
	}
	return lease, nil
}

func (s *Service) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	if s == nil {
		cancel()
		return ctx, cancel
	}
	s.mu.Lock()
	if s.lifecycle == nil {
		s.lifecycle, s.shutdown = context.WithCancel(context.Background())
	}
	lifecycle, closed := s.lifecycle, s.closed
	s.mu.Unlock()
	stop := context.AfterFunc(lifecycle, cancel)
	if closed {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}

// Close cancels active operations, rejects new work and releases retained MCP
// leases. Call it before closing the manager during application shutdown.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closeDone != nil {
		done := s.closeDone
		s.mu.Unlock()
		<-done
		return nil
	}
	s.closed = true
	s.closeDone = make(chan struct{})
	done := s.closeDone
	shutdown := s.shutdown
	receipts := make([]*uploadReceipt, 0, len(s.receipts))
	for _, receipt := range s.receipts {
		receipts = append(receipts, receipt)
	}
	s.receipts = nil
	s.mu.Unlock()
	if shutdown != nil {
		shutdown()
	}
	for _, receipt := range receipts {
		// The gate owns every lease access. Waiting also joins an active transfer's
		// deferred cleanup after lifecycle cancellation, without holding Service.mu.
		<-receipt.gate
		if receipt.lease != nil {
			_ = receipt.lease.Close()
			receipt.lease = nil
		}
		receipt.gate <- struct{}{}
	}
	close(done)
	return nil
}

// Backend response text, RPC errors, signed URLs and transport errors are never
// interpolated into outward-facing errors. Only standard cancellation survives.
func safeError(operation string, err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: %w", operation, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", operation, context.DeadlineExceeded)
	}
	return fmt.Errorf("%s failed", operation)
}

type callFailure struct{ uncertain bool }

func (e *callFailure) Error() string { return "short_links MCP tool call failed" }

func call(ctx context.Context, lease *mcp.ServerLease, name string, args any, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return errors.New("invalid file tool arguments")
	}
	result, err := lease.CallTool(ctx, name, payload)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &callFailure{uncertain: true}
	}
	if result.IsError {
		return &callFailure{}
	}
	raw := bytes.TrimSpace(result.StructuredContent)
	if len(raw) < 2 || raw[0] != '{' || !json.Valid(raw) || json.Unmarshal(raw, out) != nil {
		return errors.New("invalid short_links structured result")
	}
	return nil
}

func uncertainCall(err error) bool {
	var failure *callFailure
	return errors.As(err, &failure) && failure.uncertain
}

type assetDTO struct {
	AssetID        string     `json:"assetId"`
	BusinessID     string     `json:"businessId"`
	FileName       string     `json:"fileName"`
	ContentType    string     `json:"contentType"`
	SizeBytes      *int64     `json:"sizeBytes"`
	ChecksumSHA256 string     `json:"checksumSha256"`
	DeletedAt      *time.Time `json:"deletedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
}

func validID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func validFileName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 512 &&
		!strings.ContainsAny(name, "/\\") && !strings.ContainsFunc(name, unicode.IsControl)
}

func (dto assetDTO) asset(expectedID string) (filesport.Asset, error) {
	if !validID(dto.AssetID) || expectedID != "" && dto.AssetID != expectedID ||
		!validID(dto.BusinessID) || !validFileName(dto.FileName) || dto.SizeBytes == nil ||
		*dto.SizeBytes <= 0 || *dto.SizeBytes > filesport.MaxFileBytes || dto.DeletedAt != nil || dto.CreatedAt.IsZero() {
		return filesport.Asset{}, errors.New("invalid short_links asset identity or size")
	}
	if mediaType, _, err := mime.ParseMediaType(dto.ContentType); err != nil || mediaType == "" {
		return filesport.Asset{}, errors.New("invalid short_links asset content type")
	}
	if dto.ChecksumSHA256 != "" {
		decoded, err := hex.DecodeString(dto.ChecksumSHA256)
		if err != nil || len(decoded) != 32 {
			return filesport.Asset{}, errors.New("invalid short_links asset checksum")
		}
	}
	return filesport.Asset{
		AssetID: dto.AssetID, BusinessID: dto.BusinessID, FileName: dto.FileName,
		ContentType: dto.ContentType, Size: *dto.SizeBytes, Checksum: dto.ChecksumSHA256, CreatedAt: dto.CreatedAt,
	}, nil
}

func (s *Service) Info(ctx context.Context, assetID string) (filesport.Asset, error) {
	if !validID(assetID) {
		return filesport.Asset{}, errors.New("a valid asset ID is required")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	lease, err := s.acquire(ctx, "get_asset_info")
	if err != nil {
		return filesport.Asset{}, err
	}
	defer lease.Close()
	var dto assetDTO
	if err := call(ctx, lease, "get_asset_info", map[string]any{"assetId": assetID}, &dto); err != nil {
		return filesport.Asset{}, safeError("get asset info", err)
	}
	return dto.asset(assetID)
}

func readUpload(ctx context.Context, request filesport.UploadRequest) ([]byte, filesport.UploadRequest, error) {
	request.FileName = strings.TrimSpace(request.FileName)
	if request.Reader == nil || !validFileName(request.FileName) || request.Size <= 0 || request.Size > filesport.MaxFileBytes {
		return nil, request, errors.New("file requires a name, reader and size between 1 byte and 50 MiB")
	}
	if len(request.IdempotencyKey) > 128 || strings.ContainsFunc(request.IdempotencyKey, unicode.IsControl) {
		return nil, request, errors.New("invalid file upload idempotency key")
	}
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, request.Reader}, request.Size+1))
	if err != nil {
		return nil, request, safeError("read upload file", err)
	}
	if int64(len(data)) != request.Size {
		return nil, request, errors.New("upload file size does not match the declared size")
	}
	if request.ContentType == "" {
		request.ContentType = http.DetectContentType(data)
	}
	if mediaType, _, err := mime.ParseMediaType(request.ContentType); err != nil || mediaType == "" || strings.ContainsAny(request.ContentType, "\r\n") {
		return nil, request, errors.New("invalid file content type")
	}
	return data, request, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if contextErr := r.ctx.Err(); contextErr != nil {
		return n, contextErr
	}
	return n, err
}

func validShortURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 4096 && (u.Scheme == "https" || u.Scheme == "http") &&
		u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Opaque == ""
}
