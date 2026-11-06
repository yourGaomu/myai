package pathpolicy

import "testing"

func TestGeneratedDirectoriesRetainSnapshotExclusions(t *testing.T) {
	for _, path := range []string{".idea/workspace.xml", ".expo/cache.json", "dist/assets/app.js", "build/output.txt", ".next/cache/demo", "src/node_modules/demo"} {
		if !ShouldSkipSnapshot(path) || !ShouldSkipDirectory(path) {
			t.Errorf("generated directory entered snapshot: %s", path)
		}
	}
	for _, path := range []string{"src/build.go", "src/dist.go", "src/idea.go", "docs/next.txt"} {
		if ShouldSkipSnapshot(path) || ShouldSkipDirectory(path) {
			t.Errorf("ordinary source file was excluded: %s", path)
		}
	}
}
