package cmd

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"

	modelcommand "myai/core/application/model/command"
	domainmodel "myai/core/domain/model"
	"myai/core/llm"
	"myai/core/service"
)

type chatLifecycleService struct {
	chatSessionService
	send  func(context.Context, string, llm.ChatStreamHandler) (service.ChatResponse, error)
	added []modelcommand.AddConfig
}

func (*chatLifecycleService) CurrentSessionID() string { return "session" }
func (*chatLifecycleService) CurrentModelID() string   { return "local" }
func (*chatLifecycleService) ListModels() []llm.ModelInfo {
	return nil
}
func (s *chatLifecycleService) SendMessageStream(ctx context.Context, input string, handler llm.ChatStreamHandler) (service.ChatResponse, error) {
	return s.send(ctx, input, handler)
}
func (s *chatLifecycleService) AddModelConfig(ctx context.Context, config modelcommand.AddConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.added = append(s.added, config)
	return nil
}

// Every read after prefix is consumed blocks on release, like an idle stdin.
// Close records attempts to close input that the CLI does not own.
type blockedChatTestInput struct {
	prefix  string
	late    string
	started chan struct{}
	release chan struct{}
	reads   int
	closed  bool
}

func (input *blockedChatTestInput) Read(buffer []byte) (int, error) {
	input.reads++
	if input.prefix != "" {
		n := copy(buffer, input.prefix)
		input.prefix = input.prefix[n:]
		return n, nil
	}
	close(input.started)
	<-input.release
	return copy(buffer, input.late), io.EOF
}

func (input *blockedChatTestInput) Close() error {
	input.closed = true
	return nil
}

func TestChatInputIsDemandDrivenAndCancellationDoesNotCloseStdin(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		source := &blockedChatTestInput{
			prefix: "first\nsecond\n", started: make(chan struct{}), release: make(chan struct{}),
		}
		defer close(source.release)
		input := newChatInput(ctx, source)
		synctest.Wait()
		if source.reads != 0 {
			t.Fatal("input worker read before the first prompt requested input")
		}
		scanner := bufio.NewScanner(input)
		for _, want := range []string{"first", "second"} {
			if !scanner.Scan() || scanner.Text() != want {
				t.Fatalf("shared scanner lost prompt input: got %q, error %v", scanner.Text(), scanner.Err())
			}
			synctest.Wait()
			if source.reads != 1 {
				t.Fatal("input worker read ahead while the scanner still had buffered lines")
			}
		}

		result := make(chan error, 1)
		go func() {
			if scanner.Scan() {
				result <- errors.New("scan succeeded after input cancellation")
				return
			}
			result <- scanner.Err()
		}()
		<-source.started
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked input did not return cancellation: %v", err)
		}
		if source.closed {
			t.Fatal("cancel closed the caller's input")
		}
		source.release <- struct{}{}
		<-input.done
	})
}

func TestChatInputCanceledReadKeepsCallerBufferOwnedByCaller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		source := &blockedChatTestInput{
			late: "late", started: make(chan struct{}), release: make(chan struct{}),
		}
		defer close(source.release)
		input := newChatInput(ctx, source)
		buffer := make([]byte, 4)
		result := make(chan error, 1)
		go func() {
			_, err := input.Read(buffer)
			result <- err
		}()
		<-source.started
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("read did not stop on cancellation: %v", err)
		}
		copy(buffer, "safe")
		source.release <- struct{}{}
		<-input.done
		if string(buffer) != "safe" {
			t.Fatal("underlying input wrote into a buffer after canceled Read returned")
		}
	})
}

func TestChatLoopCancellationReachesActiveGeneration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started := make(chan struct{})
		calls := 0
		chat := &chatLifecycleService{send: func(actual context.Context, text string, _ llm.ChatStreamHandler) (service.ChatResponse, error) {
			calls++
			if actual != ctx || text != "hello" {
				t.Errorf("generation did not receive the loop context/input")
			}
			close(started)
			<-actual.Done()
			return service.ChatResponse{}, actual.Err()
		}}
		done := make(chan struct{})
		go func() {
			runChatLoop(ctx, bufio.NewScanner(strings.NewReader("hello\nnext turn\n")), chat)
			close(done)
		}()
		<-started
		cancel()
		<-done
		if calls != 1 {
			t.Fatalf("canceled chat admitted another generation: %d calls", calls)
		}
	})
}

