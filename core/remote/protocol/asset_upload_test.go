package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAssetUploadContract(t *testing.T) {
	requests := []struct {
		kind    MessageType
		payload any
	}{
		{TypeAssetUploadBegin, AssetUploadBeginPayload{FileName: "report.txt", ContentType: "text/plain", Size: 3, IdempotencyKey: "key"}},
		{TypeAssetUploadChunk, AssetUploadChunkPayload{UploadID: "upload", Offset: 0, Data: "YWJj"}},
		{TypeAssetUploadStatus, AssetUploadStatusPayload{UploadID: "upload"}},
		{TypeAssetUploadComplete, AssetUploadCompletePayload{UploadID: "upload"}},
		{TypeAssetUploadCancel, AssetUploadCancelPayload{UploadID: "upload"}},
	}
	for _, request := range requests {
		message, err := NewMessage(request.kind, "request", "user", "device", "session", request.payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateAssetUploadMessage(message); err != nil {
			t.Fatalf("%s rejected valid contract: %v", request.kind, err)
		}
	}
	data, err := json.Marshal(AssetUploadResultPayload{
		Operation: "complete", UploadID: "upload", Offset: 3,
		ChunkSize: AssetUploadChunkSize, MaxFileBytes: AssetUploadMaxFileBytes,
		Asset: &UploadedAsset{AssetID: "asset", FileName: "report.txt", Size: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{`"operation":"complete"`, `"chunk_size":262144`, `"max_file_bytes":52428800`, `"asset_id":"asset"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing contract field %s in %s", expected, text)
		}
	}
	if strings.Contains(text, "short_url") || strings.Contains(text, "code") {
		t.Fatal("upload-only receipt emitted public share metadata")
	}
}

func TestAssetUploadRejectsOversizedAndChangedPayloads(t *testing.T) {
	for _, payload := range []string{
		`{"upload_id":"upload","size":3}`,
		`{"upload_id":"upload","file_name":"changed"}`,
		`null`,
		`{"upload_id":"upload"} {}`,
		`{"upload_id":"` + strings.Repeat("x", AssetUploadMaxJSONBytes) + `"}`,
	} {
		message := Message{Type: TypeAssetUploadComplete, RequestID: "request", SessionID: "session", Payload: json.RawMessage(payload)}
		if ValidateAssetUploadMessage(message) == nil {
			t.Fatalf("accepted malformed, changed, or oversized payload of length %d", len(payload))
		}
	}
	if ValidateAssetUploadMessage(Message{Type: TypeAssetUploadBegin, Payload: json.RawMessage(`{}`)}) == nil {
		t.Fatal("accepted upload without request or session identity")
	}
}

func TestKnowledgeIngestContractOnlyUsesAssetID(t *testing.T) {
	payload := KnowledgeDocumentIngestPayload{KnowledgeBaseID: "kb", AssetID: "asset"}
	data, err := json.Marshal(payload)
	if err != nil || string(data) != `{"knowledge_base_id":"kb","asset_id":"asset"}` {
		t.Fatalf("unexpected ingest contract: %s %v", data, err)
	}
}
