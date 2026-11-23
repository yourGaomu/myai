package shortlinks

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"myai/core/mcp"
	filesport "myai/core/port/files"
)

const maxPartBytes int64 = 32 << 20

type uploadSession struct {
	UploadSessionID string    `json:"uploadSessionId"`
	IdempotencyKey  string    `json:"idempotencyKey"`
	Status          string    `json:"status"`
	FileName        string    `json:"fileName"`
	BusinessID      string    `json:"businessId"`
	Path            string    `json:"path"`
	SizeBytes       *int64    `json:"sizeBytes"`
	PartSizeBytes   int64     `json:"partSizeBytes"`
	PartCount       int       `json:"partCount"`
	ReceivedParts   []int     `json:"receivedParts"`
	ReceivedBytes   *int64    `json:"receivedBytes"`
	ExpiresAt       time.Time `json:"expiresAt"`
	Asset           *assetDTO `json:"asset"`
}

func (u uploadSession) received(number int) bool {
	for _, part := range u.ReceivedParts {
		if number == part {
			return true
		}
	}
	return false
}

func (u uploadSession) validate(request filesport.UploadRequest, previous *uploadSession) error {
	if !validID(u.UploadSessionID) || !validID(u.BusinessID) || u.FileName != request.FileName ||
		u.SizeBytes == nil || *u.SizeBytes != request.Size || u.PartSizeBytes < 1 || u.PartSizeBytes > maxPartBytes ||
		u.PartCount < 1 || u.PartCount > 10000 || int64(u.PartCount) != (request.Size-1)/u.PartSizeBytes+1 ||
		u.ReceivedBytes == nil || u.ReceivedParts == nil || u.ExpiresAt.IsZero() ||
		u.IdempotencyKey != "" && u.IdempotencyKey != request.IdempotencyKey {
		return errors.New("invalid short_links upload session")
	}
	switch u.Status {
	case "initializing", "uploading", "completing", "completed", "cancelled", "cleanup_pending", "failed", "expired":
	default:
		return errors.New("invalid short_links upload state")
	}
	seen := make(map[int]bool, len(u.ReceivedParts))
	var received int64
	for _, part := range u.ReceivedParts {
		if part < 1 || part > u.PartCount || seen[part] {
			return errors.New("invalid short_links upload part inventory")
		}
		seen[part] = true
		size := u.PartSizeBytes
		if part == u.PartCount {
			size = request.Size - int64(u.PartCount-1)*u.PartSizeBytes
		}
		received += size
	}
	if *u.ReceivedBytes != received || received > request.Size {
		return errors.New("invalid short_links upload received size")
	}
	if previous != nil {
		if previous.UploadSessionID != u.UploadSessionID || previous.BusinessID != u.BusinessID ||
			previous.Path != u.Path || previous.PartSizeBytes != u.PartSizeBytes || previous.PartCount != u.PartCount {
			return errors.New("short_links upload session identity changed")
		}
		for _, part := range previous.ReceivedParts {
			if !seen[part] && !u.terminal() && u.Status != "cleanup_pending" {
				return errors.New("short_links upload progress regressed")
			}
		}
	}
	if u.Status == "completed" {
		if u.Asset == nil || received != request.Size {
			return errors.New("incomplete short_links upload result")
		}
		asset, err := u.Asset.asset("")
		if err != nil || asset.BusinessID != u.BusinessID || asset.FileName != request.FileName || asset.Size != request.Size {
			return errors.New("invalid completed short_links asset")
		}
	} else if u.Asset != nil {
		return errors.New("short_links asset returned before completion")
	}
	return nil
}

var uploadTools = []string{"begin_asset_upload", "upload_asset_part", "get_asset_upload_status", "complete_asset_upload"}

var _ filesport.UploadPreflighter = (*Service)(nil)

// PreflightUpload checks the current managed catalog without calling a tool or
// retaining a lease. A later transfer still acquires and validates its own lease.
func (s *Service) PreflightUpload(ctx context.Context) error {
	lease, err := s.acquire(ctx, uploadTools...)
	if err != nil {
		return err
	}
	return lease.Close()
}

func (s *Service) Upload(ctx context.Context, request filesport.UploadRequest) (filesport.Asset, error) {
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	releaseSlot, err := s.uploadSlot(ctx)
	if err != nil {
		return filesport.Asset{}, err
	}
	defer releaseSlot()
	data, request, err := readUpload(ctx, request)
	if err != nil {
		return filesport.Asset{}, err
	}
	asset, _, _, done, err := s.transfer(ctx, request, data, uploadTools)
	defer done()
	return asset, err
}

func (u uploadSession) terminal() bool {
	return u.Status == "cancelled" || u.Status == "failed" || u.Status == "expired"
}

