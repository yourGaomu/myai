package local

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

// Bound file data independently of the requested line count, including files
// with a single very long line. The continuation notice is added separately.
const maxReadFileBytes = 256 * 1024
const readFileBufferBytes = 32 * 1024

type ReadFileTool struct {
	workspace string
}

type readFileArgs struct {
	Path       string `json:"path"`
	Offset     int64  `json:"offset,omitempty"`
	Limit      *int64 `json:"limit,omitempty"`
	ByteOffset int64  `json:"byte_offset,omitempty"`
}

func NewReadFileToolWithWorkspace(workspace string) *ReadFileTool {
	return &ReadFileTool{workspace: workspace}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read a UTF-8 workspace text file, optionally from a 1-based line offset with a line limit. Returns at most 256 KiB of file data and a continuation instruction when truncated."
}

func (t *ReadFileTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to read.",
			},
			"offset": map[string]any{
				"type": "integer", "minimum": 1,
				"description": "1-based first line to read. Defaults to 1.",
			},
			"limit": map[string]any{
				"type": "integer", "minimum": 1,
				"description": "Maximum number of lines to read. File data is always capped at 256 KiB.",
			},
			"byte_offset": map[string]any{
				"type": "integer", "minimum": 0,
				"description": "Bytes already read in the first requested line. Use only the value in a continuation instruction for a long line.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

func (t *ReadFileTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	var input readFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return tooldef.ToolOutput{}, err
	}
	if input.Offset < 0 || (input.Limit != nil && *input.Limit <= 0) || input.ByteOffset < 0 {
		return tooldef.ToolOutput{}, errors.New("offset and limit must be positive, and byte_offset must be nonnegative")
	}
	if input.Offset == 0 {
		input.Offset = 1
	}
	workspace, err := toolWorkspace(toolruntime.WorkspaceRoot(ctx, t.workspace))
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	path, err := cleanWorkspacePath(workspace, input.Path)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	relative, err := filepath.Rel(workspace, path)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	defer root.Close()
	file, err := root.Open(relative)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if !info.Mode().IsRegular() {
		return tooldef.ToolOutput{}, errors.New("read_file requires a regular text file")
	}

	content, next, more, err := readTextRange(ctx, file, input)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if more {
		continuation, err := json.Marshal(next)
		if err != nil {
			return tooldef.ToolOutput{}, err
		}
		content += fmt.Sprintf("\n\n[read_file truncated: returned at most %d bytes of file data. Continue with read_file %s]", maxReadFileBytes, continuation)
	}
	return tooldef.SuccessOutput(content), nil
}

// Read runes through a fixed-size buffer so neither long lines nor large line
// offsets cause an unbounded allocation. ByteOffset makes even one long line
// resumable without returning an invalid UTF-8 prefix or losing any bytes.
func readTextRange(ctx context.Context, source io.Reader, input readFileArgs) (string, readFileArgs, bool, error) {
	reader := bufio.NewReaderSize(source, readFileBufferBytes)
	content := make([]byte, 0, readFileBufferBytes)
	line, byteInLine, completedLines := int64(1), int64(0), int64(0)
	for {
		if err := ctx.Err(); err != nil {
			return "", readFileArgs{}, false, err
		}
		r, size, err := reader.ReadRune()
		if errors.Is(err, io.EOF) {
			if line == input.Offset && byteInLine < input.ByteOffset {
				return "", readFileArgs{}, false, errors.New("byte_offset exceeds the requested line length")
			}
			return string(content), readFileArgs{}, false, nil
		}
		if err != nil {
			return "", readFileArgs{}, false, err
		}
		if (r == utf8.RuneError && size == 1) || (r < 0x20 && r != '\n' && r != '\r' && r != '\t' && r != '\f') || r == 0x7f {
			return "", readFileArgs{}, false, errors.New("file is binary or is not valid UTF-8 text; use a binary-aware tool instead")
		}
		skipping := line < input.Offset || (line == input.Offset && byteInLine < input.ByteOffset)
		if skipping {
			if line == input.Offset && (byteInLine+int64(size) > input.ByteOffset || r == '\n') {
				return "", readFileArgs{}, false, errors.New("byte_offset must be a UTF-8 boundary within the requested line")
			}
		} else {
			if len(content)+size > maxReadFileBytes {
				if err := reader.UnreadRune(); err != nil {
					return "", readFileArgs{}, false, err
				}
				break
			}
			content = utf8.AppendRune(content, r)
			if r == '\n' {
				completedLines++
			}
		}
		byteInLine += int64(size)
		if r == '\n' {
			line++
			byteInLine = 0
		}
		if !skipping && (len(content) == maxReadFileBytes || (input.Limit != nil && completedLines == *input.Limit)) {
			break
		}
	}
	if _, err := reader.Peek(1); errors.Is(err, io.EOF) {
		return string(content), readFileArgs{}, false, nil
	} else if err != nil {
		return "", readFileArgs{}, false, err
	}
	next := readFileArgs{Path: input.Path, Offset: line, Limit: input.Limit, ByteOffset: byteInLine}
	if input.Limit != nil && completedLines < *input.Limit {
		remaining := *input.Limit - completedLines
		next.Limit = &remaining
	}
	return string(content), next, true, nil
}
