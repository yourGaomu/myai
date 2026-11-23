package shortlinks

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"myai/core/mcp"
	filesport "myai/core/port/files"
)

const (
	maxUploadReceipts = 256
	uploadReceiptTTL  = 15 * time.Minute
)

// A known session is never discarded while its outcome is uncertain. The
// backend only deduplicates unfinished uploads, so forgetting a completed but
// unacknowledged session and repeating begin could create a second asset.
// Once the bounded table fills with uncertain operations, new explicit keys
// fail before side effects. Completed receipts can expire after fifteen minutes.
type uploadReceipt struct {
	fingerprint   [32]byte
	gate          chan struct{}
	users         int       // guarded by Service.mu
	finishedAt    time.Time // guarded by Service.mu
	disposable    bool      // guarded by Service.mu
	lease         *mcp.ServerLease
	session       *uploadSession
	started       bool
	request       filesport.UploadRequest // normalized metadata, without a reader
	checksum      [32]byte
	terminalError bool
	cancelPending bool
	asset         filesport.Asset
	shareTried    bool
	shareKey      string
	shared        filesport.Asset
}

func (s *Service) receipt(ctx context.Context, request filesport.UploadRequest, data []byte) (*uploadReceipt, func(), error) {
	checksum := sha256.Sum256(data)
	metadata, _ := json.Marshal(struct {
		Name, ContentType string
		Size              int64
		Checksum          [32]byte
	}{request.FileName, request.ContentType, request.Size, checksum})
	fingerprint := sha256.Sum256(metadata)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, nil, errors.New("short_links file service is closed")
	}
	if s.receipts == nil {
		s.receipts = make(map[string]*uploadReceipt)
	}
	for key, old := range s.receipts {
		if old.users == 0 && !old.finishedAt.IsZero() && time.Since(old.finishedAt) >= uploadReceiptTTL {
			delete(s.receipts, key)
		}
	}
	receipt := s.receipts[request.IdempotencyKey]
	if receipt == nil {
		if len(s.receipts) >= maxUploadReceipts {
			s.mu.Unlock()
			return nil, nil, errors.New("file upload recovery table is full; resume an existing upload first")
		}
		metadata := request
		metadata.Reader = nil
		receipt = &uploadReceipt{fingerprint: fingerprint, request: metadata, checksum: checksum, gate: make(chan struct{}, 1)}
		receipt.gate <- struct{}{}
		s.receipts[request.IdempotencyKey] = receipt
	}
	if receipt.fingerprint != fingerprint {
		s.mu.Unlock()
		return nil, nil, errors.New("file upload idempotency key was already used for different content or metadata")
	}
	return s.lockReceipt(ctx, request.IdempotencyKey, receipt)
}

// lockReceipt is entered with Service.mu held. Reference counting prevents
// eviction while a caller is waiting for or holding the per-upload gate.
func (s *Service) lockReceipt(ctx context.Context, key string, receipt *uploadReceipt) (*uploadReceipt, func(), error) {
	receipt.users++
	s.mu.Unlock()
	dropUser := func() {
		s.mu.Lock()
		receipt.users--
		if receipt.disposable && receipt.users == 0 && s.receipts[key] == receipt {
			delete(s.receipts, key)
		}
		s.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		dropUser()
		return nil, nil, ctx.Err()
	case <-receipt.gate:
	}
	return receipt, func() {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed || receipt.asset.AssetID != "" || receipt.terminalError || !receipt.started && receipt.session == nil {
			if receipt.lease != nil {
				_ = receipt.lease.Close()
				receipt.lease = nil
			}
			s.mu.Lock()
			if !receipt.started && receipt.session == nil {
				receipt.disposable = true
			} else if receipt.finishedAt.IsZero() {
				receipt.finishedAt = time.Now()
			}
			s.mu.Unlock()
		}
		receipt.gate <- struct{}{}
		dropUser()
	}, nil
}

func (r *uploadReceipt) remember(session uploadSession) {
	r.session = &session
	if session.terminal() {
		r.terminalError = true
	}
}

var _ filesport.UploadCanceller = (*Service)(nil)