func uploadBegin(ctx context.Context, lease *mcp.ServerLease, request filesport.UploadRequest, receipt *uploadReceipt) (uploadSession, error) {
	args := map[string]any{"fileName": request.FileName, "sizeBytes": request.Size,
		"contentType": request.ContentType, "idempotencyKey": request.IdempotencyKey}
	var session uploadSession
	alreadyAttempted := receipt != nil && receipt.started
	if err := ctx.Err(); err != nil {
		return session, err
	}
	if receipt != nil {
		receipt.started = true
	}
	err := call(ctx, lease, "begin_asset_upload", args, &session)
	if uncertainCall(err) && ctx.Err() == nil {
		// No parts or completion have been sent. Recover once using the same
		// key and original lease. A rejection cannot resolve the first call.
		alreadyAttempted = true
		session = uploadSession{}
		err = call(ctx, lease, "begin_asset_upload", args, &session)
	}
	if err != nil {
		var failure *callFailure
		if receipt != nil && !alreadyAttempted && errors.As(err, &failure) && failure.notCreated {
			receipt.started = false
		}
		return uploadSession{}, safeError("begin file upload", err)
	}
	if err := session.validate(request, nil); err != nil {
		return uploadSession{}, err
	}
	if receipt != nil {
		receipt.remember(session)
	}
	return session, nil
}

func upload(ctx context.Context, lease *mcp.ServerLease, request filesport.UploadRequest, data []byte, receipt *uploadReceipt) (filesport.Asset, error) {
	if request.IdempotencyKey == "" {
		var key [16]byte
		if _, err := rand.Read(key[:]); err != nil {
			return filesport.Asset{}, errors.New("cannot create file upload idempotency key")
		}
		request.IdempotencyKey = "myai-" + hex.EncodeToString(key[:])
	}
	var session uploadSession
	var err error
	if receipt != nil && receipt.session != nil {
		// Never begin a known session again, including a completion whose
		// response and first status query were lost.
		session, err = uploadStatus(ctx, lease, request, *receipt.session)
	} else {
		session, err = uploadBegin(ctx, lease, request, receipt)
	}
	if err != nil {
		return filesport.Asset{}, err
	}
	if receipt != nil {
		receipt.remember(session)
		defer func() { receipt.remember(session) }()
	}
	if session.Status == "initializing" || session.Status == "completing" {
		next, statusErr := uploadStatus(ctx, lease, request, session)
		if statusErr != nil {
			return filesport.Asset{}, statusErr
		}
		session = next
	}
	if session.terminal() {
		return filesport.Asset{}, errors.New("short_links upload session has ended")
	}
	if !session.ExpiresAt.After(time.Now()) && session.Status != "completed" {
		return filesport.Asset{}, errors.New("short_links upload session deadline passed; query or cancel the existing session")
	}
	if session.Status != "uploading" && session.Status != "completed" && session.Status != "completing" {
		return filesport.Asset{}, errors.New("short_links upload is not ready; resume its existing session later")
	}
	if session.Status == "completing" && *session.ReceivedBytes != request.Size {
		return filesport.Asset{}, errors.New("short_links completion is missing confirmed parts")
	}

	for number := 1; number <= session.PartCount && session.Status == "uploading"; number++ {
		if session.received(number) {
			continue
		}
		start := int64(number-1) * session.PartSizeBytes
		end := min(start+session.PartSizeBytes, request.Size)
		args := map[string]any{"uploadSessionId": session.UploadSessionID, "partNumber": number,
			"contentBase64": base64.StdEncoding.EncodeToString(data[start:end])}
		var next uploadSession
		err = call(ctx, lease, "upload_asset_part", args, &next)
		if uncertainCall(err) && ctx.Err() == nil {
			// A timed-out PUT may have succeeded. Query this same server/session;
			// only a confirmed missing part may be sent once more.
			next, err = uploadStatus(ctx, lease, request, session)
			if err == nil && next.Status == "uploading" && !next.received(number) {
				session = next
				next = uploadSession{}
				err = call(ctx, lease, "upload_asset_part", args, &next)
			}
		}
		if err != nil {
			return filesport.Asset{}, safeError("upload file part; existing session may be resumed", err)
		}
		if err := next.validate(request, &session); err != nil {
			return filesport.Asset{}, err
		}
		session = next
		if !next.received(number) || next.Status != "uploading" && next.Status != "completed" {
			return filesport.Asset{}, errors.New("short_links did not confirm the uploaded part")
		}
		if session.Status != "completed" && !session.ExpiresAt.After(time.Now()) {
			return filesport.Asset{}, errors.New("short_links upload session deadline passed")
		}
	}
	if session.Status != "completed" {
		if *session.ReceivedBytes != request.Size {
			return filesport.Asset{}, errors.New("short_links completion is missing confirmed parts")
		}
		var next uploadSession
		// On an explicit caller retry, status has already confirmed this same
		// session and all parts. Completing it again is idempotent under the
		// backend session lock; begin/publication are never repeated here.
		err = call(ctx, lease, "complete_asset_upload", map[string]any{"uploadSessionId": session.UploadSessionID}, &next)
		if uncertainCall(err) && ctx.Err() == nil {
			next, err = uploadStatus(ctx, lease, request, session)
		}
		if err != nil {
			return filesport.Asset{}, safeError("complete file upload; existing session may be resumed", err)
		}
		if err := next.validate(request, &session); err != nil {
			return filesport.Asset{}, err
		}
		session = next
	}
	if session.Status != "completed" || session.Asset == nil {
		return filesport.Asset{}, errors.New("file completion is pending; resume the existing upload session")
	}
	return session.checkedAsset(sha256.Sum256(data))
}