func TestChatLoopCancellationLeavesIdlePrompt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		source := &blockedChatTestInput{started: make(chan struct{}), release: make(chan struct{})}
		defer close(source.release)
		input := newChatInput(ctx, source)
		done := make(chan struct{})
		go func() {
			runChatLoop(ctx, bufio.NewScanner(input), &chatLifecycleService{})
			close(done)
		}()
		<-source.started
		cancel()
		<-done
		if source.closed {
			t.Fatal("exiting chat closed stdin")
		}
		source.release <- struct{}{}
		<-input.done
	})
}

func TestChatLoopCancellationUnblocksToolPermissionPrompt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		source := &blockedChatTestInput{prefix: "hello\n", started: make(chan struct{}), release: make(chan struct{})}
		defer close(source.release)
		input := newChatInput(ctx, source)
		allowed := true
		chat := &chatLifecycleService{send: func(actual context.Context, _ string, stream llm.ChatStreamHandler) (service.ChatResponse, error) {
			allowed = stream.OnToolAsk(llm.ToolPermissionRequest{Name: "write_file", Mode: "ask"})
			return service.ChatResponse{}, actual.Err()
		}}
		done := make(chan struct{})
		go func() {
			runChatLoop(ctx, bufio.NewScanner(input), chat)
			close(done)
		}()
		<-source.started
		cancel()
		<-done
		if allowed {
			t.Fatal("canceled permission prompt approved the tool")
		}
		source.release <- struct{}{}
		<-input.done
	})
}

func TestChatSharedScannerPreservesInteractiveModelAndPermissionInputs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		lines := []string{
			"/model add", "local", "Local model", "ollama", // command, id, name, protocol
			"", "", "", "", "", // provider, auth, base URL, key, model name defaults
			"", "", "", // generation defaults
			"hello", "yes", "/exit", "",
		}
		input := newChatInput(ctx, strings.NewReader(strings.Join(lines, "\n")))
		calls := 0
		chat := &chatLifecycleService{send: func(_ context.Context, text string, stream llm.ChatStreamHandler) (service.ChatResponse, error) {
			calls++
			if text != "hello" || !stream.OnToolAsk(llm.ToolPermissionRequest{Name: "write_file", Mode: "ask"}) {
				t.Error("shared scanner did not preserve the generation and permission inputs")
			}
			return service.ChatResponse{}, nil
		}}
		runChatLoop(ctx, bufio.NewScanner(input), chat)
		if calls != 1 || len(chat.added) != 1 {
			t.Fatalf("interactive inputs went to the wrong consumer: calls=%d models=%d", calls, len(chat.added))
		}
		config := chat.added[0]
		if config.ID != "local" || config.Name != "Local model" || config.Protocol != domainmodel.ProtocolOllamaChat || config.AuthType != domainmodel.AuthTypeNone {
			t.Fatal("interactive model fields were lost or reordered")
		}
		cancel()
		<-input.done
	})
}

func TestAddModelInteractiveCancellationStopsBlockedField(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		source := &blockedChatTestInput{prefix: "local\n", started: make(chan struct{}), release: make(chan struct{})}
		defer close(source.release)
		input := newChatInput(ctx, source)
		chat := &chatLifecycleService{}
		result := make(chan error, 1)
		go func() { result <- addModelInteractive(ctx, bufio.NewScanner(input), chat) }()
		<-source.started
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("interactive field did not return cancellation: %v", err)
		}
		if len(chat.added) != 0 {
			t.Fatal("canceled form saved a model")
		}
		source.release <- struct{}{}
		<-input.done
	})
}
