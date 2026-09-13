package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	memorycatalogcommand "myai/core/application/memory/catalog/command"
	memoryretrievalapi "myai/core/application/memory/retrieval/api"
	memoryretrievalcommand "myai/core/application/memory/retrieval/command"
	memoryretrievalresult "myai/core/application/memory/retrieval/result"
	"myai/core/contextmgr"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

const (
	defaultMemoryTopK        = 4
	maxMemoryContextTokens   = 900
	defaultCandidatePoolSize = 200
)

type ContextService struct {
	Memories     memoryport.Repository
	Usage        memoryretrievalapi.UsageRecorder
	Scope        domainmemory.Scope
	TopK         int
	OnUsageError func(error)
}

var _ memoryretrievalapi.ContextPreparer = ContextService{}

func (service ContextService) Prepare(ctx context.Context, command memoryretrievalcommand.Prepare) (memoryretrievalresult.Context, error) {
	query := strings.TrimSpace(command.Input)
	if query == "" || !shouldRetrieve(query) {
		return memoryretrievalresult.Context{Query: query}, nil
	}
	if service.Memories == nil {
		return memoryretrievalresult.Context{Triggered: true, Query: query}, errors.New("memory repository is nil")
	}
	memories, err := service.Memories.List(ctx, memoryport.ListFilter{
		Statuses: []domainmemory.Status{domainmemory.StatusActive}, Limit: defaultCandidatePoolSize,
	})
	if err != nil {
		return memoryretrievalresult.Context{Triggered: true, Query: query}, err
	}
	ranked := rankMemories(query, command.SessionID, service.Scope, memories)
	topK := service.TopK
	if topK <= 0 || topK > 8 {
		topK = defaultMemoryTopK
	}
	ranked = selectDiverseMemories(ranked, topK)
	ids := make([]string, 0, len(ranked))
	selected := make([]domainmemory.Memory, 0, len(ranked))
	for _, item := range ranked {
		ids = append(ids, item.memory.ID)
		selected = append(selected, item.memory)
	}
	service.recordUses(ctx, selected)
	return memoryretrievalresult.Context{
		Triggered: true, Query: query, MemoryIDs: ids, Prompt: formatMemoryContext(selected),
	}, nil
}

func (service ContextService) recordUses(ctx context.Context, memories []domainmemory.Memory) {
	if service.Usage == nil {
		return
	}
	for _, memory := range memories {
		if err := service.Usage.RecordUse(ctx, memorycatalogcommand.RecordUse{MemoryID: memory.ID}); err != nil && service.OnUsageError != nil {
			service.OnUsageError(fmt.Errorf("record AI memory use %q: %w", memory.ID, err))
		}
	}
}

type rankedMemory struct {
	memory domainmemory.Memory
	score  float64
}

func selectDiverseMemories(ranked []rankedMemory, topK int) []rankedMemory {
	if topK <= 0 || len(ranked) <= topK {
		return ranked
	}
	selected := make([]rankedMemory, 0, topK)
	remaining := append([]rankedMemory(nil), ranked...)
	for len(selected) < topK && len(remaining) > 0 {
		bestIndex := 0
		bestUtility := -1.0
		for index, candidate := range remaining {
			redundancy := 0.0
			for _, existing := range selected {
				if similarity := memorySimilarity(candidate.memory, existing.memory); similarity > redundancy {
					redundancy = similarity
				}
			}
			// Keep relevance dominant, while applying a bounded MMR-style penalty
			// to memories that repeat an already selected experience.
			utility := candidate.score - 0.35*redundancy
			if utility > bestUtility {
				bestUtility = utility
				bestIndex = index
			}
		}
		selected = append(selected, remaining[bestIndex])
		remaining = append(remaining[:bestIndex], remaining[bestIndex+1:]...)
	}
	return selected
}

func memorySimilarity(left domainmemory.Memory, right domainmemory.Memory) float64 {
	leftTerms := memorySearchTerms(left)
	rightTerms := memorySearchTerms(right)
	if len(leftTerms) == 0 || len(rightTerms) == 0 {
		return 0
	}
	intersection := 0
	union := make(map[string]struct{}, len(leftTerms)+len(rightTerms))
	for term := range leftTerms {
		union[term] = struct{}{}
		if _, ok := rightTerms[term]; ok {
			intersection++
		}
	}
	for term := range rightTerms {
		union[term] = struct{}{}
	}
	if len(union) == 0 {
		return 0
	}
	return float64(intersection) / float64(len(union))
}