func (u uploadSession) checkedAsset(checksum [32]byte) (filesport.Asset, error) {
	asset, err := u.Asset.asset("")
	if err != nil {
		return filesport.Asset{}, err
	}
	if asset.Checksum != "" && asset.Checksum != hex.EncodeToString(checksum[:]) {
		return filesport.Asset{}, errors.New("uploaded asset checksum does not match the file")
	}
	return asset, nil
}

func uploadStatus(ctx context.Context, lease *mcp.ServerLease, request filesport.UploadRequest, previous uploadSession) (uploadSession, error) {
	var result uploadSession
	if err := call(ctx, lease, "get_asset_upload_status", map[string]any{"uploadSessionId": previous.UploadSessionID}, &result); err != nil {
		return result, safeError("get existing file upload status", err)
	}
	if err := result.validate(request, &previous); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) Share(ctx context.Context, request filesport.ShareRequest) (filesport.Asset, error) {
	if request.TTLSeconds < 0 || request.TTLSeconds > int64((100*365*24*time.Hour)/time.Second) || request.MaxVisits < -1 {
		return filesport.Asset{}, errors.New("invalid file share expiration or visit limit")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	releaseSlot, err := s.uploadSlot(ctx)
	if err != nil {
		return filesport.Asset{}, err
	}
	defer releaseSlot()
	data, uploadRequest, err := readUpload(ctx, request.UploadRequest)
	if err != nil {
		return filesport.Asset{}, err
	}
	tools := append(append([]string(nil), uploadTools...), "create_asset_shortlink")
	asset, lease, receipt, done, err := s.transfer(ctx, uploadRequest, data, tools)
	defer done()
	if err != nil {
		return asset, err
	}
	shareKey := fmt.Sprintf("%s\x00%d\x00%d", request.Title, request.TTLSeconds, request.MaxVisits)
	if receipt != nil {
		if receipt.shareTried {
			if receipt.shareKey == shareKey && receipt.shared.ShortURL != "" {
				return receipt.shared, nil
			}
			return asset, fmt.Errorf("asset %s uploaded; previous publication outcome requires explicit follow-up", asset.AssetID)
		}
		receipt.shareTried, receipt.shareKey = true, shareKey
	}
	args := map[string]any{"assetId": asset.AssetID, "title": request.Title, "maxVisits": request.MaxVisits}
	if request.TTLSeconds > 0 {
		args["expiresAt"] = time.Now().UTC().Add(time.Duration(request.TTLSeconds) * time.Second).Format(time.RFC3339)
	}
	var result struct {
		Asset     assetDTO   `json:"asset"`
		ShortURL  string     `json:"shortUrl"`
		Code      string     `json:"code"`
		ExpiresAt *time.Time `json:"expiresAt"`
		MaxVisits *int64     `json:"maxVisits"`
	}
	// Publication is never retried; a lost response could already have created
	// a link. The caller retains the uploaded asset ID for an explicit follow-up.
	if err := call(ctx, lease, "create_asset_shortlink", args, &result); err != nil {
		return asset, fmt.Errorf("asset %s uploaded; %w", asset.AssetID, safeError("create file share", err))
	}
	linked, err := result.Asset.asset(asset.AssetID)
	if err != nil || linked.BusinessID != asset.BusinessID || linked.Size != asset.Size || linked.FileName != asset.FileName ||
		!validShortURL(result.ShortURL) || !validID(result.Code) || result.MaxVisits == nil ||
		*result.MaxVisits < -1 || result.ExpiresAt != nil && !result.ExpiresAt.After(time.Now()) {
		return asset, fmt.Errorf("asset %s uploaded; invalid short_links share result", asset.AssetID)
	}
	asset.ShortURL, asset.Code, asset.ExpiresAt, asset.MaxVisits = result.ShortURL, result.Code, result.ExpiresAt, *result.MaxVisits
	if receipt != nil {
		receipt.shared = asset
	}
	return asset, nil
}
