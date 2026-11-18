package uploads

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"myai/core/port/files"
)

const (
	ChunkSize       = 256 * 1024
	MaxJSONBytes    = 512 * 1024
	MaxActiveOwner  = 3
	MaxActiveGlobal = 8
	MaxDiskBytes    = 200 * 1024 * 1024
	maxReceipts     = 256
)

// Owner contains only identities verified by Relay, including the paired client
// token's hash. Session IDs and upload IDs alone never confer upload access.
type Owner struct {
	UserID    string
	DeviceID  string
	SessionID string
	ClientID  string
}

type Metadata struct {
	FileName       string
	ContentType    string
	Size           int64
	IdempotencyKey string
	// SessionEpoch is captured by host admission and never accepted from wire JSON.
	SessionEpoch uint64
}

// Recorder must be idempotent for the same session/request/epoch/asset. A failed
// recording is retried by Complete without uploading the file again.
type Recorder func(context.Context, string, string, uint64, files.Asset) error

type Config struct {
	TempDir           string
	UploadTTL         time.Duration
	ReceiptTTL        time.Duration
	CompletionTimeout time.Duration
	CleanupInterval   time.Duration
}

type Result struct {
	UploadID string
	Offset   int64
	Asset    *files.Asset
}

type Completion struct {
	done   chan struct{}
	result Result
	err    error
}