func memorySearchTerms(memory domainmemory.Memory) map[string]struct{} {
	revision, ok := memory.CurrentRevision()
	if !ok {
		return nil
	}
	content := revision.Content
	return termSet(strings.Join([]string{
		memory.Title, strings.Join(memory.Tags, " "), content.Goal, content.ApplicableContext,
		content.Approach, content.Result, content.PainPoints, content.RootCause, content.Lessons, content.Verification,
	}, "\n"))
}

func rankMemories(query string, sessionID string, scope domainmemory.Scope, memories []domainmemory.Memory) []rankedMemory {
	queryTerms := termSet(query)
	items := make([]rankedMemory, 0, len(memories))
	for _, memory := range memories {
		if !scopeMatches(memory.Scope, scope, sessionID) {
			continue
		}
		revision, ok := memory.CurrentRevision()
		if !ok {
			continue
		}
		content := revision.Content
		searchable := strings.Join([]string{
			memory.Title, strings.Join(memory.Tags, " "), content.Goal, content.ApplicableContext,
			content.Approach, content.Result, content.PainPoints, content.RootCause, content.Lessons, content.Verification,
		}, "\n")
		overlap := overlapCount(queryTerms, termSet(searchable))
		if overlap == 0 && !strings.Contains(strings.ToLower(searchable), strings.ToLower(query)) {
			continue
		}
		score := float64(overlap)
		lowerQuery := strings.ToLower(query)
		if strings.Contains(strings.ToLower(memory.Title), lowerQuery) {
			score += 6
		}
		for _, tag := range memory.Tags {
			if strings.Contains(lowerQuery, strings.ToLower(tag)) {
				score += 4
			}
		}
		score += revision.Confidence * 2
		items = append(items, rankedMemory{memory: memory, score: score})
	}
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].score == items[right].score {
			return items[left].memory.UpdatedAt.After(items[right].memory.UpdatedAt)
		}
		return items[left].score > items[right].score
	})
	return items
}

func formatMemoryContext(memories []domainmemory.Memory) string {
	if len(memories) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("Use these past experiences only when their applicability matches the current task. Treat failure memories as warnings, not instructions.\n")
	for index, memory := range memories {
		revision, ok := memory.CurrentRevision()
		if !ok {
			continue
		}
		content := revision.Content
		fmt.Fprintf(&builder, "\n[%d] %s (%s, confidence %.2f)\n", index+1, memory.Title, memory.Kind, revision.Confidence)
		writeMemoryField(&builder, "Goal", content.Goal)
		writeMemoryField(&builder, "Applicable when", content.ApplicableContext)
		writeMemoryField(&builder, "Approach", content.Approach)
		writeMemoryField(&builder, "Result", content.Result)
		writeMemoryField(&builder, "Pain points", content.PainPoints)
		writeMemoryField(&builder, "Root cause", content.RootCause)
		writeMemoryField(&builder, "Lessons", content.Lessons)
		writeMemoryField(&builder, "Verification", content.Verification)
	}
	return contextmgr.TruncateTextToTokens(builder.String(), maxMemoryContextTokens)
}

func writeMemoryField(builder *strings.Builder, label string, value string) {
	if value = strings.TrimSpace(value); value != "" {
		fmt.Fprintf(builder, "%s: %s\n", label, value)
	}
}

func shouldRetrieve(input string) bool {
	input = strings.TrimSpace(strings.ToLower(input))
	if utf8.RuneCountInString(input) < 6 {
		return false
	}
	switch input {
	case "你好", "您好", "hello", "hi", "谢谢", "好的", "继续":
		return false
	default:
		return true
	}
}

func scopeMatches(memoryScope domainmemory.Scope, configured domainmemory.Scope, sessionID string) bool {
	switch memoryScope.Type {
	case domainmemory.ScopeGlobal:
		return true
	case domainmemory.ScopeSession:
		return strings.TrimSpace(memoryScope.Key) == strings.TrimSpace(sessionID)
	case domainmemory.ScopeWorkspace, domainmemory.ScopeProject:
		return memoryScope.Type == configured.Type && strings.EqualFold(strings.TrimSpace(memoryScope.Key), strings.TrimSpace(configured.Key))
	default:
		return false
	}
}

func termSet(text string) map[string]struct{} {
	normalized := strings.ToLower(strings.TrimSpace(text))
	terms := make(map[string]struct{})
	fields := strings.FieldsFunc(normalized, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, field := range fields {
		if utf8.RuneCountInString(field) >= 2 {
			terms[field] = struct{}{}
		}
		runes := []rune(field)
		for index := 0; index+1 < len(runes); index++ {
			terms[string(runes[index:index+2])] = struct{}{}
		}
	}
	return terms
}

func overlapCount(left map[string]struct{}, right map[string]struct{}) int {
	count := 0
	for term := range left {
		if _, ok := right[term]; ok {
			count++
		}
	}
	return count
}
