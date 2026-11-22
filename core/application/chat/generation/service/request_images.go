package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/imageasset"
	"myai/core/port/files"
	modelport "myai/core/port/model"
)

const (
	maxVisionReferences = 32
	// These are local context estimates. Actual vision token costs depend on the provider/model.
	imageBaseTokens = 85
	imageTileTokens = 170
	imageTileSide   = 512
)

// turnImages holds bounded, lazy state for one Run only. It is never stored in
// a session, snapshot, or execution context (which may reach hooks/subagents).
type turnImages struct {
	reads        map[string]map[string]string // assigned result message ID -> trusted call ID -> asset ID
	projected    []domainmessage.ImageReceipt
	replayReads  bool
	assets       map[string]*resolvedImage
	downloaded   int
	downloadSize int
	// A run started by new synthetic input can reuse completed history, but
	// must not resurrect an unfinished image turn in front of that input.
	resumingSyntheticInput bool
}

type resolvedImage struct {
	asset   files.Asset
	image   modelport.InputImage
	tokens  int
	sha256  string
	problem string
}

type visionCandidate struct {
	attachedAssetRef
	messageIndex int
	sourceID     string
	anchorID     string
	callID       string
	sha256       string
	mimeType     string
	toolResult   bool
}

func (turn *turnImages) acceptReads(reads map[string]string, messages []domainmessage.Message) {
	for _, message := range messages {
		if message.Role != domainmessage.RoleTool || message.ID == "" {
			continue
		}
		for _, part := range message.Parts {
			result := part.ToolResult
			if result == nil || result.Name != "read_asset" || result.Status != domaintool.ResultStatusSuccess || result.Truncated {
				continue
			}
			if id := reads[result.ToolCallID]; files.IsSessionAssetID(id) {
				if turn.reads == nil {
					turn.reads = make(map[string]map[string]string)
				}
				if turn.reads[message.ID] == nil {
					turn.reads[message.ID] = make(map[string]string)
				}
				turn.reads[message.ID][result.ToolCallID] = id
			}
		}
	}
}

func readAssetCallID(call domainmessage.ToolCall) string {
	if call.Name != "read_asset" || call.ID == "" || len(call.Arguments) > 4096 {
		return ""
	}
	var args map[string]any
	if json.Unmarshal([]byte(call.Arguments), &args) != nil {
		return ""
	}
	raw, _ := args["asset_id"].(string)
	id := strings.TrimSpace(raw)
	if !files.IsSessionAssetID(id) {
		return ""
	}
	return id
}

func (turn *turnImages) selectedToolImages(messages []domainmessage.Message) []visionCandidate {
	if len(turn.reads) == 0 && !turn.replayReads {
		return nil
	}
	var selected []visionCandidate
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		if message.Role != domainmessage.RoleAssistant || !message.HasToolCall() {
			continue
		}
		// The snapshot already verifies complete pairs. Attach after the last
		// result of the entire batch, not in the middle of parallel results.
		end := index
		sources := make(map[string]string)
		for end+1 < len(messages) && messages[end+1].Role == domainmessage.RoleTool {
			end++
			if result, ok := messages[end].FirstToolResult(); ok && result.Status == domaintool.ResultStatusSuccess && !result.Truncated && result.Name == "read_asset" {
				sources[result.ToolCallID] = messages[end].ID
			}
		}
		for _, part := range message.Parts {
			if part.ToolCall == nil {
				continue
			}
			call := *part.ToolCall
			id, source := readAssetCallID(call), sources[call.ID]
			if id == "" || source == "" || messages[end].ID == "" || !turn.replayReads && turn.reads[source][call.ID] != id {
				continue
			}
			selected = append(selected, visionCandidate{attachedAssetRef: attachedAssetRef{id: id, imageHint: true}, messageIndex: end, sourceID: source, anchorID: messages[end].ID, callID: call.ID, toolResult: true})
		}
		index = end
	}
	return selected
}

const imageReceiptVersion = 1

type imageOccurrence struct{ asset, source, anchor, call string }

func (candidate visionCandidate) occurrence() imageOccurrence {
	return imageOccurrence{candidate.id, candidate.sourceID, candidate.anchorID, candidate.callID}
}

