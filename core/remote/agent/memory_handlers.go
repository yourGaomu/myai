package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/gorilla/websocket"

	memorycommand "myai/core/application/memory/catalog/command"
	memorydreamcommand "myai/core/application/memory/dream/command"
	memoryextractioncommand "myai/core/application/memory/extraction/command"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
	"myai/core/remote/protocol"
)

func (a *Agent) requireMemoryService() error {
	if a.memoryService == nil {
		return errors.New("AI memory service is not configured")
	}
	return nil
}

func (a *Agent) requireMemoryExtractionService() error {
	if a.memoryExtraction == nil {
		return errors.New("AI memory extraction service is not configured")
	}
	return nil
}

func (a *Agent) requireMemoryDreamService() error {
	if a.memoryDream == nil {
		return errors.New("AI memory dream service is not configured")
	}
	return nil
}

func (a *Agent) handleAIMemoryList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireMemoryService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.AIMemoryListPayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory list failed: %w", err)
	}
	result, err := a.memoryService.List(ctx, memorycommand.List{Filter: aiMemoryListFilter(payload)})
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeAIMemoryListResult, message.RequestID, message.SessionID, aiMemoriesPayload(result.Memories, ""))
}

func (a *Agent) handleAIMemoryCreate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.AIMemoryCreatePayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory create failed: %w", err)
	}
	return a.mutateAIMemories(ctx, conn, message, "Memory created.", func() error {
		_, createErr := a.memoryService.Create(ctx, createMemoryCommand(payload.Memory))
		return createErr
	})
}

func (a *Agent) handleAIMemoryUpdate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.AIMemoryUpdatePayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory update failed: %w", err)
	}
	return a.mutateAIMemories(ctx, conn, message, "Memory updated.", func() error {
		input := payload.Memory
		_, updateErr := a.memoryService.Update(ctx, memorycommand.Update{
			MemoryID: payload.MemoryID, Title: input.Title, Kind: domainmemory.Kind(input.Kind),
			Scope: aiMemoryScopeDomain(input.Scope), Tags: input.Tags, Content: aiMemoryContentDomain(input.Content),
			Confidence: input.Confidence,
		})
		return updateErr
	})
}

func (a *Agent) handleAIMemoryDelete(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.AIMemoryDeletePayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory delete failed: %w", err)
	}
	return a.mutateAIMemories(ctx, conn, message, "Memory deleted.", func() error {
		return a.memoryService.Delete(ctx, memorycommand.Delete{MemoryID: payload.MemoryID, Reason: payload.Reason})
	})
}

func (a *Agent) handleAIMemoryRestore(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.AIMemoryRestorePayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory restore failed: %w", err)
	}
	return a.mutateAIMemories(ctx, conn, message, "Memory restored.", func() error {
		_, restoreErr := a.memoryService.Restore(ctx, memorycommand.Restore{MemoryID: payload.MemoryID})
		return restoreErr
	})
}

func (a *Agent) mutateAIMemories(ctx context.Context, conn *websocket.Conn, message protocol.Message, response string, mutate func() error) error {
	if err := a.requireMemoryService(); err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := mutate(); err != nil {
		return err
	}
	result, err := a.memoryService.List(ctx, memorycommand.List{Filter: memoryport.ListFilter{IncludeDeleted: true, Limit: 100}})
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeAIMemoryMutationResult, message.RequestID, message.SessionID, aiMemoriesPayload(result.Memories, response))
}

func (a *Agent) handleAIMemoryCandidateList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireMemoryService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.AIMemoryCandidateListPayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory candidate list failed: %w", err)
	}
	statuses := candidateStatuses(payload.Statuses)
	if len(statuses) == 0 {
		statuses = []domainmemory.CandidateStatus{domainmemory.CandidatePending}
	}
	result, err := a.memoryService.ListCandidates(ctx, memorycommand.ListCandidates{Filter: memoryport.CandidateFilter{Statuses: statuses, Limit: payload.Limit}})
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeAIMemoryCandidateListResult, message.RequestID, message.SessionID, aiMemoryCandidatesPayload(result.Items, ""))
}

