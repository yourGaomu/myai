package uploads

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"myai/core/port/files"
)

type fakeService struct {
	files.Service
	upload func(context.Context, files.UploadRequest) (files.Asset, error)
	calls  atomic.Int32
}

func (s *fakeService) Upload(ctx context.Context, request files.UploadRequest) (files.Asset, error) {
	s.calls.Add(1)
	if s.upload != nil {
		return s.upload(ctx, request)
	}
	_, err := io.Copy(io.Discard, request.Reader)
	return files.Asset{AssetID: "asset-1"}, err
}

func testOwner() Owner {
	return Owner{UserID: "user", DeviceID: "device", SessionID: "session", ClientID: "paired-client"}
}

type cancellingFakeService struct {
	*fakeService
	cancel func(context.Context, string) (files.Asset, error)
}

func (s *cancellingFakeService) CancelUpload(ctx context.Context, key string) (files.Asset, error) {
	return s.cancel(ctx, key)
}

func testReceiver(t *testing.T, service files.Service, recorder Recorder, config Config) *Receiver {
	t.Helper()
	if config.TempDir == "" {
		config.TempDir = t.TempDir()
	}
	r := New(service, recorder, config)
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	return r
}

func beginUpload(t *testing.T, r *Receiver, owner Owner, key string, data []byte) Result {
	t.Helper()
	result, err := r.Begin(context.Background(), owner, Metadata{FileName: "report.txt", ContentType: "text/plain", Size: int64(len(data)), IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		result, err = r.Chunk(context.Background(), owner, result.UploadID, 0, base64.StdEncoding.EncodeToString(data))
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func completeUpload(t *testing.T, r *Receiver, owner Owner, id, request string) (Result, error) {
	t.Helper()
	completion, err := r.Complete(context.Background(), owner, id, request)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return completion.Wait(ctx)
}

type preflightFakeService struct {
	*fakeService
	preflight func(context.Context) error
}

func (s *preflightFakeService) PreflightUpload(ctx context.Context) error {
	return s.preflight(ctx)
}

func TestReceiverCancelledPreflightDoesNotStageUpload(t *testing.T) {
	entered := make(chan struct{})
	service := &preflightFakeService{fakeService: &fakeService{}, preflight: func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}}
	r := testReceiver(t, service, nil, Config{})
	metadata := Metadata{FileName: "report.txt", Size: 1, IdempotencyKey: "key"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := r.Begin(ctx, testOwner(), metadata)
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("upload did not start preflight")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("preflight cancellation = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not stop preflight")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dir != "" || r.reserved != 0 || len(r.uploads) != 0 || service.calls.Load() != 0 {
		t.Fatal("preflight cancellation left staged data, a receipt, or an upload call")
	}
}

func TestReceiverSequentialChunksAndExactRetries(t *testing.T) {
	data := []byte("abcdefgh")
	service := &fakeService{upload: func(_ context.Context, request files.UploadRequest) (files.Asset, error) {
		got, err := io.ReadAll(request.Reader)
		if err != nil || !bytes.Equal(got, data) || request.Size != int64(len(data)) || request.FileName != "report.txt" || request.ContentType != "text/plain" {
			t.Errorf("wrong backend upload: %q %#v %v", got, request, err)
		}
		if request.IdempotencyKey == "upload-key" || request.IdempotencyKey == "" {
			t.Error("backend idempotency key was not scoped to authenticated owner")
		}
		return files.Asset{AssetID: "asset-1", BusinessID: "business-1", Checksum: "sha256"}, nil
	}}
	r := testReceiver(t, service, nil, Config{})
	owner := testOwner()
	metadata := Metadata{FileName: "report.txt", ContentType: "text/plain", Size: int64(len(data)), IdempotencyKey: "upload-key", SessionEpoch: 17}
	begun, err := r.Begin(context.Background(), owner, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := r.Begin(context.Background(), owner, metadata); err != nil || again.UploadID != begun.UploadID {
		t.Fatalf("begin retry changed upload: %#v %v", again, err)
	}
	changed := metadata
	changed.FileName = "changed.txt"
	if _, err := r.Begin(context.Background(), owner, changed); err == nil {
		t.Fatal("accepted changed metadata with same key")
	}
	changed = metadata
	changed.SessionEpoch++
	if _, err := r.Begin(context.Background(), owner, changed); err == nil {
		t.Fatal("reattached old upload after session epoch changed")
	}
	for _, chunk := range []struct {
		offset int64
		data   string
		ok     bool
	}{
		{1, "abc", false},
		{0, "abcd", true},
		{0, "abcd", true},
		{0, "ABCD", false},
		{1, "bc", false},
		{0, "abcde", false},
		{5, "fgh", false},
		{4, "efghi", false},
		{-1, "a", false},
		{4, "efgh", true},
		{0, "abcdefgh", false},
		{4, "efgh", true},
	} {
		_, err := r.Chunk(context.Background(), owner, begun.UploadID, chunk.offset, base64.StdEncoding.EncodeToString([]byte(chunk.data)))
		if (err == nil) != chunk.ok {
			t.Fatalf("chunk offset=%d data=%q ok=%v err=%v", chunk.offset, chunk.data, chunk.ok, err)
		}
	}
	result, err := completeUpload(t, r, owner, begun.UploadID, "complete-1")
	if err != nil || result.Asset == nil || result.Asset.AssetID != "asset-1" || result.Asset.ShortURL != "" || result.Offset != 8 {
		t.Fatalf("wrong completed receipt: %#v %v", result, err)
	}
	for i := 0; i < 3; i++ {
		retry, err := completeUpload(t, r, owner, begun.UploadID, fmt.Sprintf("complete-retry-%d", i))
		if err != nil || retry.Asset.AssetID != result.Asset.AssetID {
			t.Fatalf("completion receipt lost: %#v %v", retry, err)
		}
	}
	if service.calls.Load() != 1 {
		t.Fatal("complete retries uploaded to backend again")
	}
	if r.active != 0 || r.reserved != 0 {
		t.Fatal("completed upload retained disk reservation")
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("completed upload retained temp file: %v %v", entries, err)
	}
}

func TestReceiverOwnerIsolation(t *testing.T) {
	r := testReceiver(t, &fakeService{}, nil, Config{})
	owner := testOwner()
	result := beginUpload(t, r, owner, "shared-key", []byte("x"))
	for _, change := range []func(*Owner){
		func(o *Owner) { o.UserID = "other" },
		func(o *Owner) { o.DeviceID = "other" },
		func(o *Owner) { o.SessionID = "other" },
		func(o *Owner) { o.ClientID = "other" },
	} {
		other := owner
		change(&other)
		if _, err := r.Status(context.Background(), other, result.UploadID); err == nil {
			t.Fatal("cross-owner status succeeded")
		}
		if _, err := r.Chunk(context.Background(), other, result.UploadID, 0, "eA=="); err == nil {
			t.Fatal("cross-owner chunk succeeded")
		}
		if _, err := r.Complete(context.Background(), other, result.UploadID, "steal"); err == nil {
			t.Fatal("cross-owner complete succeeded")
		}
		if _, err := r.Cancel(context.Background(), other, result.UploadID); err == nil {
			t.Fatal("cross-owner cancel succeeded")
		}
		otherResult := beginUpload(t, r, other, "shared-key", []byte("x"))
		if otherResult.UploadID == result.UploadID {
			t.Fatal("idempotency key crossed authenticated owner")
		}
	}
	if _, err := completeUpload(t, r, owner, result.UploadID, "complete"); err != nil {
		t.Fatal(err)
	}
	other := owner
	other.ClientID = "other-client"
	if _, err := r.Status(context.Background(), other, result.UploadID); err == nil {
		t.Fatal("completed receipt leaked across clients")
	}
}

func TestReceiverRejectsInvalidAndOversizedUploads(t *testing.T) {
	r := testReceiver(t, &fakeService{}, nil, Config{})
	owner := testOwner()
	valid := Metadata{FileName: "report.txt", Size: ChunkSize + 1, IdempotencyKey: "key"}
	for _, change := range []func(*Metadata){
		func(m *Metadata) { m.Size = files.MaxFileBytes + 1 },
		func(m *Metadata) { m.Size = -1 },
		func(m *Metadata) { m.Size = 0 },
		func(m *Metadata) { m.FileName = "../evil" },
		func(m *Metadata) { m.FileName = `C:\evil` },
		func(m *Metadata) { m.FileName = "invalid\x00" },
		func(m *Metadata) { m.ContentType = "invalid" },
		func(m *Metadata) { m.IdempotencyKey = "" },
		func(m *Metadata) { m.IdempotencyKey = strings.Repeat("x", 257) },
	} {
		metadata := valid
		change(&metadata)
		if _, err := r.Begin(context.Background(), owner, metadata); err == nil {
			t.Fatalf("accepted invalid metadata: %#v", metadata)
		}
	}
	if _, err := r.Begin(context.Background(), Owner{}, valid); err == nil {
		t.Fatal("accepted empty owner")
	}
	result, err := r.Begin(context.Background(), owner, valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"", "!!!", "eA==\n", base64.StdEncoding.EncodeToString(make([]byte, ChunkSize+1))} {
		if _, err := r.Chunk(context.Background(), owner, result.UploadID, 0, data); err == nil {
			t.Fatal("accepted invalid or oversized chunk")
		}
	}
	if _, err := r.Complete(context.Background(), owner, result.UploadID, "incomplete"); err == nil {
		t.Fatal("completed missing file bytes")
	}
	if _, err := r.Chunk(context.Background(), owner, result.UploadID, 0, base64.StdEncoding.EncodeToString(make([]byte, ChunkSize))); err != nil {
		t.Fatal("maximum size chunk was rejected:", err)
	}
}

func TestReceiverActiveAndDiskQuotas(t *testing.T) {
	t.Run("owner", func(t *testing.T) {
		r := testReceiver(t, &fakeService{}, nil, Config{})
		for i := 0; i < MaxActiveOwner; i++ {
			beginUpload(t, r, testOwner(), fmt.Sprint(i), []byte("x"))
		}
		if _, err := r.Begin(context.Background(), testOwner(), Metadata{FileName: "x", Size: 1, IdempotencyKey: "extra"}); err == nil {
			t.Fatal("exceeded owner active count")
		}
	})
	t.Run("global", func(t *testing.T) {
		r := testReceiver(t, &fakeService{}, nil, Config{})
		for i := 0; i < MaxActiveGlobal; i++ {
			owner := testOwner()
			owner.ClientID = fmt.Sprint(i)
			beginUpload(t, r, owner, "key", []byte("x"))
		}
		if _, err := r.Begin(context.Background(), testOwner(), Metadata{FileName: "x", Size: 1, IdempotencyKey: "extra"}); err == nil {
			t.Fatal("exceeded global active count")
		}
	})
	t.Run("disk", func(t *testing.T) {
		r := testReceiver(t, &fakeService{}, nil, Config{})
		for i := 0; i < MaxDiskBytes/int(files.MaxFileBytes); i++ {
			owner := testOwner()
			owner.ClientID = fmt.Sprint(i)
			if _, err := r.Begin(context.Background(), owner, Metadata{FileName: "x", Size: files.MaxFileBytes, IdempotencyKey: "key"}); err != nil {
				t.Fatal(err)
			}
		}
		if r.reserved != MaxDiskBytes {
			t.Fatal("did not reserve declared disk usage")
		}
		if _, err := r.Begin(context.Background(), testOwner(), Metadata{FileName: "x", Size: 1, IdempotencyKey: "extra"}); err == nil {
			t.Fatal("exceeded disk reservation")
		}
	})
}

func TestReceiverBackendAndRecorderFailuresRetryWithoutDuplicateUpload(t *testing.T) {
	var attempt int
	var backendKey string
	service := &fakeService{upload: func(_ context.Context, request files.UploadRequest) (files.Asset, error) {
		attempt++
		if backendKey != "" && backendKey != request.IdempotencyKey {
			t.Error("backend retry changed idempotency key")
		}
		backendKey = request.IdempotencyKey
		if attempt == 1 {
			return files.Asset{}, errors.New("temporary backend failure")
		}
		return files.Asset{AssetID: "saved-asset"}, nil
	}}
	var recordCalls int
	var recordRequest string
	r := testReceiver(t, service, func(_ context.Context, session, request string, epoch uint64, asset files.Asset) error {
		recordCalls++
		if session != "session" || epoch != 23 || asset.AssetID != "saved-asset" {
			t.Error("wrong recorder asset or session")
		}
		if recordRequest != "" && recordRequest != request {
			t.Error("recorder retry changed request identity")
		}
		recordRequest = request
		if recordCalls == 1 {
			return errors.New("temporary recording failure")
		}
		return nil
	}, Config{})
	result, err := r.Begin(context.Background(), testOwner(), Metadata{
		FileName: "report.txt", ContentType: "text/plain", Size: 7, IdempotencyKey: "key", SessionEpoch: 23,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Chunk(context.Background(), testOwner(), result.UploadID, 0, base64.StdEncoding.EncodeToString([]byte("content"))); err != nil {
		t.Fatal(err)
	}
	if _, err := completeUpload(t, r, testOwner(), result.UploadID, "first-request"); err == nil {
		t.Fatal("backend failure was not reported")
	}
	if _, err := completeUpload(t, r, testOwner(), result.UploadID, "retry-backend"); err == nil {
		t.Fatal("recorder failure was not reported")
	}
	receipt, err := completeUpload(t, r, testOwner(), result.UploadID, "retry-recorder")
	if err != nil || receipt.Asset.AssetID != "saved-asset" || service.calls.Load() != 2 || recordCalls != 2 || recordRequest != "first-request" {
		t.Fatalf("wrong retry behavior receipt=%#v err=%v uploads=%d recordCalls=%d request=%s", receipt, err, service.calls.Load(), recordCalls, recordRequest)
	}
}

func TestReceiverHidesAssetUntilRecordingSucceeds(t *testing.T) {
	service := &fakeService{}
	started := make(chan string, 2)
	decisions := make(chan error)
	var recordCalls atomic.Int32
	r := testReceiver(t, service, func(ctx context.Context, _, request string, _ uint64, _ files.Asset) error {
		recordCalls.Add(1)
		started <- request
		select {
		case err := <-decisions:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}, Config{})
	owner := testOwner()
	content := []byte("content")
	result := beginUpload(t, r, owner, "key", content)
	metadata := Metadata{FileName: "report.txt", ContentType: "text/plain", Size: int64(len(content)), IdempotencyKey: "key"}
	assertClientReceipt := func(recorded bool) {
		t.Helper()
		status, statusErr := r.Status(context.Background(), owner, result.UploadID)
		begun, beginErr := r.Begin(context.Background(), owner, metadata)
		cancelled, cancelErr := r.Cancel(context.Background(), owner, result.UploadID)
		for _, response := range []struct {
			operation string
			result    Result
			err       error
		}{
			{"status", status, statusErr},
			{"begin", begun, beginErr},
			{"cancel", cancelled, cancelErr},
		} {
			if response.err != nil || response.result.UploadID != result.UploadID || response.result.Offset != int64(len(content)) {
				t.Fatalf("%s lost upload state: %#v %v", response.operation, response.result, response.err)
			}
			if (response.result.Asset != nil) != recorded {
				t.Fatalf("%s exposed incorrect recording state recorded=%v: %#v", response.operation, recorded, response.result)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	waitForRecorder := func() {
		t.Helper()
		select {
		case request := <-started:
			if request != "first-complete" {
				t.Fatalf("recorder retry changed request identity: %s", request)
			}
		case <-ctx.Done():
			t.Fatal("recorder did not start")
		}
	}
	completion, err := r.Complete(context.Background(), owner, result.UploadID, "first-complete")
	if err != nil {
		t.Fatal(err)
	}
	waitForRecorder()
	assertClientReceipt(false)
	decisions <- errors.New("recording failed")
	failed, err := completion.Wait(ctx)
	if err == nil || failed.Asset != nil {
		t.Fatalf("failed recording exposed a successful receipt: %#v %v", failed, err)
	}
	assertClientReceipt(false)
	retry, err := r.Complete(context.Background(), owner, result.UploadID, "retry-complete")
	if err != nil {
		t.Fatal(err)
	}
	waitForRecorder()
	assertClientReceipt(false)
	decisions <- nil
	receipt, err := retry.Wait(ctx)
	if err != nil || receipt.Asset == nil || receipt.Asset.AssetID != "asset-1" {
		t.Fatalf("successful recording did not publish the receipt: %#v %v", receipt, err)
	}
	assertClientReceipt(true)
	if service.calls.Load() != 1 || recordCalls.Load() != 2 {
		t.Fatalf("recording retry duplicated backend upload: uploads=%d records=%d", service.calls.Load(), recordCalls.Load())
	}
}

func TestReceiverCompleteCoalescesAndCancelJoinsWorker(t *testing.T) {
	started, exited := make(chan struct{}), make(chan struct{})
	service := &fakeService{upload: func(ctx context.Context, _ files.UploadRequest) (files.Asset, error) {
		close(started)
		<-ctx.Done()
		close(exited)
		return files.Asset{}, ctx.Err()
	}}
	r := testReceiver(t, service, nil, Config{})
	result := beginUpload(t, r, testOwner(), "key", []byte("content"))
	completion, err := r.Complete(context.Background(), testOwner(), result.UploadID, "first")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	retry, err := r.Complete(context.Background(), testOwner(), result.UploadID, "retry")
	if err != nil || retry != completion {
		t.Fatal("concurrent completion did not share its worker")
	}
	if _, err := r.Cancel(context.Background(), testOwner(), result.UploadID); err != nil {
		t.Fatal(err)
	}
	if _, err := completion.Wait(context.Background()); err == nil {
		t.Fatal("cancelled backend completion succeeded")
	}
	select {
	case <-exited:
	default:
		t.Fatal("backend worker did not exit")
	}
	if service.calls.Load() != 1 {
		t.Fatal("concurrent completion repeated backend upload")
	}
	if _, err := r.Cancel(context.Background(), testOwner(), result.UploadID); err != nil {
		t.Fatal("cancel retry was not idempotent:", err)
	}
	if _, err := r.Complete(context.Background(), testOwner(), result.UploadID, "cancelled-retry"); err == nil {
		t.Fatal("cancelled upload restarted")
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancel retained temp file: %v %v", entries, err)
	}
}

func TestReceiverCancellationScopesBackendAndJoinsUpload(t *testing.T) {
	started := make(chan string, 1)
	exited := make(chan struct{})
	var calls atomic.Int32
	service := &cancellingFakeService{
		fakeService: &fakeService{upload: func(ctx context.Context, request files.UploadRequest) (files.Asset, error) {
			started <- request.IdempotencyKey
			<-ctx.Done()
			close(exited)
			return files.Asset{}, ctx.Err()
		}},
	}
	service.cancel = func(_ context.Context, _ string) (files.Asset, error) {
		select {
		case <-exited:
		default:
			t.Error("backend cancel ran before upload worker exited")
		}
		if calls.Add(1) == 1 {
			return files.Asset{}, errors.New("cleanup pending")
		}
		return files.Asset{}, nil
	}
	r := testReceiver(t, service, nil, Config{ReceiptTTL: time.Nanosecond})
	owner := testOwner()
	begun := beginUpload(t, r, owner, "client-key", []byte("content"))
	if _, err := r.Complete(context.Background(), owner, begun.UploadID, "first"); err != nil {
		t.Fatal(err)
	}
	backendKey := <-started
	cancelBackend := service.cancel
	service.cancel = func(ctx context.Context, key string) (files.Asset, error) {
		if key != backendKey || key == "client-key" {
			t.Error("cancel did not use the scoped upload key")
		}
		return cancelBackend(ctx, key)
	}
	other := owner
	other.ClientID = "different-client"
	if _, err := r.Cancel(context.Background(), other, begun.UploadID); err == nil || calls.Load() != 0 {
		t.Fatal("unrelated owner reached backend cancellation")
	}
	if _, err := r.Cancel(context.Background(), owner, begun.UploadID); err == nil {
		t.Fatal("cleanup failure was acknowledged")
	}
	if _, err := r.Complete(context.Background(), owner, begun.UploadID, "late-complete"); err == nil {
		t.Fatal("completion restarted during pending cleanup")
	}
	if _, err := r.Cancel(context.Background(), owner, begun.UploadID); err != nil || calls.Load() != 2 {
		t.Fatalf("cleanup could not be retried: %v", err)
	}
}

func TestReceiverCancellationRecoversCommittedAssetAndRecords(t *testing.T) {
	service := &cancellingFakeService{
		fakeService: &fakeService{upload: func(context.Context, files.UploadRequest) (files.Asset, error) {
			return files.Asset{}, errors.New("completion response lost")
		}},
		cancel: func(context.Context, string) (files.Asset, error) {
			return files.Asset{AssetID: "committed"}, nil
		},
	}
	var recorded atomic.Int32
	r := testReceiver(t, service, func(_ context.Context, session, request string, _ uint64, asset files.Asset) error {
		if session != "session" || request != "original" || asset.AssetID != "committed" {
			t.Error("recording lost original upload identity")
		}
		recorded.Add(1)
		return nil
	}, Config{})
	owner := testOwner()
	begun := beginUpload(t, r, owner, "key", []byte("content"))
	if _, err := completeUpload(t, r, owner, begun.UploadID, "original"); err == nil {
		t.Fatal("lost completion was acknowledged")
	}
	result, err := r.Cancel(context.Background(), owner, begun.UploadID)
	if err != nil || result.Asset == nil || result.Asset.AssetID != "committed" || recorded.Load() != 1 {
		t.Fatalf("committed asset was not recorded: %+v %v", result, err)
	}
	if _, err := completeUpload(t, r, owner, begun.UploadID, "retry"); err != nil || service.calls.Load() != 1 || recorded.Load() != 1 {
		t.Fatalf("completion did not reuse recorded asset: %v", err)
	}
}

func TestReceiverDisconnectAndCloseCleanTemporaryFiles(t *testing.T) {
	started, exited := make(chan struct{}), make(chan struct{})
	service := &fakeService{upload: func(ctx context.Context, _ files.UploadRequest) (files.Asset, error) {
		close(started)
		<-ctx.Done()
		close(exited)
		return files.Asset{}, ctx.Err()
	}}
	r := testReceiver(t, service, nil, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	result, err := r.Begin(ctx, testOwner(), Metadata{FileName: "x", Size: 1, IdempotencyKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.active == 0
	})
	if _, err := r.Status(context.Background(), testOwner(), result.UploadID); err == nil {
		t.Fatal("disconnect left staging upload usable")
	}
	result = beginUpload(t, r, testOwner(), "worker", []byte("content"))
	completion, err := r.Complete(context.Background(), testOwner(), result.UploadID, "complete")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	dir := r.dir
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("close did not join backend worker")
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("close left temporary directory: %v", err)
	}
	if _, err := completion.Wait(context.Background()); err == nil {
		t.Fatal("close did not cancel completion")
	}
}

func TestReceiverTTLAndBoundedReceipts(t *testing.T) {
	r := testReceiver(t, &fakeService{}, nil, Config{UploadTTL: time.Hour, ReceiptTTL: time.Hour})
	result := beginUpload(t, r, testOwner(), "expired", []byte("x"))
	r.mu.Lock()
	r.pruneLocked(time.Now().Add(2 * time.Hour))
	if r.active != 0 || r.reserved != 0 {
		t.Error("expired upload retained reservation")
	}
	// The just-cancelled entry is a bounded receipt, then expires separately.
	r.pruneLocked(time.Now().Add(4 * time.Hour))
	_, exists := r.uploads[result.UploadID]
	r.mu.Unlock()
	if exists {
		t.Fatal("expired cancellation receipt was retained")
	}
	for i := 0; i < maxReceipts+8; i++ {
		result = beginUpload(t, r, testOwner(), fmt.Sprint(i), []byte("x"))
		if _, err := r.Cancel(context.Background(), testOwner(), result.UploadID); err != nil {
			t.Fatal(err)
		}
	}
	r.mu.Lock()
	r.pruneLocked(time.Now())
	count := len(r.uploads)
	r.mu.Unlock()
	if count > maxReceipts {
		t.Fatalf("unbounded receipts: %d", count)
	}
}

func TestReceiverSuccessfulCommitSurvivesDisconnectRace(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	service := &fakeService{upload: func(_ context.Context, _ files.UploadRequest) (files.Asset, error) {
		close(started)
		<-release
		return files.Asset{AssetID: "committed"}, nil
	}}
	r := testReceiver(t, service, nil, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	result, err := r.Begin(ctx, testOwner(), Metadata{FileName: "x", Size: 1, IdempotencyKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Chunk(ctx, testOwner(), result.UploadID, 0, "eA=="); err != nil {
		t.Fatal(err)
	}
	completion, err := r.Complete(ctx, testOwner(), result.UploadID, "first")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	close(release)
	if _, err := completion.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	retry, err := completeUpload(t, r, testOwner(), result.UploadID, "reconnected")
	if err != nil || retry.Asset.AssetID != "committed" || service.calls.Load() != 1 {
		t.Fatalf("disconnect lost committed receipt: %#v %v", retry, err)
	}
}

func TestReceiverConcurrentStatusAndCompletion(t *testing.T) {
	r := testReceiver(t, &fakeService{}, nil, Config{})
	result := beginUpload(t, r, testOwner(), "key", []byte("content"))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_, _ = r.Status(context.Background(), testOwner(), result.UploadID)
			}
		}()
	}
	if _, err := completeUpload(t, r, testOwner(), result.UploadID, "complete"); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !condition() {
		select {
		case <-deadline.C:
			t.Fatal("condition was not reached")
		case <-ticker.C:
		}
	}
}