func (turn *turnImages) imageCandidates(messages, transcript []domainmessage.Message) ([]visionCandidate, bool) {
	recent, older, limited := selectedUserAttachments(messages, transcript, turn.resumingSyntheticInput)
	tools := turn.selectedToolImages(messages)
	possible := make(map[imageOccurrence]visionCandidate)
	for _, group := range [][]visionCandidate{recent, older, tools} {
		for _, candidate := range group {
			if candidate.sourceID != "" {
				possible[candidate.occurrence()] = candidate
			}
		}
	}
	// Receipts are only on assistant outputs of successful samples. Scan in
	// transcript order so a later mention cannot move an already sent image.
	var candidates []visionCandidate
	pinned := make(map[string]bool)
	for index, message := range messages {
		if message.Role != domainmessage.RoleAssistant || len(pinned) >= maxVisionReferences {
			continue
		}
		for _, receipt := range message.ImageReceipts {
			if len(pinned) >= maxVisionReferences {
				limited = true
				break
			}
			key := imageOccurrence{receipt.AssetID, receipt.SourceMessageID, receipt.AnchorMessageID, receipt.ToolCallID}
			candidate, ok := possible[key]
			if !ok || candidate.messageIndex >= index || pinned[candidate.id] || receipt.Version != imageReceiptVersion ||
				len(receipt.SHA256) != sha256.Size*2 || (receipt.MIMEType != "image/jpeg" && receipt.MIMEType != "image/png") {
				continue
			}
			candidate.sha256, candidate.mimeType = receipt.SHA256, receipt.MIMEType
			candidate.imageHint = true
			candidates = append(candidates, candidate)
			pinned[candidate.id] = true
		}
	}
	// Keep explicit attachment placement for assets never presented before.
	// A historical tool exchange without a receipt cannot create a candidate.
	readIDs := make(map[string]bool)
	for _, candidate := range tools {
		if turn.reads[candidate.sourceID][candidate.callID] == candidate.id {
			readIDs[candidate.id] = true
		}
	}
	explicit := make(map[string]bool)
	for _, group := range [][]visionCandidate{recent, older} {
		for index := range group {
			candidate := &group[index]
			candidate.imageHint = candidate.imageHint || readIDs[candidate.id]
			explicit[candidate.id] = true
		}
	}
	candidates = append(candidates, recent...)
	for _, candidate := range tools {
		if turn.reads[candidate.sourceID][candidate.callID] == candidate.id && !explicit[candidate.id] {
			candidates = append(candidates, candidate)
		}
	}
	return append(candidates, older...), limited
}

func selectedUserAttachments(messages, transcript []domainmessage.Message, resumingSyntheticInput bool) (recent, older []visionCandidate, limited bool) {
	// Most requests have no suffix. Resolve original roles lazily because
	// context projection can also map historical system text to the user role.
	var originalUsers map[string]bool
	var legacyUsers map[string]bool
	latest, answered := !resumingSyntheticInput, false
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.IsSynthetic() {
			continue
		}
		if message.Role == domainmessage.RoleAssistant && !message.HasToolCall() {
			answered = true
			continue
		}
		if message.Role != domainmessage.RoleUser {
			continue
		}
		isLatest, hasAnswer := latest, answered
		latest, answered = false, false
		if !isLatest && !hasAnswer {
			// A rejected request leaves its user text in history. Its images
			// should not trigger the same provider rejection on every later turn.
			continue
		}
		text := message.Text()
		if !strings.Contains(text, attachmentTailMarker) {
			continue
		}
		if originalUsers == nil {
			originalUsers, legacyUsers = make(map[string]bool), make(map[string]bool)
			for _, original := range transcript {
				if original.Role != domainmessage.RoleUser || original.IsSynthetic() {
					continue
				}
				if original.ID != "" {
					originalUsers[original.ID] = true
				} else {
					legacyUsers[original.Text()] = true
				}
			}
		}
		if message.ID != "" && !originalUsers[message.ID] || message.ID == "" && !legacyUsers[text] {
			continue
		}
		refs, capped := parseAttachmentTail(text)
		limited = limited || capped
		for _, ref := range refs {
			if len(recent)+len(older) == maxVisionReferences {
				return recent, older, true
			}
			candidate := visionCandidate{attachedAssetRef: ref, messageIndex: index, sourceID: message.ID, anchorID: message.ID}
			if isLatest {
				recent = append(recent, candidate)
			} else {
				older = append(older, candidate)
			}
		}
	}
	return recent, older, limited
}