func (a *Agent) handleAIMemoryCandidateApprove(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.AIMemoryCandidateApprovePayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory candidate approve failed: %w", err)
	}
	return a.mutateAIMemoryCandidates(ctx, conn, message, "Candidate approved.", func() error {
		_, approveErr := a.memoryService.ApproveCandidate(ctx, memorycommand.ApproveCandidate{
			CandidateID: payload.CandidateID, MemoryID: payload.MemoryID, HumanApprove: true,
		})
		return approveErr
	})
}

func (a *Agent) handleAIMemoryCandidateReject(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.AIMemoryCandidateRejectPayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory candidate reject failed: %w", err)
	}
	return a.mutateAIMemoryCandidates(ctx, conn, message, "Candidate rejected.", func() error {
		_, rejectErr := a.memoryService.RejectCandidate(ctx, memorycommand.RejectCandidate{CandidateID: payload.CandidateID, Note: payload.Note})
		return rejectErr
	})
}

func (a *Agent) handleAIMemoryExtractionJobList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireMemoryExtractionService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.AIMemoryExtractionJobListPayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory extraction job list failed: %w", err)
	}
	statuses := extractionJobStatuses(payload.Statuses)
	if len(statuses) == 0 {
		statuses = []domainmemory.JobStatus{domainmemory.JobFailed}
	}
	result, err := a.memoryExtraction.ListJobs(ctx, memoryextractioncommand.ListJobs{Statuses: statuses, Limit: payload.Limit})
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeAIMemoryExtractionJobListResult, message.RequestID, message.SessionID, aiMemoryExtractionJobsPayload(result.Items, ""))
}

func (a *Agent) handleAIMemoryExtractionJobRetry(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireMemoryExtractionService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.AIMemoryExtractionJobRetryPayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory extraction job retry failed: %w", err)
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if _, err := a.memoryExtraction.Retry(ctx, memoryextractioncommand.Retry{JobID: payload.JobID}); err != nil {
		return err
	}
	result, err := a.memoryExtraction.ListJobs(ctx, memoryextractioncommand.ListJobs{
		Statuses: []domainmemory.JobStatus{domainmemory.JobFailed}, Limit: 100,
	})
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeAIMemoryExtractionJobRetryResult, message.RequestID, message.SessionID, aiMemoryExtractionJobsPayload(result.Items, "Extraction retry scheduled."))
}

func (a *Agent) handleAIMemoryDreamList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireMemoryDreamService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.AIMemoryDreamListPayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory dream list failed: %w", err)
	}
	runs, err := a.memoryDream.List(ctx, memorydreamcommand.List{Limit: payload.Limit})
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeAIMemoryDreamListResult, message.RequestID, message.SessionID, protocol.AIMemoryDreamResultPayload{
		Runs: aiMemoryDreamRunsPayload(runs.Runs),
	})
}

func (a *Agent) handleAIMemoryDreamRun(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireMemoryDreamService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.AIMemoryDreamRunPayload](message)
	if err != nil {
		return fmt.Errorf("decode AI memory dream run failed: %w", err)
	}
	dreamResult, err := a.memoryDream.Run(ctx, memorydreamcommand.Run{
		Trigger: "manual", CandidateLimit: payload.CandidateLimit, MemoryLimit: payload.MemoryLimit,
	})
	if err != nil {
		runs, listErr := a.memoryDream.List(ctx, memorydreamcommand.List{Limit: 20})
		if listErr != nil {
			return errors.Join(err, listErr)
		}
		return a.writeRemoteMessage(conn, protocol.TypeAIMemoryDreamRunResult, message.RequestID, message.SessionID, protocol.AIMemoryDreamResultPayload{
			Runs: aiMemoryDreamRunsPayload(runs.Runs), Message: "Dream failed: " + err.Error(),
		})
	}
	runs, err := a.memoryDream.List(ctx, memorydreamcommand.List{Limit: 20})
	if err != nil {
		return err
	}
	memories, err := a.memoryService.List(ctx, memorycommand.List{Filter: memoryport.ListFilter{Limit: 100}})
	if err != nil {
		return err
	}
	candidates, err := a.memoryService.ListCandidates(ctx, memorycommand.ListCandidates{Filter: memoryport.CandidateFilter{
		Statuses: []domainmemory.CandidateStatus{domainmemory.CandidatePending}, Limit: 100,
	}})
	if err != nil {
		return err
	}
	messageText := fmt.Sprintf(
		"Dream completed: %d created, %d merged, %d rejected.",
		dreamResult.DreamRun.CreatedCount, dreamResult.DreamRun.MergedCount, dreamResult.DreamRun.RejectedCount,
	)
	return a.writeRemoteMessage(conn, protocol.TypeAIMemoryDreamRunResult, message.RequestID, message.SessionID, protocol.AIMemoryDreamResultPayload{
		Runs:       aiMemoryDreamRunsPayload(runs.Runs),
		Memories:   aiMemoriesPayload(memories.Memories, "").Memories,
		Candidates: aiMemoryCandidatesPayload(candidates.Items, "").Candidates,
		Message:    messageText,
	})
}

