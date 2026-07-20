package service

import (
	"strings"
	"unicode/utf8"

	retrievalport "myai/core/application/chat/retrieval/port"
)

type DefaultTriggerPolicy struct{}

var _ retrievalport.TriggerPolicy = DefaultTriggerPolicy{}

var retrievalSkipInputs = map[string]struct{}{
	"你好": {}, "您好": {}, "谢谢": {}, "好的": {}, "好": {}, "可以": {}, "继续": {},
	"嗯": {}, "收到": {}, "ok": {}, "hello": {}, "hi": {}, "thanks": {}, "continue": {},
}

var retrievalSignals = []string{
	"为什么", "如何", "怎么", "什么", "哪里", "哪个", "是否", "解释", "介绍", "查找", "搜索",
	"项目", "架构", "设计", "实现", "代码", "函数", "接口", "配置", "文档", "流程", "错误", "问题",
	"plan", "rag", "knowledge", "document", "architecture", "implementation", "function", "config", "error",
}

func (DefaultTriggerPolicy) ShouldRetrieve(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" || utf8.RuneCountInString(normalized) < 2 {
		return false
	}
	if _, skipped := retrievalSkipInputs[normalized]; skipped {
		return false
	}
	if strings.ContainsAny(normalized, "?？") {
		return true
	}
	for _, signal := range retrievalSignals {
		if strings.Contains(normalized, signal) {
			return true
		}
	}
	return utf8.RuneCountInString(normalized) >= 24
}
