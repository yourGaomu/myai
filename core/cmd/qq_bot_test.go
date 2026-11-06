package cmd

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
)

func TestOneBotUsesApplicationHistoryStoreFactory(t *testing.T) {
	_, testPath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate one-bot wiring")
	}
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(testPath), "qq_bot.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		constructor, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || constructor.Sel.Name != "NewWithStoreFactory" {
			return true
		}
		found++
		if len(call.Args) != 3 {
			t.Error("one-bot changes service must receive the application history factory")
			return false
		}
		factoryCall, ok := call.Args[2].(*ast.CallExpr)
		if !ok {
			t.Error("one-bot history factory is not resolved from the application")
			return false
		}
		factory, ok := factoryCall.Fun.(*ast.SelectorExpr)
		if !ok || factory.Sel.Name != "HistoryStoreFactory" {
			t.Error("one-bot uses a separate history database instead of HistoryStoreFactory")
			return false
		}
		appCall, ok := factory.X.(*ast.CallExpr)
		if !ok {
			t.Error("history factory does not belong to core.GetApp()")
			return false
		}
		app, ok := appCall.Fun.(*ast.SelectorExpr)
		if !ok || app.Sel.Name != "GetApp" {
			t.Error("history factory does not belong to core.GetApp()")
			return false
		}
		owner, ok := app.X.(*ast.Ident)
		if !ok || owner.Name != "core" {
			t.Error("history factory does not belong to core.GetApp()")
		}
		return false
	})
	if found != 1 {
		t.Fatalf("expected one shared-history changes service, found %d", found)
	}
}

func TestOneBotHonorsCanceledCommandContextBeforeInitialization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command := &cobra.Command{}
	command.SetContext(ctx)
	if err := oneBotCmd.RunE(command, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("one-bot ignored command cancellation: %v", err)
	}
}

func TestChatHonorsCanceledCommandContextBeforeInitialization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command := &cobra.Command{}
	command.SetContext(ctx)
	chatCmd.Run(command, nil)
}
