package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	domainimage "myai/core/domain/imagemodel"
	domaintool "myai/core/domain/tool"
	"myai/core/history"
	"myai/core/imageasset"
	filesport "myai/core/port/files"
	historyport "myai/core/port/history"
	imageport "myai/core/port/imagemodel"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

const maxImageReferences = 4

type GenerateImageTool struct {
	workspace      string
	models         imageport.Registry
	files          filesport.Service
	lookup         filesport.SessionAssetLookup
	history        historyport.StoreFactory
	recorder       historyRecorder
	localRegistrar filesport.SessionAssetRegistrar
}

type generateImageArgs struct {
	Prompt          string   `json:"prompt"`
	ModelID         string   `json:"model_id"`
	ReferenceImages []string `json:"reference_images"`
	MaskImage       string   `json:"mask_image"`
	OutputPath      string   `json:"output_path"`
	Size            string   `json:"size"`
	AspectRatio     string   `json:"aspect_ratio"`
	Background      string   `json:"background"`
	Quality         string   `json:"quality"`
	OutputFormat    string   `json:"output_format"`
}
type generateImageResult struct {
	AssetID       string `json:"asset_id"`
	Path          string `json:"path"`
	FileName      string `json:"file_name"`
	ContentType   string `json:"content_type"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Size          int64  `json:"size"`
	ModelID       string `json:"model_id"`
	MarkdownEmbed string `json:"markdown_embed"`
}

func NewGenerateImageTool(workspace string, models imageport.Registry, service filesport.Service, lookup filesport.SessionAssetLookup, historyFactory historyport.StoreFactory, registrars ...filesport.SessionAssetRegistrar) *GenerateImageTool {
	tool := &GenerateImageTool{workspace: workspace, models: models, files: service, lookup: lookup, history: historyFactory}
	if len(registrars) > 0 {
		tool.localRegistrar = registrars[0]
	}
	return tool
}
func (*GenerateImageTool) Name() string { return "generate_image" }
func (t *GenerateImageTool) Description() string {
	var available []string
	if t.models != nil {
		for _, config := range t.models.List() {
			if config.Enabled {
				supports := "reference images"
				if config.Protocol == domainimage.ProtocolGoogleImagen {
					supports = "text-to-image only"
				}
				defaultLabel := ""
				if config.IsDefault {
					defaultLabel = ", default"
				}
				available = append(available, fmt.Sprintf("%s (%s, %s%s)", config.ID, config.Name, supports, defaultLabel))
			}
		}
	}
	if len(available) == 0 {
		return "Generate or edit an image using an enabled image model. Configure an image model before use."
	}
	return "Generate or edit an image with an enabled model. Available models: " + strings.Join(available, "; ")
}
func (*GenerateImageTool) Permission() tooldef.Permission { return tooldef.PermissionWrite }
func (*GenerateImageTool) RequiresExplicitApproval() bool { return false }
func (*GenerateImageTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"prompt":           map[string]any{"type": "string", "description": "Describe the image to generate or how to edit the reference images."},
		"model_id":         map[string]any{"type": "string", "description": "Optional enabled image model ID; defaults to the configured default."},
		"reference_images": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": maxImageReferences, "description": "Up to four workspace-relative file paths or session asset IDs (ast_... / asset:ast_...)."},
		"mask_image":       map[string]any{"type": "string", "description": "Optional workspace image path or current-session asset ID mask."},
		"output_path":      map[string]any{"type": "string", "description": "Optional workspace path. Leave empty to use .myai/generated with a unique filename."},
		"size":             map[string]any{"type": "string"}, "aspect_ratio": map[string]any{"type": "string"},
		"background":    map[string]any{"type": "string", "enum": []string{"auto", "transparent", "opaque"}},
		"quality":       map[string]any{"type": "string", "enum": []string{"auto", "low", "medium", "high"}},
		"output_format": map[string]any{"type": "string", "enum": []string{"png", "jpeg", "webp"}},
	}, "required": []string{"prompt"}}
}
func (t *GenerateImageTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	var input generateImageArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return tooldef.ToolOutput{}, fmt.Errorf("decode generate_image arguments: %w", err)
	}
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Prompt == "" {
		return tooldef.ToolOutput{}, errors.New("prompt is required")
	}
	if len(input.ReferenceImages) > maxImageReferences {
		return tooldef.ToolOutput{}, fmt.Errorf("at most %d reference images are supported", maxImageReferences)
	}
	if t.models == nil {
		return tooldef.ToolOutput{}, errors.New("image model registry is not configured")
	}
	var config domainimage.Config
	var generator imageport.Generator
	var ok bool
	if strings.TrimSpace(input.ModelID) == "" {
		config, generator, ok = t.models.Default()
	} else {
		config, generator, ok = t.models.Get(strings.TrimSpace(input.ModelID))
	}
	if !ok {
		return tooldef.ToolOutput{}, errors.New("no enabled image model is available")
	}
	if (len(input.ReferenceImages) > 0 || input.MaskImage != "") && !generator.SupportsReferenceImages() {
		return tooldef.ToolOutput{}, errors.New("selected image model does not support reference images")
	}
	execution, _ := toolruntime.CurrentExecution(ctx)
	workspace := strings.TrimSpace(toolruntime.WorkspaceRoot(ctx, t.workspace))
	if execution.SandboxID != "" && execution.WorkspaceRoot == "" {
		return tooldef.ToolOutput{}, errors.New("session workspace is unavailable")
	}
	workspace, err := toolWorkspace(workspace)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	refs := make([]imageport.ReferenceImage, 0, len(input.ReferenceImages))
	for _, raw := range input.ReferenceImages {
		ref, err := t.loadReference(ctx, workspace, execution.SessionID, raw)
		if err != nil {
			return tooldef.ToolOutput{}, err
		}
		refs = append(refs, ref)
	}
	var mask *imageport.ReferenceImage
	if strings.TrimSpace(input.MaskImage) != "" {
		ref, err := t.loadReference(ctx, workspace, execution.SessionID, input.MaskImage)
		if err != nil {
			return tooldef.ToolOutput{}, err
		}
		mask = &ref
	}
	generated, err := generator.Generate(ctx, imageport.GenerateRequest{Prompt: input.Prompt, ReferenceImages: refs, Mask: mask, Size: input.Size, AspectRatio: input.AspectRatio, Background: input.Background, Quality: input.Quality, OutputFormat: input.OutputFormat})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	info, err := imageasset.Inspect(generated.Data)
	if err != nil || info.MIMEType != generated.MIMEType {
		return tooldef.ToolOutput{}, errors.New("image model returned invalid image data")
	}
	relative := strings.TrimSpace(input.OutputPath)
	if relative == "" {
		relative = filepath.ToSlash(filepath.Join(".myai", "generated", fmt.Sprintf("img_%s_%s%s", time.Now().Format("20060102_150405"), uuid.NewString()[:8], extensionForMIME(info.MIMEType))))
	}
	if filepath.Ext(relative) == "" {
		relative += extensionForMIME(info.MIMEType)
	} else if !strings.EqualFold(filepath.Ext(relative), extensionForMIME(info.MIMEType)) {
		return tooldef.ToolOutput{}, errors.New("output_path extension does not match generated image format")
	}
	absPath, err := safeGeneratedPath(workspace, relative)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	recorder, closeRecorder, err := openHistoryRecorder(ctx, t.recorder, workspace, t.history)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	defer closeRecorder()
	before, err := recorder.SnapshotPath(absPath)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if err := writeGeneratedFile(ctx, workspace, relative, generated.Data); err != nil {
		return tooldef.ToolOutput{}, err
	}
	rel, _ := filepath.Rel(workspace, absPath)
	rel = filepath.ToSlash(rel)
	if _, err := recorder.RecordFileChange(ctx, absPath, before, history.RecordCommand{Title: "generate_image " + rel, Reason: "generate image", SessionID: execution.SessionID, RequestID: execution.RequestID}); err != nil {
		return tooldef.ToolOutput{}, fmt.Errorf("image saved but history snapshot failed: %w", err)
	}
	sum := sha256.Sum256(generated.Data)
	checksum := hex.EncodeToString(sum[:])
	fileName := filepath.Base(rel)
	assetID, businessID := "", ""
	if t.files != nil {
		asset, err := t.files.Upload(ctx, filesport.UploadRequest{Reader: bytes.NewReader(generated.Data), FileName: fileName, ContentType: info.MIMEType, Size: int64(len(generated.Data))})
		if err != nil {
			return tooldef.ToolOutput{}, err
		}
		assetID, businessID = asset.AssetID, asset.BusinessID
		if !filesport.IsSessionAssetID(assetID) {
			return tooldef.ToolOutput{}, errors.New("upload backend returned an invalid asset_id")
		}
	} else {
		assetID = "ast_local_" + uuid.NewString()
		if t.localRegistrar != nil {
			if err := t.localRegistrar.RegisterLocalSessionAsset(ctx, execution.SessionID, filesport.Asset{AssetID: assetID, FileName: fileName, ContentType: info.MIMEType, Size: int64(len(generated.Data)), Checksum: checksum}.WithLocalImage(rel, workspace)); err != nil {
				return tooldef.ToolOutput{}, err
			}
		}
	}
	result := generateImageResult{AssetID: assetID, Path: rel, FileName: fileName, ContentType: info.MIMEType, Width: info.Width, Height: info.Height, Size: int64(len(generated.Data)), ModelID: config.ID, MarkdownEmbed: "![Generated image](asset:" + assetID + ")"}
	encoded, err := json.Marshal(result)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	output := tooldef.SuccessOutput(string(encoded))
	output.SharedAsset = &domaintool.SharedAsset{AssetID: assetID, BusinessID: businessID, LocalPath: rel, FileName: fileName, ContentType: info.MIMEType, Size: int64(len(generated.Data)), Checksum: checksum}
	return output, nil
}

func extensionForMIME(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".img"
	}
}
func (t *GenerateImageTool) loadReference(ctx context.Context, workspace, sessionID, raw string) (imageport.ReferenceImage, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "asset:") {
		raw = strings.TrimPrefix(raw, "asset:")
	}
	if strings.HasPrefix(raw, "ast_") {
		if t.lookup == nil || sessionID == "" {
			return imageport.ReferenceImage{}, errors.New("session image assets are unavailable")
		}
		asset, err := t.lookup(ctx, sessionID, raw)
		if err != nil || asset.AssetID != raw || asset.Size <= 0 || asset.Size > imageasset.MaxSourceBytes {
			return imageport.ReferenceImage{}, filesport.ErrSessionAssetUnavailable
		}
		var data []byte
		if strings.HasPrefix(raw, "ast_local_") {
			data, err = readSessionAssetPath(ctx, asset)
		} else if t.files != nil {
			var object filesport.Download
			object, err = t.files.Download(ctx, filesport.DownloadRequest{AssetID: raw, MaxBytes: imageasset.MaxSourceBytes})
			if err == nil {
				if object.Truncated || int64(len(object.Data)) != asset.Size || object.Asset.AssetID != raw {
					err = filesport.ErrSessionAssetUnavailable
				} else {
					data = object.Data
				}
			}
		} else {
			err = filesport.ErrSessionAssetUnavailable
		}
		if err != nil {
			return imageport.ReferenceImage{}, err
		}
		return referenceImage(data, asset.FileName)
	}
	data, rel, err := readWorkspaceAttachment(ctx, workspace, raw)
	if err != nil {
		return imageport.ReferenceImage{}, err
	}
	return referenceImage(data, filepath.Base(rel))
}
func checksumMatches(data []byte, expected string) bool {
	sum := sha256.Sum256(data)
	return strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(expected))
}

func referenceImage(data []byte, name string) (imageport.ReferenceImage, error) {
	info, err := imageasset.Inspect(data)
	if err != nil {
		return imageport.ReferenceImage{}, err
	}
	if info.IsAnimated {
		img, _, err := imageasset.Decode(data)
		if err != nil {
			return imageport.ReferenceImage{}, err
		}
		var buf bytes.Buffer
		err = png.Encode(&buf, img)
		if err != nil {
			return imageport.ReferenceImage{}, err
		}
		data = buf.Bytes()
		info, err = imageasset.Inspect(data)
		if err != nil {
			return imageport.ReferenceImage{}, err
		}
	}
	return imageport.ReferenceImage{Data: data, MIMEType: info.MIMEType, FileName: name}, nil
}
func readSessionAssetPath(ctx context.Context, asset filesport.Asset) ([]byte, error) {
	if asset.WorkspaceRoot() == "" || asset.LocalPath() == "" || asset.Checksum == "" || asset.Size <= 0 || asset.Size > imageasset.MaxSourceBytes {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	slash := strings.ReplaceAll(asset.LocalPath(), "\\", "/")
	for _, part := range strings.Split(slash, "/") {
		if part == ".." || part == "" {
			return nil, filesport.ErrSessionAssetUnavailable
		}
	}
	relative := filepath.Clean(filepath.FromSlash(slash))
	if !filepath.IsLocal(relative) || relative == "." {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	workspace := filepath.Clean(asset.WorkspaceRoot())
	rootInfo, err := os.Lstat(workspace)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	defer func() { _ = root.Close() }()
	openedRoot, err := root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, openedRoot) {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for _, component := range parts[:len(parts)-1] {
		before, err := root.Lstat(component)
		if err != nil || !before.IsDir() || before.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return nil, filesport.ErrSessionAssetUnavailable
		}
		next, err := root.OpenRoot(component)
		if err != nil {
			return nil, filesport.ErrSessionAssetUnavailable
		}
		check, err := next.Stat(".")
		if err != nil || !os.SameFile(before, check) {
			_ = next.Close()
			return nil, filesport.ErrSessionAssetUnavailable
		}
		_ = root.Close()
		root = next
	}
	name := parts[len(parts)-1]
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() != asset.Size || before.Size() > imageasset.MaxSourceBytes {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || opened.Size() != asset.Size {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, imageasset.MaxSourceBytes+1))
	if err != nil || int64(len(data)) != asset.Size || len(data) > imageasset.MaxSourceBytes {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), asset.Checksum) {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	if _, err := imageasset.Inspect(data); err != nil {
		return nil, filesport.ErrSessionAssetUnavailable
	}
	return data, nil
}

func safeGeneratedPath(workspace, relative string) (string, error) {
	slash := strings.ReplaceAll(relative, "\\", "/")
	for _, p := range strings.Split(slash, "/") {
		if p == ".." || p == "" {
			return "", errors.New("output_path must be a local workspace path")
		}
	}
	path, err := cleanWorkspacePath(workspace, slash)
	if err != nil {
		return "", err
	}
	if !filepath.IsLocal(mustRelative(workspace, path)) {
		return "", errors.New("output_path must stay within the workspace")
	}
	return path, nil
}
func mustRelative(root, path string) string { rel, _ := filepath.Rel(root, path); return rel }
func writeGeneratedFile(ctx context.Context, workspace, relative string, data []byte) error {
	rootInfo, err := os.Lstat(workspace)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return errors.New("workspace root is not a regular directory")
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, opened) {
		return errors.New("workspace changed during image write")
	}
	rel := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(relative, "\\", "/")))
	if !filepath.IsLocal(rel) || rel == "." {
		return errors.New("invalid output path")
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		before, err := root.Lstat(part)
		if errors.Is(err, os.ErrNotExist) {
			if err = root.Mkdir(part, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			before, err = root.Lstat(part)
		}
		if err != nil || !before.IsDir() || before.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return errors.New("output path contains link or non-directory")
		}
		next, err := root.OpenRoot(part)
		if err != nil {
			return err
		}
		check, err := next.Stat(".")
		if err != nil || !os.SameFile(before, check) {
			_ = next.Close()
			return errors.New("output directory changed during write")
		}
		_ = root.Close()
		root = next
	}
	name := parts[len(parts)-1]
	if err := ctx.Err(); err != nil {
		return err
	}
	if existing, err := root.Lstat(name); err == nil {
		if !existing.Mode().IsRegular() {
			return errors.New("output path is not a regular file")
		}
		file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		opened, statErr := file.Stat()
		_ = file.Close()
		if statErr != nil || !os.SameFile(existing, opened) {
			return errors.New("output file changed during write")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary := "." + name + "." + uuid.NewString() + ".tmp"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = root.Remove(temporary)
		return err
	}
	if err := root.Rename(temporary, name); err != nil {
		_ = root.Remove(temporary)
		return err
	}
	return nil
}
