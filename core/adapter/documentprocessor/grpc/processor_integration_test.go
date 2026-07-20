package grpcprocessor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	domainknowledge "myai/core/domain/knowledge"
	documentprocessorport "myai/core/port/knowledge/documentprocessor"
)

func TestProcessorRunsConcurrentPythonWorkers(t *testing.T) {
	python, script := pythonRuntime(t)
	processor, err := New(context.Background(), Config{
		PythonExecutable: python,
		Script:           script,
		Transport:        TransportTCP,
		WorkerCount:      2,
		MaxPendingJobs:   4,
		StartupTimeout:   15 * time.Second,
		RequestTimeout:   15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer processor.Close()

	var waitGroup sync.WaitGroup
	errors := make(chan error, 4)
	for index := 0; index < 4; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			sink := &collectingSink{}
			document := validDocument(index)
			summary, err := processor.Process(context.Background(), documentprocessorport.Request{
				Document:        document,
				ParsingProfile:  validParsingProfile(),
				ChunkingProfile: validChunkingProfile(),
				Content:         strings.NewReader("你好世界"),
			}, sink)
			if err != nil {
				errors <- err
				return
			}
			if summary.ChunkCount != 2 || len(sink.chunks) != 2 {
				errors <- &integrationError{"unexpected chunk count"}
				return
			}
			if sink.chunks[0].StartOffset != 0 || sink.chunks[0].EndOffset != 6 || sink.chunks[1].StartOffset != 6 || sink.chunks[1].EndOffset != 12 {
				errors <- &integrationError{"unexpected UTF-8 offsets"}
				return
			}
			if sink.chunks[0].ContentHash == "" {
				errors <- &integrationError{"missing Go-computed content hash"}
			}
		}(index)
	}
	waitGroup.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}

	invalidProfile := validParsingProfile()
	invalidProfile.ParserID = "unknown-parser"
	_, err = processor.Process(context.Background(), documentprocessorport.Request{
		Document:        validDocument(5),
		ParsingProfile:  invalidProfile,
		ChunkingProfile: validChunkingProfile(),
		Content:         strings.NewReader("content"),
	}, &collectingSink{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument from Python worker, got %v", err)
	}

	sink := &collectingSink{}
	if _, err := processor.Process(context.Background(), documentprocessorport.Request{
		Document:        validDocument(6),
		ParsingProfile:  validParsingProfile(),
		ChunkingProfile: validChunkingProfile(),
		Content:         strings.NewReader("恢复正常"),
	}, sink); err != nil {
		t.Fatalf("worker was not reusable after a business error: %v", err)
	}
}

func TestProcessorReplacesCrashedPythonWorker(t *testing.T) {
	python, script := pythonRuntime(t)
	processor, err := New(context.Background(), Config{
		PythonExecutable: python,
		Script:           script,
		Transport:        TransportTCP,
		WorkerCount:      1,
		MaxPendingJobs:   1,
		StartupTimeout:   15 * time.Second,
		RequestTimeout:   15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer processor.Close()

	crashed := <-processor.pool.available
	if err := crashed.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-crashed.exited
	processor.pool.available <- crashed

	_, err = processor.Process(context.Background(), documentprocessorport.Request{
		Document:        validDocument(7),
		ParsingProfile:  validParsingProfile(),
		ChunkingProfile: validChunkingProfile(),
		Content:         strings.NewReader("first attempt"),
	}, &collectingSink{})
	if err == nil {
		t.Fatal("expected the request assigned to the crashed worker to fail")
	}

	if _, err := processor.Process(context.Background(), documentprocessorport.Request{
		Document:        validDocument(8),
		ParsingProfile:  validParsingProfile(),
		ChunkingProfile: validChunkingProfile(),
		Content:         strings.NewReader("replacement worker"),
	}, &collectingSink{}); err != nil {
		t.Fatalf("replacement worker did not recover the pool: %v", err)
	}
}

func pythonRuntime(t *testing.T) (string, string) {
	t.Helper()
	if os.Getenv("MYAI_RUN_PYTHON_PROCESSOR_TEST") == "" {
		t.Skip("set MYAI_RUN_PYTHON_PROCESSOR_TEST=1 to run the Python worker integration test")
	}
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skip("python is not available")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "document_processor", "main.py"))
	if err != nil {
		t.Fatal(err)
	}
	return python, script
}

type collectingSink struct {
	chunks []domainknowledge.ChunkDraft
}

func (sink *collectingSink) Accept(_ context.Context, chunks []domainknowledge.ChunkDraft) error {
	sink.chunks = append(sink.chunks, chunks...)
	return nil
}

type integrationError struct {
	message string
}

func (err *integrationError) Error() string {
	return err.message
}

func validDocument(index int) domainknowledge.Document {
	return domainknowledge.Document{
		ID:              "document-" + string(rune('a'+index)),
		KnowledgeBaseID: "knowledge-1",
		FileName:        "content.txt",
		ContentType:     "text/plain",
		ObjectKey:       "knowledge/content.txt",
		ContentHash:     "document-hash",
		Version:         1,
		Status:          domainknowledge.DocumentStatusUploaded,
	}
}

func validParsingProfile() domainknowledge.ParsingProfile {
	return domainknowledge.ParsingProfile{
		ID:            "parsing-1",
		Name:          "Python Text",
		ParserID:      "python-text",
		ParserVersion: "1",
	}
}

func validChunkingProfile() domainknowledge.ChunkingProfile {
	return domainknowledge.ChunkingProfile{
		ID:              "chunking-1",
		Name:            "Text",
		StrategyID:      "text",
		StrategyVersion: "1",
		MaxChunkSize:    2,
		Overlap:         0,
	}
}
