package dream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	domaingeneration "myai/core/domain/generation"
	domainmemory "myai/core/domain/memory"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

type ModelConsolidator struct {
	Model modelport.ChatModelPort
}

func (consolidator ModelConsolidator) Consolidate(ctx context.Context, candidates []domainmemory.Candidate, existing []domainmemory.Memory) ([]domainmemory.DreamAction, error) {
	if consolidator.Model == nil {
		return nil, errors.New("dream consolidation model is nil")
	}
	evidence, err := json.Marshal(dreamEvidence{
		Candidates: candidateEvidenceItems(candidates),
		Memories:   memoryEvidenceItems(existing),
	})
	if err != nil {
		return nil, fmt.Errorf("encode dream evidence: %w", err)
	}
	settings := domaingeneration.SystemDefaults()
	settings.Temperature = 0.1
	settings.MaxOutputTokens = 3000
	generated, err := consolidator.Model.Generate(ctx, modelport.GenerateRequest{
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, dreamSystemPrompt()),
			domainmessage.Text(domainmessage.RoleUser, "Review these pending candidates and existing memories:\n"+string(evidence)),
		},
		Settings: settings,
	})
	if err != nil {
		return nil, fmt.Errorf("generate dream actions: %w", err)
	}
	output, err := decodeDreamOutput(generated.Content)
	if err != nil {
		return nil, err
	}
	actions := make([]domainmemory.DreamAction, 0, len(output.Actions))
	for _, item := range output.Actions {
		actions = append(actions, domainmemory.DreamAction{
			CandidateID: strings.TrimSpace(item.CandidateID),
			MemoryID:    strings.TrimSpace(item.MemoryID),
			Decision:    domainmemory.DreamDecision(strings.TrimSpace(item.Decision)),
			Reason:      strings.TrimSpace(item.Reason),
		})
	}
	return actions, nil
}

type dreamEvidence struct {
	Candidates []candidateEvidence `json:"candidates"`
	Memories   []memoryEvidence    `json:"existing_memories"`
}

type candidateEvidence struct {
	ID         string               `json:"id"`
	Title      string               `json:"title"`
	Kind       string               `json:"kind"`
	ScopeType  string               `json:"scope_type"`
	ScopeKey   string               `json:"scope_key,omitempty"`
	Tags       []string             `json:"tags,omitempty"`
	Content    domainmemory.Content `json:"content"`
	Confidence float64              `json:"confidence"`
}

type memoryEvidence struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Kind        string               `json:"kind"`
	ScopeType   string               `json:"scope_type"`
	ScopeKey    string               `json:"scope_key,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
	Version     int                  `json:"version"`
	HumanLocked bool                 `json:"human_locked"`
	Content     domainmemory.Content `json:"content"`
}

func candidateEvidenceItems(candidates []domainmemory.Candidate) []candidateEvidence {
	items := make([]candidateEvidence, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, candidateEvidence{
			ID: candidate.ID, Title: candidate.Title, Kind: string(candidate.Kind),
			ScopeType: string(candidate.Scope.Type), ScopeKey: candidate.Scope.Key,
			Tags: append([]string(nil), candidate.Tags...), Content: candidate.Content,
			Confidence: candidate.Confidence,
		})
	}
	return items
}

func memoryEvidenceItems(memories []domainmemory.Memory) []memoryEvidence {
	items := make([]memoryEvidence, 0, len(memories))
	for _, memory := range memories {
		revision, exists := memory.CurrentRevision()
		if !exists {
			continue
		}
		items = append(items, memoryEvidence{
			ID: memory.ID, Title: memory.Title, Kind: string(memory.Kind),
			ScopeType: string(memory.Scope.Type), ScopeKey: memory.Scope.Key,
			Tags: append([]string(nil), memory.Tags...), Version: memory.CurrentVersion,
			HumanLocked: memory.HumanLocked, Content: revision.Content,
		})
	}
	return items
}

type dreamOutput struct {
	Actions []dreamActionOutput `json:"actions"`
}

type dreamActionOutput struct {
	CandidateID string `json:"candidate_id"`
	MemoryID    string `json:"memory_id"`
	Decision    string `json:"decision"`
	Reason      string `json:"reason"`
}

func decodeDreamOutput(content string) (dreamOutput, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return dreamOutput{}, errors.New("dream consolidator returned no JSON object")
	}
	var output dreamOutput
	if err := json.Unmarshal([]byte(content[start:end+1]), &output); err != nil {
		return dreamOutput{}, fmt.Errorf("decode dream actions: %w", err)
	}
	return output, nil
}

func dreamSystemPrompt() string {
	return `You are the quality-control stage of an AI memory system.
Return exactly one JSON object with an "actions" array and no markdown.
Create exactly one action for every pending candidate. Each action contains candidate_id, memory_id, decision, and reason.
decision must be create, merge, keep_both, reject, or needs_review. Do not return supersede; automatic superseding is not yet supported.
Use create when no reusable memory covers the candidate. Use merge only when an active, non-human-locked memory has the same goal and compatible scope.
Use keep_both when records are related but apply to materially different contexts. Use reject only for unsupported, unsafe, duplicate-without-new-value, or non-reusable content.
Use needs_review whenever confidence is insufficient or a suitable target is human_locked. merge requires an existing memory_id; all other decisions must use an empty memory_id.
Never invent candidate IDs or memory IDs. Prefer preserving verified newer solutions, but do not modify any data yourself.`
}
