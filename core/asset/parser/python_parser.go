package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultPythonParserTimeout = 20 * time.Second

type pythonParseRequest struct {
	FilePath    string `json:"file_path"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	FileType    string `json:"file_type"`
	MaxChars    int    `json:"max_chars"`
}

type pythonParseResponse struct {
	Supported bool              `json:"supported"`
	Kind      string            `json:"kind"`
	Text      string            `json:"text"`
	Truncated bool              `json:"truncated"`
	Message   string            `json:"message"`
	Metadata  map[string]string `json:"metadata"`
}

func parsePython(ctx context.Context, fileType string, filePath string, request Request) (Result, error) {
	fileType = strings.TrimSpace(fileType)
	if fileType == "" {
		return Result{}, errors.New("file type is empty")
	}
	if strings.TrimSpace(request.FileName) == "" {
		return Result{}, errors.New("file name is empty")
	}
	if strings.TrimSpace(filePath) == "" {
		return Result{}, errors.New("file path is empty")
	}

	maxChars := request.MaxChars
	if maxChars <= 0 {
		maxChars = DefaultMaxTextChars
	}

	scriptPath, err := pythonScriptPath()
	if err != nil {
		return Result{}, err
	}

	payload, err := json.Marshal(pythonParseRequest{
		FilePath:    filePath,
		FileName:    request.FileName,
		ContentType: request.ContentType,
		FileType:    fileType,
		MaxChars:    maxChars,
	})
	if err != nil {
		return Result{}, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, defaultPythonParserTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, pythonBinary(), scriptPath)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if timeoutCtx.Err() != nil {
			return Result{}, timeoutCtx.Err()
		}
		return Result{}, fmt.Errorf("python parser failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var response pythonParseResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return Result{}, fmt.Errorf("decode python parser response: %w: %s", err, strings.TrimSpace(stdout.String()))
	}
	if response.Kind == "" {
		response.Kind = fileType
	}
	if response.Metadata == nil {
		response.Metadata = map[string]string{}
	}

	return Result{
		FileName:    request.FileName,
		ContentType: normalizeContentType(request.ContentType),
		Size:        len(request.Data),
		Text:        response.Text,
		Truncated:   response.Truncated,
		Supported:   response.Supported,
		Kind:        response.Kind,
		Message:     response.Message,
		Metadata:    response.Metadata,
	}, nil
}

func pythonScriptPath() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate python parser source directory")
	}
	path := filepath.Join(filepath.Dir(currentFile), "python", "ParsePDF.py")
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

func pythonBinary() string {
	if value := strings.TrimSpace(os.Getenv("MYAI_PYTHON_BIN")); value != "" {
		return value
	}
	return "python"
}
