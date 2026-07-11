package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestLayerDependenciesPointInward(t *testing.T) {
	coreRoot := coreDirectory(t)

	err := filepath.WalkDir(coreRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relative, err := filepath.Rel(coreRoot, path)
		if err != nil {
			return err
		}
		layer := topLevelDirectory(relative)
		if layer != "domain" && layer != "port" && layer != "application" && layer != "service" && layer != "remote" {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if forbiddenDependency(layer, importPath) {
				t.Errorf("%s layer imports forbidden dependency %s in %s", layer, importPath, relative)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInnerLayersDoNotContainTransportOrPersistenceTags(t *testing.T) {
	coreRoot := coreDirectory(t)
	checkedLayers := map[string]bool{
		"application": true,
		"domain":      true,
		"port":        true,
		"service":     true,
	}

	err := filepath.WalkDir(coreRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(coreRoot, path)
		if err != nil {
			return err
		}
		if !checkedLayers[topLevelDirectory(relative)] {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		for _, tag := range []string{"json:\"", "bson:\"", "yaml:\""} {
			if strings.Contains(text, tag) {
				t.Errorf("inner layer contains serialization tag %q in %s", tag, relative)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInterfaceFilesDoNotMixStructObjects(t *testing.T) {
	coreRoot := coreDirectory(t)
	checkedLayers := map[string]bool{"application": true, "port": true}

	err := filepath.WalkDir(coreRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(coreRoot, path)
		if err != nil {
			return err
		}
		if !checkedLayers[topLevelDirectory(relative)] {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		hasInterface := false
		hasStruct := false
		ast.Inspect(file, func(node ast.Node) bool {
			typeSpec, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			switch typeSpec.Type.(type) {
			case *ast.InterfaceType:
				hasInterface = true
			case *ast.StructType:
				hasStruct = true
			}
			return true
		})
		if hasInterface && hasStruct {
			t.Errorf("interface file also contains struct objects: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicationPackageRolesDoNotMixDeclarations(t *testing.T) {
	applicationRoot := filepath.Join(coreDirectory(t), "application")

	err := filepath.WalkDir(applicationRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		role := filepath.Base(filepath.Dir(path))
		if role != "api" && role != "port" && role != "command" && role != "result" && role != "service" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			typeSpec, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			relative, _ := filepath.Rel(applicationRoot, path)
			switch typeSpec.Type.(type) {
			case *ast.StructType:
				if role == "api" || role == "port" {
					t.Errorf("%s package contains struct declaration: %s", role, relative)
				}
			case *ast.InterfaceType:
				if role == "command" || role == "result" || role == "service" {
					t.Errorf("%s package contains interface declaration: %s", role, relative)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStructuredApplicationModuleRootsAreContainers(t *testing.T) {
	applicationRoot := filepath.Join(coreDirectory(t), "application")
	for _, module := range []string{"chat", "model", "plan", "runtime", "session", "skill", "tool"} {
		entries, err := os.ReadDir(filepath.Join(applicationRoot, module))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "doc.go" {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".go") {
				t.Errorf("application/%s root contains production declaration file: %s", module, entry.Name())
			}
		}
	}
}

func TestStructuredPersistenceAdapterRootsAreContainers(t *testing.T) {
	persistenceRoot := filepath.Join(coreDirectory(t), "adapter", "persistence")
	for _, module := range []string{"chatmessage", "toolrecords"} {
		entries, err := os.ReadDir(filepath.Join(persistenceRoot, module))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "doc.go" {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".go") {
				t.Errorf("persistence/%s root contains production declaration file: %s", module, entry.Name())
			}
		}
	}
}

func TestMongoPersistenceRolesStaySeparated(t *testing.T) {
	mongoRoot := filepath.Join(coreDirectory(t), "adapter", "persistence", "mongo")

	err := filepath.WalkDir(mongoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(content), "bson:\"") && filepath.Base(filepath.Dir(path)) != "po" {
			relative, _ := filepath.Rel(mongoRoot, path)
			t.Errorf("mongo BSON document is outside po package: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"document.go", "mapper.go", "store.go"} {
		if _, err := os.Stat(filepath.Join(mongoRoot, name)); err == nil {
			t.Errorf("mongo root contains mixed persistence role file: %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestProductionCodeDoesNotImportLegacyPackages(t *testing.T) {
	coreRoot := coreDirectory(t)

	err := filepath.WalkDir(coreRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(coreRoot, path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if strings.HasPrefix(importPath, "myai/core/store/") || strings.HasPrefix(importPath, "myai/utills") {
				t.Errorf("production code imports legacy package %s in %s", importPath, relative)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func coreDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func topLevelDirectory(relative string) string {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 2 {
		return strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
	}
	return parts[0]
}

func forbiddenDependency(layer string, importPath string) bool {
	forbidden := map[string][]string{
		"domain":      {"myai/core/application/", "myai/core/adapter/", "myai/core/service", "myai/core/remote/"},
		"port":        {"myai/core/application/", "myai/core/adapter/", "myai/core/service", "myai/core/remote/"},
		"application": {"myai/core/adapter/", "myai/core/service", "myai/core/remote/"},
		"service":     {"myai/core/adapter/"},
		"remote":      {"myai/core/adapter/"},
	}
	for _, prefix := range forbidden[layer] {
		if strings.HasPrefix(importPath, prefix) {
			return true
		}
	}
	return false
}