func (s AgentLoopService) addRequestImages(ctx context.Context, sessionID string, model modelport.ChatModelPort, turn *turnImages, snapshot contextmgr.Snapshot, transcript []domainmessage.Message, budget contextmgr.RequestBudget, request *modelport.GenerateRequest) (int, error) {
	turn.projected = nil
	candidates, limited := turn.imageCandidates(snapshot.Messages, transcript)
	if len(candidates) == 0 && !limited {
		return 0, nil
	}
	modelSupport, ok := model.(modelport.ImageInputSupport)
	compatible := ok && modelSupport.SupportsImageInput()
	imageHints := make(map[string]bool)
	for _, candidate := range candidates {
		imageHints[candidate.id] = imageHints[candidate.id] || candidate.imageHint
	}
	seen := make(map[string]bool)
	var omitted imageOmissions
	if limited {
		omitted.add("attachment list is too long; send fewer files")
	}
	var selected []projectedImage
	imageTokens := 0
	remaining := budget.AvailableTokens(snapshot.Messages)
	// Leave space for exact reasons when only part of a larger batch fits.
	reserve := 0
	if len(candidates) > 1 {
		reserve = 96
	}
	for _, candidate := range candidates {
		if seen[candidate.id] {
			continue
		}
		if len(seen) == maxVisionReferences {
			omitted.add("attachment list is too long; send fewer files")
			break
		}
		seen[candidate.id] = true
		asset := s.lookupImageAsset(ctx, sessionID, candidate.id, turn)
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if !imageHints[candidate.id] && !imageasset.LooksLikeImage(asset.asset.FileName, asset.asset.ContentType) {
			continue
		}
		problem := asset.problem
		var caption domainmessage.Message
		captionTokens := 0
		if candidate.toolResult {
			caption = domainmessage.SyntheticUserText(domainmessage.SyntheticReasonHookContext, "Image from read_asset (asset_id: "+candidate.id+").")
			captionTokens = contextmgr.EstimateMessagesTokens([]domainmessage.Message{caption})
		}
		switch {
		case problem != "":
		case !compatible:
			problem = "select a model using a Chat Completions adapter that accepts images"
		case len(selected) == modelport.MaxInputImages:
			problem = "too many images for this request; send fewer images"
		case remaining-reserve-captionTokens < imageBaseTokens+imageTileTokens:
			problem = "the model context has insufficient space; shorten the request or use a larger context"
		default:
			s.loadImage(ctx, turn, asset)
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			problem = asset.problem
			if problem == "" && candidate.sha256 != "" && (asset.sha256 != candidate.sha256 || asset.image.MIMEType != candidate.mimeType) {
				problem = "the image no longer matches the previously supplied attachment"
			}
			if problem == "" && asset.tokens+captionTokens > remaining-reserve {
				problem = "the model context has insufficient space; try a smaller image or a larger context"
			}
		}
		if problem != "" {
			omitted.add(candidate.id + ": " + problem)
			continue
		}
		remaining -= asset.tokens + captionTokens
		imageTokens += asset.tokens
		selected = append(selected, projectedImage{candidate: candidate, image: asset.image, caption: caption, sha256: asset.sha256})
	}
	feedback, err := omitted.feedback(remaining + budget.SafetyTokens)
	if err != nil {
		return 0, err
	}
	if len(selected) > 0 || feedback.IsSynthetic() {
		turn.projected = applyImageProjection(request, selected, feedback)
	}
	return imageTokens, nil
}

func (s AgentLoopService) lookupImageAsset(ctx context.Context, sessionID, id string, turn *turnImages) *resolvedImage {
	if item := turn.assets[id]; item != nil {
		return item
	}
	item := &resolvedImage{}
	if s.AssetFiles == nil || s.LookupSessionAsset == nil || sessionID == "" {
		item.problem = "attachment access is unavailable; try reattaching the image"
		return item
	}
	if len(turn.assets) == maxVisionReferences {
		item.problem = "too many attachments for this turn; try sending fewer files"
		return item
	}
	if turn.assets == nil {
		turn.assets = make(map[string]*resolvedImage)
	}
	turn.assets[id] = item
	asset, err := s.LookupSessionAsset(ctx, sessionID, id)
	if err != nil || asset.AssetID != id || asset.Size <= 0 {
		item.problem = "the attachment is unavailable in this session; try reattaching it"
		return item
	}
	item.asset = asset
	return item
}

