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
	reads        map[string]string // call ID -> asset ID from an actually executed read_asset
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
	problem string
}

type visionCandidate struct {
	attachedAssetRef
	messageIndex int
	toolResult   bool
}

func (turn *turnImages) acceptReads(calls []domainmessage.ToolCall, messages []domainmessage.Message) {
	assets := make(map[string]string)
	for _, call := range calls {
		if id := readAssetCallID(call); id != "" {
			assets[call.ID] = id
		}
	}
	if len(assets) == 0 {
		return
	}
	for _, message := range messages {
		if message.Role != domainmessage.RoleTool {
			continue
		}
		for _, part := range message.Parts {
			result := part.ToolResult
			if part.Type != domainmessage.PartToolResult || result == nil {
				continue
			}
			if id := assets[result.ToolCallID]; id != "" && isImageReadResult(result, id) {
				if turn.reads == nil {
					turn.reads = make(map[string]string)
				}
				turn.reads[result.ToolCallID] = id
			}
		}
	}
}

func isImageReadResult(result *domainmessage.ToolResult, assetID string) bool {
	if result.Name != "read_asset" || result.Status != domaintool.ResultStatusSuccess || result.Truncated {
		return false
	}
	content := result.Content
	if result.PromptTruncated {
		content = result.FullContent
	}
	var reference map[string]any
	return len(content) <= 4096 && json.Unmarshal([]byte(content), &reference) == nil &&
		reference["asset_id"] == assetID && reference["image_reference"] == true &&
		(reference["truncated"] == nil || reference["truncated"] == false)
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
	if len(turn.reads) == 0 {
		return nil
	}
	var selected []visionCandidate
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		if message.Role != domainmessage.RoleAssistant || !message.HasToolCall() {
			continue
		}
		// prepareSnapshot already verified complete pairs. Find the last result
		// of the whole batch before inserting any request-only user content.
		end := index
		batch := make(map[string]string)
		for end+1 < len(messages) && messages[end+1].Role == domainmessage.RoleTool {
			end++
			for _, part := range messages[end].Parts {
				if part.Type != domainmessage.PartToolResult || part.ToolResult == nil {
					continue
				}
				result := part.ToolResult
				if id := turn.reads[result.ToolCallID]; id != "" && isImageReadResult(result, id) {
					batch[result.ToolCallID] = id
				}
			}
		}
		for _, part := range message.Parts {
			if part.Type != domainmessage.PartToolCall || part.ToolCall == nil {
				continue
			}
			call := *part.ToolCall
			if id := batch[call.ID]; id != "" && id == readAssetCallID(call) {
				selected = append(selected, visionCandidate{attachedAssetRef: attachedAssetRef{id: id, imageHint: true}, messageIndex: end, toolResult: true})
			}
		}
		index = end
	}
	return selected
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
			candidate := visionCandidate{attachedAssetRef: ref, messageIndex: index}
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
	recent, older, limited := selectedUserAttachments(snapshot.Messages, transcript, turn.resumingSyntheticInput)
	toolImages := turn.selectedToolImages(snapshot.Messages)
	if len(recent)+len(older)+len(toolImages) == 0 && !limited {
		return 0, nil
	}
	modelSupport, ok := model.(modelport.ImageInputSupport)
	compatible := ok && modelSupport.SupportsImageInput()
	// Prefer an explicit user attachment's location even if read_asset returned
	// the same asset in this turn. Keep recent user input ahead of older history.
	locations := make(map[string]visionCandidate)
	for _, candidate := range append(recent, older...) {
		if _, exists := locations[candidate.id]; !exists {
			locations[candidate.id] = candidate
		}
	}
	candidates := append(recent, toolImages...)
	candidates = append(candidates, older...)
	imageHints := make(map[string]bool)
	for _, candidate := range candidates {
		if candidate.imageHint {
			imageHints[candidate.id] = true
		}
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
		if direct, exists := locations[candidate.id]; exists && candidate.toolResult {
			candidate = direct
		}
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
		selected = append(selected, projectedImage{candidate: candidate, image: asset.image, caption: caption})
	}
	feedback, err := omitted.feedback(remaining + budget.SafetyTokens)
	if err != nil {
		return 0, err
	}
	if len(selected) > 0 || feedback.IsSynthetic() {
		applyImageProjection(request, selected, feedback)
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
}

func matchesImageChecksum(data []byte, want string) bool {
	sum := sha256.Sum256(data)
	return strings.EqualFold(hex.EncodeToString(sum[:]), want)
}

type projectedImage struct {
	candidate visionCandidate
	image     modelport.InputImage
	caption   domainmessage.Message
}

func applyImageProjection(request *modelport.GenerateRequest, selected []projectedImage, feedback domainmessage.Message) {
	messages := make([]domainmessage.Message, 0, len(request.Messages)+len(selected)+1)
	images := make([]modelport.InputImage, 0, len(selected))
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
		}
	}
	if feedback.IsSynthetic() {
		messages = append(messages, feedback)
	}
	request.Messages, request.Images = messages, images
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
