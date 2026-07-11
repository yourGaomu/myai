package changes

import (
	"fmt"
	"strings"

	domainhistory "myai/core/domain/history"
)

const (
	maxDiffBytes       = 256 * 1024
	maxDiffLineProduct = 250000
)

func historyAdditionDiff(entry *domainhistory.FileSnapshot) (string, bool, bool) {
	if entry == nil {
		return "", false, false
	}
	return additionDiff(historyFileToSnapshot(*entry))
}

func historyDeletionDiff(entry *domainhistory.FileSnapshot) (string, bool, bool) {
	if entry == nil {
		return "", false, false
	}
	return deletionDiff(historyFileToSnapshot(*entry))
}

func historyModifiedDiff(base *domainhistory.FileSnapshot, now *domainhistory.FileSnapshot) (string, bool, bool) {
	if base == nil || now == nil {
		return "", false, false
	}
	return modifiedDiff(historyFileToSnapshot(*base), historyFileToSnapshot(*now))
}

func historyFileToSnapshot(value domainhistory.FileSnapshot) snapshotEntry {
	item := snapshotEntry{
		Path:      value.Path,
		Size:      value.Size,
		Hash:      value.Hash,
		Content:   value.Content,
		Binary:    value.Binary,
		TooLarge:  value.TooLarge,
		Mode:      value.Mode,
		Available: value.Available,
	}
	if value.Content != nil {
		item.Content = append([]byte(nil), value.Content...)
	}
	return item
}

func additionDiff(entry snapshotEntry) (string, bool, bool) {
	if entry.Binary || entry.TooLarge {
		return "", entry.TooLarge, true
	}
	diff, truncated := fileDiff("", string(entry.Content), entry.Path)
	return diff, truncated, false
}

func deletionDiff(entry snapshotEntry) (string, bool, bool) {
	if entry.Binary || entry.TooLarge || !entry.Available {
		return "", entry.TooLarge, true
	}
	diff, truncated := fileDiff(string(entry.Content), "", entry.Path)
	return diff, truncated, false
}

func modifiedDiff(base snapshotEntry, now snapshotEntry) (string, bool, bool) {
	if base.Binary || now.Binary || base.TooLarge || now.TooLarge || !base.Available || !now.Available {
		return "", base.TooLarge || now.TooLarge, true
	}
	diff, truncated := fileDiff(string(base.Content), string(now.Content), now.Path)
	return diff, truncated, false
}

func fileDiff(oldText string, newText string, path string) (string, bool) {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)
	truncated := false

	var builder strings.Builder
	builder.WriteString("diff --myai a/")
	builder.WriteString(path)
	builder.WriteString(" b/")
	builder.WriteString(path)
	builder.WriteString("\n--- a/")
	builder.WriteString(path)
	builder.WriteString("\n+++ b/")
	builder.WriteString(path)
	builder.WriteString(fmt.Sprintf("\n@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines)))

	var ops []lineOp
	if len(oldLines)*len(newLines) > maxDiffLineProduct {
		truncated = true
		ops = compactDiffLines(oldLines, newLines)
	} else {
		ops = diffLines(oldLines, newLines)
	}
	for _, op := range ops {
		switch op.kind {
		case "equal":
			builder.WriteString(" ")
		case "delete":
			builder.WriteString("-")
		case "insert":
			builder.WriteString("+")
		}
		builder.WriteString(op.text)
		if !strings.HasSuffix(op.text, "\n") {
			builder.WriteString("\n")
		}
	}

	diff := builder.String()
	if len(diff) > maxDiffBytes {
		return diff[:maxDiffBytes], true
	}
	return diff, truncated
}

type lineOp struct {
	kind string
	text string
}

func diffLines(oldLines []string, newLines []string) []lineOp {
	m, n := len(oldLines), len(newLines)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	ops := make([]lineOp, 0, m+n)
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			ops = append(ops, lineOp{kind: "equal", text: oldLines[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, lineOp{kind: "delete", text: oldLines[i]})
			i++
		} else {
			ops = append(ops, lineOp{kind: "insert", text: newLines[j]})
			j++
		}
	}
	for i < m {
		ops = append(ops, lineOp{kind: "delete", text: oldLines[i]})
		i++
	}
	for j < n {
		ops = append(ops, lineOp{kind: "insert", text: newLines[j]})
		j++
	}
	return ops
}

func compactDiffLines(oldLines []string, newLines []string) []lineOp {
	ops := make([]lineOp, 0, len(oldLines)+len(newLines))
	for _, line := range oldLines {
		ops = append(ops, lineOp{kind: "delete", text: line})
	}
	for _, line := range newLines {
		ops = append(ops, lineOp{kind: "insert", text: line})
	}
	return ops
}

func splitLines(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.SplitAfter(text, "\n")
}

func emptyDiffMessage(diff string, binary bool) string {
	if binary {
		return "Binary or oversized file diff is not available."
	}
	if strings.TrimSpace(diff) == "" {
		return "No diff is available for this path."
	}
	return ""
}