func (a *Agent) mutateAIMemoryCandidates(ctx context.Context, conn *websocket.Conn, message protocol.Message, response string, mutate func() error) error {
	if err := a.requireMemoryService(); err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := mutate(); err != nil {
		return err
	}
	memories, err := a.memoryService.List(ctx, memorycommand.List{Filter: memoryport.ListFilter{Limit: 100}})
	if err != nil {
		return err
	}
	candidates, err := a.memoryService.ListCandidates(ctx, memorycommand.ListCandidates{Filter: memoryport.CandidateFilter{
		Statuses: []domainmemory.CandidateStatus{domainmemory.CandidatePending}, Limit: 100,
	}})
	if err != nil {
		return err
	}
	memoryPayload := aiMemoriesPayload(memories.Memories, "")
	candidatePayload := aiMemoryCandidatesPayload(candidates.Items, "")
	return a.writeRemoteMessage(conn, protocol.TypeAIMemoryCandidateMutationResult, message.RequestID, message.SessionID, protocol.AIMemoryCandidateMutationResultPayload{
		Memories: memoryPayload.Memories, Candidates: candidatePayload.Candidates, Message: response,
	})
}

func createMemoryCommand(input protocol.AIMemoryInput) memorycommand.Create {
	return memorycommand.Create{
		Title: input.Title, Kind: domainmemory.Kind(input.Kind), Scope: aiMemoryScopeDomain(input.Scope),
		Tags: input.Tags, Content: aiMemoryContentDomain(input.Content), Confidence: input.Confidence,
	}
}

func aiMemoryScopeDomain(scope protocol.AIMemoryScope) domainmemory.Scope {
	return domainmemory.Scope{Type: domainmemory.ScopeType(scope.Type), Key: scope.Key}
}

func aiMemoryContentDomain(content protocol.AIMemoryContent) domainmemory.Content {
	return domainmemory.Content{
		Goal: content.Goal, ApplicableContext: content.ApplicableContext, Approach: content.Approach,
		Result: content.Result, PainPoints: content.PainPoints, RootCause: content.RootCause,
		Lessons: content.Lessons, Verification: content.Verification,
	}
}

func aiMemoryListFilter(payload protocol.AIMemoryListPayload) memoryport.ListFilter {
	filter := memoryport.ListFilter{
		Text: payload.Text, Tags: payload.Tags, ScopeKey: payload.ScopeKey,
		IncludeDeleted: payload.IncludeDeleted, Limit: payload.Limit,
	}
	for _, value := range payload.Kinds {
		filter.Kinds = append(filter.Kinds, domainmemory.Kind(value))
	}
	for _, value := range payload.Statuses {
		filter.Statuses = append(filter.Statuses, domainmemory.Status(value))
	}
	for _, value := range payload.ScopeTypes {
		filter.ScopeTypes = append(filter.ScopeTypes, domainmemory.ScopeType(value))
	}
	return filter
}

func candidateStatuses(values []string) []domainmemory.CandidateStatus {
	items := make([]domainmemory.CandidateStatus, 0, len(values))
	for _, value := range values {
		items = append(items, domainmemory.CandidateStatus(value))
	}
	return items
}

func extractionJobStatuses(values []string) []domainmemory.JobStatus {
	items := make([]domainmemory.JobStatus, 0, len(values))
	for _, value := range values {
		items = append(items, domainmemory.JobStatus(value))
	}
	return items
}