func (s AgentLoopService) loadImage(ctx context.Context, turn *turnImages, item *resolvedImage) {
	if len(item.image.Bytes()) != 0 || item.problem != "" {
		return
	}
	asset := item.asset
	if asset.Size > modelport.MaxInputImageBytes {
		item.problem = "the image exceeds the 5 MiB limit; try a smaller JPEG or PNG"
		return
	}
	if turn.downloaded >= modelport.MaxInputImages || asset.Size > int64(modelport.MaxTotalInputImageBytes-turn.downloadSize) {
		item.problem = "the turn reached its image count or total byte limit; try fewer or smaller images"
		return
	}
	turn.downloaded++
	turn.downloadSize += int(asset.Size)
	object, err := s.AssetFiles.Download(ctx, files.DownloadRequest{AssetID: asset.AssetID, MaxBytes: asset.Size})
	if err != nil {
		item.problem = "could not read the attached image; try reattaching it"
		return
	}
	if object.Truncated || object.Asset.AssetID != asset.AssetID || object.Asset.Size != asset.Size ||
		int64(len(object.Data)) != asset.Size || asset.BusinessID != "" && object.Asset.BusinessID != asset.BusinessID ||
		asset.Checksum != "" && !matchesImageChecksum(object.Data, asset.Checksum) {
		item.problem = "the download did not match the attachment record; try reattaching it"
		return
	}
	decoded, mimeType, err := imageasset.Decode(object.Data)
	if err != nil {
		if errors.Is(err, imageasset.ErrTooLarge) {
			item.problem = "the image exceeds the resolution limit; resize it and attach it again"
		} else {
			item.problem = "the file is not a complete supported image; attach a JPEG or PNG"
		}
		return
	}
	width, height := decoded.Bounds().Dx(), decoded.Bounds().Dy()
	item.tokens = imageBaseTokens + imageTileTokens*((width+imageTileSide-1)/imageTileSide)*((height+imageTileSide-1)/imageTileSide)
	item.image = modelport.NewInputImage(mimeType, object.Data)
	item.sha256 = imageChecksum(object.Data)
}

func imageChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func matchesImageChecksum(data []byte, want string) bool {
	return strings.EqualFold(imageChecksum(data), want)
}

type projectedImage struct {
	candidate visionCandidate
	image     modelport.InputImage
	caption   domainmessage.Message
	sha256    string
}

func applyImageProjection(request *modelport.GenerateRequest, selected []projectedImage, feedback domainmessage.Message) []domainmessage.ImageReceipt {
	messages := make([]domainmessage.Message, 0, len(request.Messages)+len(selected)+1)
	images := make([]modelport.InputImage, 0, len(selected))
	var receipts []domainmessage.ImageReceipt
	for index, message := range request.Messages {
		messages = append(messages, message)
		userIndex := len(messages) - 1
		for _, item := range selected {
			if item.candidate.messageIndex != index {
				continue
			}
			image := item.image
			image.MessageIndex = userIndex
			if item.candidate.toolResult {
				messages = append(messages, item.caption)
				image.MessageIndex = len(messages) - 1
			}
			images = append(images, image)
			if item.candidate.sourceID != "" {
				receipts = append(receipts, domainmessage.ImageReceipt{
					Version: imageReceiptVersion, AssetID: item.candidate.id, SHA256: item.sha256, MIMEType: image.MIMEType,
					SourceMessageID: item.candidate.sourceID, AnchorMessageID: item.candidate.anchorID, ToolCallID: item.candidate.callID,
				})
			}
		}
	}
	if feedback.IsSynthetic() {
		messages = append(messages, feedback)
	}
	request.Messages, request.Images = messages, images
	return receipts
}

type imageOmissions struct {
	details []string
	count   int
}

func (omitted *imageOmissions) add(detail string) {
	if len(omitted.details) < modelport.MaxInputImages {
		omitted.details = append(omitted.details, detail)
	}
	omitted.count++
}

func (omitted imageOmissions) feedback(tokens int) (domainmessage.Message, error) {
	if omitted.count == 0 {
		return domainmessage.Message{}, nil
	}
	text := "Image content was not provided for these attachments:\n- " + strings.Join(omitted.details, "\n- ")
	if more := omitted.count - len(omitted.details); more > 0 {
		text += fmt.Sprintf("\n- %d more", more)
	}
	feedback := domainmessage.SyntheticUserText(domainmessage.SyntheticReasonHookContext, text)
	if contextmgr.EstimateMessagesTokens([]domainmessage.Message{feedback}) <= tokens {
		return feedback, nil
	}
	feedback = domainmessage.SyntheticUserText(domainmessage.SyntheticReasonHookContext, "Image content was not provided for some attachments. Try fewer or smaller JPEG or PNG images and a model that supports images.")
	if contextmgr.EstimateMessagesTokens([]domainmessage.Message{feedback}) <= tokens {
		return feedback, nil
	}
	return domainmessage.Message{}, errors.New("image content was not provided: the context is full; shorten the request or use a larger context")
}
