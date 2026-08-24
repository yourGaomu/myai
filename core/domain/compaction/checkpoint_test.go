package compaction

import (
	"strings"
	"testing"
	"time"
)

func TestParseSummaryExtractsDurableSections(t *testing.T) {
	parsed := ParseSummary(strings.TrimSpace(`
## 当前目标
修复上下文压缩
## 用户偏好与约束
- 偏好：保持缓存前缀稳定
- 约束：不能拆开工具调用
## 关键决策
- 使用消息范围和源哈希
## 修改文件
- core/contextmgr/context.go
## 工具与验证
- go test ./...
## 问题与根因
- 旧摘要没有来源校验
## 未完成任务与下一步
- 未完成：移动端展示
- 下一步：补协议
## 重要引用
- source hash
`))
	if parsed.CurrentGoal != "修复上下文压缩" {
		t.Fatalf("unexpected goal: %#v", parsed)
	}
	if len(parsed.Preferences) != 1 || len(parsed.Constraints) != 1 || len(parsed.Decisions) != 1 || len(parsed.ModifiedFiles) != 1 {
		t.Fatalf("unexpected parsed sections: %#v", parsed)
	}
	if len(parsed.OpenTasks) != 1 || len(parsed.NextSteps) != 1 {
		t.Fatalf("expected open tasks and next steps, got %#v", parsed)
	}
}

func TestCheckpointValidationRequiresVerifiableSource(t *testing.T) {
	summary := validJSONSummary()
	if _, err := NewCheckpoint(summary, 0, 2, "", now()); err == nil {
		t.Fatal("expected source hash validation error")
	}
	checkpoint, err := NewCheckpoint(summary, 0, 2, "hash", now())
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Version != CurrentVersion || checkpoint.SourceEndMessage != 2 {
		t.Fatalf("unexpected checkpoint: %#v", checkpoint)
	}
}

func validJSONSummary() string {
	return `{"current_goal":"summary","preferences":[],"constraints":[],"decisions":[],"completed_work":[],"modified_files":[],"tool_verification":[],"problems":[],"open_tasks":[],"next_steps":[],"references":[]}`
}

func now() (value time.Time) {
	return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
}
