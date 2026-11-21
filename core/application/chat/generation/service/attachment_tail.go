package service

import (
	"regexp"
	"strconv"
	"strings"

	"myai/core/imageasset"
	"myai/core/port/files"
)

const (
	attachmentTailMarker = "\n\nAttached files:\n"
	maxAttachmentRefs    = 64
)

var (
	uploadedFileHeader  = regexp.MustCompile(`^<uploaded_file asset_id="([A-Za-z0-9_-]{1,128})" name="([^"<>\r\n]*)" content_type="([^"<>\r\n]*)" size="([0-9]+)">$`)
	workspaceFileHeader = regexp.MustCompile(`^<file path="[^"<>\r\n]+" language="[^"<>\r\n]*" size="([0-9]+)">$`)
	attachmentUnescape  = strings.NewReplacer("&quot;", `"`, "&lt;", "<", "&gt;", ">", "&amp;", "&")
)

type attachedAssetRef struct {
	id        string
	imageHint bool
}

// parseAttachmentTail accepts the entire canonical mobile suffix or none of it.
// Start at the first header so a header quoted inside a workspace file cannot
// turn its content into independently supplied attachments.
func parseAttachmentTail(text string) ([]attachedAssetRef, bool) {
	prompt, tail, found := strings.Cut(text, attachmentTailMarker)
	if !found || strings.TrimSpace(prompt) == "" {
		return nil, false
	}
	var refs []attachedAssetRef
	limited := false
	for {
		header, content, ok := strings.Cut(tail, "\n")
		if !ok {
			return nil, false
		}
		switch {
		case strings.HasPrefix(header, "<uploaded_file "):
			match := uploadedFileHeader.FindStringSubmatch(header)
			if match == nil || !files.IsSessionAssetID(match[1]) {
				return nil, false
			}
			if _, err := strconv.ParseUint(match[4], 10, 53); err != nil {
				return nil, false
			}
			tail, ok = strings.CutPrefix(content, "[User attachment]\n</uploaded_file>")
			if !ok {
				return nil, false
			}
			hint := imageasset.LooksLikeImage(attachmentUnescape.Replace(match[2]), attachmentUnescape.Replace(match[3]))
			if len(refs) < maxAttachmentRefs {
				refs = append(refs, attachedAssetRef{id: match[1], imageHint: hint})
			} else {
				limited = true
			}
		case strings.HasPrefix(header, "<file "):
			match := workspaceFileHeader.FindStringSubmatch(header)
			if match == nil {
				return nil, false
			}
			size, err := strconv.ParseUint(match[1], 10, 53)
			var body string
			body, tail, ok = strings.Cut(content, "\n</file>")
			if err != nil || !ok || !isWorkspaceAttachmentBody(body, size) {
				return nil, false
			}
		default:
			return nil, false
		}
		if tail == "" {
			return refs, limited
		}
		tail, ok = strings.CutPrefix(tail, "\n\n")
		if !ok {
			return nil, false
		}
	}
}

func isWorkspaceAttachmentBody(body string, byteSize uint64) bool {
	if strings.HasSuffix(body, "\n\n[content truncated]") {
		return true
	}
	text, ok := strings.CutSuffix(body, "\n")
	// Binary workspace previews have no text. A full text preview retains its
	// byte length, so a quoted closing tag inside it does not complete the block.
	return ok && (text == "" || uint64(len(text)) == byteSize)
}