func (c *Completion) Wait(ctx context.Context) (Result, error) {
	// Prefer a saved receipt even when its connection was cancelled at the same
	// instant that the backend committed the upload.
	select {
	case <-c.done:
		return c.result, c.err
	default:
	}
	select {
	case <-c.done:
		return c.result, c.err
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

type idempotencyKey struct {
	Owner Owner
	Key   string
}

type upload struct {
	id         string
	owner      Owner
	metadata   Metadata
	backendKey string
	file       *os.File
	offset     int64
	// One bit per byte marks accepted chunk boundaries. This permits arbitrary
	// small chunks and exact retries without an unbounded entry per chunk.
	boundaries     []byte
	lastActivity   time.Time
	terminalAt     time.Time
	reserved       bool
	cancelled      bool
	asset          *files.Asset
	recorded       bool
	requestID      string
	completion     *Completion
	cancelWorker   context.CancelFunc
	stopDisconnect func() bool
}

// Receiver owns private staging files and bounded, expiring receipts. Disk space
// is reserved using declared sizes before a file is created. Backend work never
// runs under the receiver lock, and at most eight completion workers may run.
type Receiver struct {
	mu        sync.Mutex
	service   files.Service
	recorder  Recorder
	config    Config
	uploads   map[string]*upload
	keys      map[idempotencyKey]string
	dir       string
	active    int
	reserved  int64
	workers   int
	closed    bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

func New(service files.Service, recorder Recorder, config Config) *Receiver {
	if config.UploadTTL <= 0 {
		config.UploadTTL = 10 * time.Minute
	}
	if config.ReceiptTTL <= 0 {
		config.ReceiptTTL = 15 * time.Minute
	}
	if config.CompletionTimeout <= 0 {
		config.CompletionTimeout = 5 * time.Minute
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Receiver{
		service: service, recorder: recorder, config: config,
		uploads: make(map[string]*upload), keys: make(map[idempotencyKey]string),
		ctx: ctx, cancel: cancel,
	}
	r.wg.Add(1)
	go r.cleanupLoop()
	return r
}

func validateOwner(owner Owner) error {
	for _, value := range []string{owner.UserID, owner.DeviceID, owner.SessionID, owner.ClientID} {
		if strings.TrimSpace(value) == "" || len(value) > 512 {
			return errors.New("authenticated upload owner and session are required")
		}
	}
	return nil
}

func validateMetadata(metadata Metadata) error {
	if metadata.Size <= 0 || metadata.Size > files.MaxFileBytes {
		return fmt.Errorf("file size must be between 1 and %d bytes", files.MaxFileBytes)
	}
	if strings.TrimSpace(metadata.FileName) == "" || len(metadata.FileName) > 255 ||
		!utf8.ValidString(metadata.FileName) || metadata.FileName == "." || metadata.FileName == ".." ||
		strings.ContainsAny(metadata.FileName, `/\\:`) || strings.IndexFunc(metadata.FileName, unicode.IsControl) >= 0 {
		return errors.New("file_name must be a file name without a path")
	}
	if len(metadata.ContentType) > 255 || strings.IndexFunc(metadata.ContentType, unicode.IsControl) >= 0 {
		return errors.New("invalid content_type")
	}
	if metadata.ContentType != "" {
		mediaType, _, err := mime.ParseMediaType(metadata.ContentType)
		if err != nil || !strings.Contains(mediaType, "/") {
			return errors.New("invalid content_type")
		}
	}
	if strings.TrimSpace(metadata.IdempotencyKey) == "" || len(metadata.IdempotencyKey) > 256 ||
		strings.IndexFunc(metadata.IdempotencyKey, unicode.IsControl) >= 0 {
		return errors.New("idempotency_key is required and must be at most 256 bytes")
	}
	return nil
}

func (r *Receiver) Begin(ctx context.Context, owner Owner, metadata Metadata) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := validateOwner(owner); err != nil {
		return Result{}, err
	}
	if err := validateMetadata(metadata); err != nil {
		return Result{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.service == nil {
		return Result{}, errors.New("file upload service is unavailable")
	}
	r.pruneLocked(time.Now())
	key := idempotencyKey{Owner: owner, Key: metadata.IdempotencyKey}
	if previous := r.uploads[r.keys[key]]; previous != nil {
		if previous.metadata != metadata {
			return Result{}, errors.New("idempotency_key was already used with different upload metadata")
		}
		if previous.cancelled && previous.asset == nil {
			return Result{}, errors.New("upload was cancelled; use a new idempotency_key")
		}
		previous.lastActivity = time.Now()
		return resultFor(previous), nil
	}
	ownerActive, globalActive := r.activeCountsLocked(owner)
	if ownerActive >= MaxActiveOwner || globalActive >= MaxActiveGlobal || metadata.Size > MaxDiskBytes-r.reserved {
		return Result{}, errors.New("too many active uploads or upload disk quota exceeded")
	}
	if r.dir == "" {
		dir, err := os.MkdirTemp(r.config.TempDir, "myai-uploads-*")
		if err != nil {
			return Result{}, fmt.Errorf("create private upload directory: %w", err)
		}
		if err := os.Chmod(dir, 0700); err != nil {
			_ = os.Remove(dir)
			return Result{}, err
		}
		r.dir = dir
	}
	file, err := os.CreateTemp(r.dir, "asset-*")
	if err != nil {
		return Result{}, fmt.Errorf("create private upload file: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return Result{}, err
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return Result{}, err
	}
	// Scope backend idempotency to the authenticated pair, client, and session.
	// Never forward a client-controlled global backend idempotency key.
	encodedKey, _ := json.Marshal(key)
	digest := sha256.Sum256(encodedKey)
	u := &upload{
		id: hex.EncodeToString(random[:]), owner: owner, metadata: metadata,
		backendKey: "remote-upload-" + hex.EncodeToString(digest[:]),
		file:       file, reserved: true, lastActivity: time.Now(),
		boundaries: make([]byte, metadata.Size/8+1),
	}
	u.boundaries[0] = 1
	r.uploads[u.id], r.keys[key] = u, u.id
	r.active++
	r.reserved += metadata.Size
	u.stopDisconnect = context.AfterFunc(ctx, func() { r.disconnect(u.id, owner) })
	return resultFor(u), nil
}

func (r *Receiver) activeCountsLocked(owner Owner) (int, int) {
	ownerActive, globalActive := 0, 0
	for _, current := range r.uploads {
		if current.reserved || current.completion != nil {
			globalActive++
			if current.owner == owner {
				ownerActive++
			}
		}
	}
	return ownerActive, globalActive
}

func (r *Receiver) findLocked(owner Owner, id string) (*upload, error) {
	if r.closed {
		return nil, errors.New("file upload receiver is closed")
	}
	u := r.uploads[id]
	if u == nil || u.owner != owner {
		return nil, errors.New("upload not found for this owner")
	}
	return u, nil
}

func (r *Receiver) Chunk(ctx context.Context, owner Owner, id string, offset int64, data string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if len(data) == 0 || len(data) > base64.StdEncoding.EncodedLen(ChunkSize) || strings.ContainsAny(data, "\r\n") {
		return Result{}, errors.New("upload chunk must contain between 1 and 262144 decoded bytes")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(data)
	if err != nil || len(decoded) == 0 || len(decoded) > ChunkSize {
		return Result{}, errors.New("invalid base64 upload chunk or chunk exceeds 262144 bytes")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(time.Now())
	u, err := r.findLocked(owner, id)
	if err != nil {
		return Result{}, err
	}
	if u.cancelled || u.asset != nil || u.completion != nil {
		return Result{}, errors.New("upload is cancelled or already completing")
	}
	if offset < 0 || offset > u.offset || offset > u.metadata.Size || int64(len(decoded)) > u.metadata.Size-offset {
		return Result{}, errors.New("upload chunk has an unexpected offset or exceeds the declared file size")
	}
	if offset < u.offset {
		end := offset + int64(len(decoded))
		if end > u.offset || !u.boundary(offset) || !u.boundary(end) {
			return Result{}, errors.New("upload chunk is not an exact duplicate")
		}
		for position := offset + 1; position < end; position++ {
			if u.boundary(position) {
				return Result{}, errors.New("upload chunk is not an exact duplicate")
			}
		}
		previous := make([]byte, len(decoded))
		if _, err := u.file.ReadAt(previous, offset); err != nil {
			return Result{}, err
		}
		if !equalBytes(previous, decoded) {
			return Result{}, errors.New("duplicate upload chunk has different content")
		}
	} else {
		n, err := u.file.WriteAt(decoded, offset)
		if err != nil || n != len(decoded) {
			// A partial disk write cannot be acknowledged or retried as a complete
			// chunk. Release the staging file instead of leaving ambiguous bytes.
			r.cancelLocked(u)
			if err == nil {
				err = io.ErrShortWrite
			}
			return Result{}, fmt.Errorf("write upload chunk: %w", err)
		}
		u.offset += int64(n)
		u.boundaries[u.offset/8] |= 1 << uint(u.offset%8)
	}
	u.lastActivity = time.Now()
	return resultFor(u), nil
}

func (u *upload) boundary(position int64) bool {
	return u.boundaries[position/8]&(1<<uint(position%8)) != 0
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *Receiver) Status(ctx context.Context, owner Owner, id string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(time.Now())
	u, err := r.findLocked(owner, id)
	if err != nil {
		return Result{}, err
	}
	if u.cancelled && u.asset == nil {
		return Result{}, errors.New("upload was cancelled")
	}
	u.lastActivity = time.Now()
	return resultFor(u), nil
}

// Complete admits backend work synchronously and returns immediately. Repeated
// completion requests share a running worker or a saved receipt. Failed backend
// attempts retain their staging file for retry with the same backend key.
func (r *Receiver) Complete(ctx context.Context, owner Owner, id, requestID string) (*Completion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(requestID) == "" {
		return nil, errors.New("upload completion request_id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(time.Now())
	u, err := r.findLocked(owner, id)
	if err != nil {
		return nil, err
	}
	if u.asset != nil && u.recorded {
		completion := &Completion{done: make(chan struct{}), result: resultFor(u)}
		close(completion.done)
		return completion, nil
	}
	if u.completion != nil {
		return u.completion, nil
	}
	if u.cancelled && u.asset == nil {
		return nil, errors.New("upload was cancelled")
	}
	if u.offset != u.metadata.Size {
		return nil, errors.New("upload is incomplete")
	}
	if r.workers >= MaxActiveGlobal {
		return nil, errors.New("too many active upload completion workers; retry later")
	}
	if !u.reserved {
		ownerActive, globalActive := r.activeCountsLocked(owner)
		if ownerActive >= MaxActiveOwner || globalActive >= MaxActiveGlobal {
			return nil, errors.New("too many active uploads; retry recording later")
		}
	}
	if u.requestID == "" {
		u.requestID = requestID
	}
	workerCtx, cancel := context.WithTimeout(ctx, r.config.CompletionTimeout)
	stop := context.AfterFunc(r.ctx, cancel)
	u.cancelWorker = cancel
	u.lastActivity = time.Now()
	completion := &Completion{done: make(chan struct{})}
	u.completion = completion
	r.workers++
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()
		defer stop()
		r.finish(workerCtx, u, completion)
	}()
	return completion, nil
}

func (r *Receiver) finish(ctx context.Context, u *upload, completion *Completion) {
	// The file and metadata are immutable while completion is running. Cancel
	// only signals this worker; it never closes a reader used by the backend.
	var err error
	if u.asset == nil {
		var asset files.Asset
		asset, err = r.service.Upload(ctx, files.UploadRequest{
			Reader:   io.NewSectionReader(u.file, 0, u.metadata.Size),
			FileName: u.metadata.FileName, ContentType: u.metadata.ContentType,
			Size: u.metadata.Size, IdempotencyKey: u.backendKey,
		})
		if err == nil && strings.TrimSpace(asset.AssetID) == "" {
			err = errors.New("upload backend returned an empty asset_id")
		}
		if err == nil {
			if asset.FileName == "" {
				asset.FileName = u.metadata.FileName
			}
			if asset.ContentType == "" {
				asset.ContentType = u.metadata.ContentType
			}
			asset.Size = u.metadata.Size
			r.mu.Lock()
			u.asset = &asset
			// Preserve successful backend receipts even if a disconnect/cancel
			// raced with the backend response. Recorder retries never reupload.
			u.cancelled = false
			r.mu.Unlock()
		}
	}
	if err == nil && r.recorder != nil {
		if err = ctx.Err(); err == nil {
			err = r.recorder(ctx, u.owner.SessionID, u.requestID, u.metadata.SessionEpoch, *u.asset)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if u.asset != nil {
		r.releaseFileLocked(u)
		u.terminalAt = time.Now()
	}
	if err == nil {
		u.recorded = true
	} else if u.asset == nil && (u.cancelled || ctx.Err() != nil || r.closed) {
		u.cancelled = true
		r.releaseFileLocked(u)
		u.terminalAt = time.Now()
	}
	u.cancelWorker = nil
	u.completion = nil
	r.workers--
	completion.result, completion.err = resultFor(u), err
	close(completion.done)
	r.pruneLocked(time.Now())
}

func (r *Receiver) Cancel(ctx context.Context, owner Owner, id string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(time.Now())
	u, err := r.findLocked(owner, id)
	if err != nil {
		return Result{}, err
	}
	// A committed backend upload cannot be undone through this port. Keep it
	// for recorder retries, but expose its receipt only after recording succeeds.
	if u.asset == nil {
		r.cancelLocked(u)
	}
	return resultFor(u), nil
}

func (r *Receiver) disconnect(id string, owner Owner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u := r.uploads[id]; u != nil && u.owner == owner && u.asset == nil {
		r.cancelLocked(u)
	}
}

func (r *Receiver) cancelLocked(u *upload) {
	u.cancelled = true
	if u.stopDisconnect != nil {
		u.stopDisconnect()
		u.stopDisconnect = nil
	}
	if u.cancelWorker != nil {
		u.cancelWorker()
	} else {
		r.releaseFileLocked(u)
		u.terminalAt = time.Now()
	}
}

func (r *Receiver) releaseFileLocked(u *upload) {
	u.boundaries = nil
	if u.stopDisconnect != nil {
		u.stopDisconnect()
		u.stopDisconnect = nil
	}
	if u.file != nil {
		name := u.file.Name()
		_ = u.file.Close()
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			// Keep the reservation until deletion succeeds. Cleanup retries a
			// temporarily locked file instead of silently exceeding disk quota.
			return
		}
		u.file = nil
	}
	if u.reserved {
		u.reserved = false
		r.active--
		r.reserved -= u.metadata.Size
	}
}

func resultFor(u *upload) Result {
	result := Result{UploadID: u.id, Offset: u.offset}
	// A backend commit is retained internally for recorder retries. Only a
	// recorded asset is a successful receipt visible to the client.
	if u.asset != nil && u.recorded {
		copy := *u.asset
		result.Asset = &copy
	}
	return result
}

func (r *Receiver) pruneLocked(now time.Time) {
	for id, u := range r.uploads {
		if u.reserved && u.completion == nil && (u.cancelled || u.asset != nil) {
			r.releaseFileLocked(u)
		}
		if u.reserved && u.asset == nil && now.Sub(u.lastActivity) >= r.config.UploadTTL && !u.cancelled {
			r.cancelLocked(u)
		}
		if !u.reserved && u.completion == nil && !u.terminalAt.IsZero() && now.Sub(u.terminalAt) >= r.config.ReceiptTTL {
			delete(r.uploads, id)
			delete(r.keys, idempotencyKey{Owner: u.owner, Key: u.metadata.IdempotencyKey})
		}
	}
	// Receipts are bounded even if clients complete/cancel faster than TTL.
	for len(r.uploads)-r.active > maxReceipts {
		var oldest *upload
		for _, u := range r.uploads {
			if !u.reserved && u.completion == nil && (oldest == nil || u.terminalAt.Before(oldest.terminalAt)) {
				oldest = u
			}
		}
		if oldest == nil {
			break
		}
		delete(r.uploads, oldest.id)
		delete(r.keys, idempotencyKey{Owner: oldest.owner, Key: oldest.metadata.IdempotencyKey})
	}
}

func (r *Receiver) cleanupLoop() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.config.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case now := <-ticker.C:
			r.mu.Lock()
			r.pruneLocked(now)
			r.mu.Unlock()
		}
	}
}

// Close cancels and joins all workers before removing the private directory.
// files.Service implementations must observe their context's cancellation.
func (r *Receiver) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.cancel()
		for _, u := range r.uploads {
			if u.asset == nil {
				r.cancelLocked(u)
			} else if u.cancelWorker != nil {
				u.cancelWorker()
			}
		}
		r.mu.Unlock()
		r.wg.Wait()
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, u := range r.uploads {
			r.releaseFileLocked(u)
		}
		if r.dir != "" {
			r.closeErr = os.RemoveAll(r.dir)
		}
		r.uploads = nil
		r.keys = nil
	})
	return r.closeErr
}