func (s *Service) CancelUpload(ctx context.Context, key string) (filesport.Asset, error) {
	if s == nil || key == "" {
		return filesport.Asset{}, errors.New("file upload cancellation requires an internal key")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	releaseSlot, err := s.uploadSlot(ctx)
	if err != nil {
		return filesport.Asset{}, err
	}
	defer releaseSlot()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return filesport.Asset{}, errors.New("short_links file service is closed")
	}
	receipt := s.receipts[key]
	if receipt == nil {
		s.mu.Unlock()
		return filesport.Asset{}, nil
	}
	receipt, done, err := s.lockReceipt(ctx, key, receipt)
	if err != nil {
		return filesport.Asset{}, err
	}
	defer done()
	if receipt.terminalError || !receipt.started || receipt.asset.AssetID != "" {
		return receipt.asset, nil
	}
	receipt.cancelPending = true
	lease, request := receipt.lease, receipt.request
	if lease == nil || !lease.HasTool("cancel_asset_upload") {
		return filesport.Asset{}, errors.New("short_links MCP cannot cancel the existing upload")
	}
	if receipt.session == nil {
		// No parts or completion were dispatched. Resolve the lost begin using
		// its original key, then clean up the session the backend identifies.
		if _, err := uploadBegin(ctx, lease, request, receipt); err != nil {
			return filesport.Asset{}, err
		}
	}
	previous := *receipt.session
	if previous.terminal() {
		return filesport.Asset{}, nil
	}
	next := previous
	if previous.Status != "completed" {
		next = uploadSession{}
		err = call(ctx, lease, "cancel_asset_upload", map[string]any{"uploadSessionId": previous.UploadSessionID}, &next)
		if err == nil {
			err = next.validate(request, &previous)
		}
		if err != nil || !next.terminal() && next.Status != "completed" {
			// The backend reports a completion race as a tool error. A status
			// query also resolves a lost cancel acknowledgement safely.
			next, err = uploadStatus(ctx, lease, request, previous)
			if err != nil {
				return filesport.Asset{}, safeError("confirm existing file upload cancellation", err)
			}
		}
	}
	receipt.remember(next)
	if next.terminal() {
		return filesport.Asset{}, nil
	}
	if next.Status != "completed" {
		return filesport.Asset{}, errors.New("file upload cancellation is pending; retry cancellation")
	}
	asset, err := next.checkedAsset(receipt.checksum)
	if err == nil {
		receipt.asset = asset
	}
	return asset, err
}

// transfer retains the gate and lease until done is called, including the
// publication step in Share. Only explicit caller keys need cross-call receipts.
func (s *Service) transfer(ctx context.Context, request filesport.UploadRequest, data []byte, tools []string) (filesport.Asset, *mcp.ServerLease, *uploadReceipt, func(), error) {
	if request.IdempotencyKey == "" {
		lease, err := s.acquire(ctx, tools...)
		if err != nil {
			return filesport.Asset{}, nil, nil, func() {}, err
		}
		asset, err := upload(ctx, lease, request, data, nil)
		return asset, lease, nil, func() { _ = lease.Close() }, err
	}
	if s == nil {
		return filesport.Asset{}, nil, nil, func() {}, errors.New("short_links file service is unavailable")
	}
	receipt, done, err := s.receipt(ctx, request, data)
	if err != nil {
		return filesport.Asset{}, nil, nil, func() {}, err
	}
	if receipt.terminalError {
		return filesport.Asset{}, nil, receipt, done, errors.New("previous file upload session is terminal; this idempotency key cannot create a new session")
	}
	if receipt.asset.AssetID != "" && len(tools) == len(uploadTools) {
		return receipt.asset, nil, receipt, done, nil
	}
	if receipt.cancelPending && receipt.asset.AssetID == "" {
		return filesport.Asset{}, nil, receipt, done, errors.New("file upload cancellation is pending; retry cancellation")
	}
	if receipt.lease == nil {
		receipt.lease, err = s.acquire(ctx, tools...)
		if err != nil {
			return receipt.asset, nil, receipt, done, err
		}
	}
	if err := requireTools(receipt.lease, tools); err != nil {
		return receipt.asset, receipt.lease, receipt, done, err
	}
	if receipt.asset.AssetID != "" {
		return receipt.asset, receipt.lease, receipt, done, nil
	}
	s.mu.Lock()
	receipt.disposable = false
	s.mu.Unlock()
	asset, err := upload(ctx, receipt.lease, request, data, receipt)
	if err == nil {
		receipt.asset = asset
	}
	return asset, receipt.lease, receipt, done, err
}
