package grpcprocessor

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "myai/core/adapter/documentprocessor/grpc/pb"
	domainknowledge "myai/core/domain/knowledge"
	documentprocessorport "myai/core/port/knowledge/documentprocessor"
)

type Processor struct {
	config Config
	pool   *workerPool
}

var _ documentprocessorport.DocumentProcessor = (*Processor)(nil)

func New(ctx context.Context, config Config) (*Processor, error) {
	config = config.Normalize()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	pool, err := newWorkerPool(ctx, config)
	if err != nil {
		return nil, err
	}
	return &Processor{config: config, pool: pool}, nil
}

func (processor *Processor) Process(
	ctx context.Context,
	request documentprocessorport.Request,
	sink documentprocessorport.ChunkSink,
) (domainknowledge.ProcessingSummary, error) {
	if processor == nil || processor.pool == nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor is not initialized")
	}
	document := request.Document
	parsingProfile := request.ParsingProfile
	chunkingProfile := request.ChunkingProfile
	if err := document.Validate(); err != nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("validate document: %w", err)
	}
	if strings.TrimSpace(document.ContentType) == "" {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("document content type is required")
	}
	if err := parsingProfile.Validate(); err != nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("validate parsing profile: %w", err)
	}
	if err := chunkingProfile.Validate(); err != nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("validate chunking profile: %w", err)
	}
	if chunkingProfile.MaxChunkSize > int(^uint32(0)>>1) || chunkingProfile.Overlap > int(^uint32(0)>>1) {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("chunking profile values exceed gRPC integer range")
	}
	if request.Content == nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("document content is required")
	}
	if sink == nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("document chunk sink is required")
	}

	requestContext, cancel := context.WithTimeout(ctx, processor.config.RequestTimeout)
	defer cancel()
	leasedWorker, release, err := processor.pool.acquire(requestContext)
	if err != nil {
		return domainknowledge.ProcessingSummary{}, err
	}
	reusable := false
	defer func() { release(reusable) }()

	summary, err := processor.process(requestContext, leasedWorker, document, parsingProfile, chunkingProfile, request.Content, sink)
	reusable = workerReusable(err)
	if err != nil {
		return domainknowledge.ProcessingSummary{}, err
	}
	return summary, nil
}

func (processor *Processor) process(
	ctx context.Context,
	worker *worker,
	document domainknowledge.Document,
	parsingProfile domainknowledge.ParsingProfile,
	chunkingProfile domainknowledge.ChunkingProfile,
	content io.Reader,
	sink documentprocessorport.ChunkSink,
) (domainknowledge.ProcessingSummary, error) {
	requestID := uuid.NewString()
	stream, err := worker.client.ProcessDocument(worker.context(ctx))
	if err != nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("open document processor stream: %w", err)
	}
	if err := stream.Send(&pb.ProcessRequest{Payload: &pb.ProcessRequest_Start{Start: &pb.ProcessStart{
		RequestId:       requestID,
		DocumentId:      document.ID,
		DocumentVersion: document.Version,
		ContentType:     document.ContentType,
		ParsingProfile:  parsingProfileMessage(parsingProfile),
		ChunkingProfile: chunkingProfileMessage(chunkingProfile),
	}}}); err != nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("send document processor metadata: %w", err)
	}

	buffer := make([]byte, processor.config.ContentChunkBytes)
	var totalBytes int64
	for {
		read, readErr := content.Read(buffer)
		if read > 0 {
			totalBytes += int64(read)
			if totalBytes > processor.config.MaxDocumentBytes {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("document exceeds configured size limit of %d bytes", processor.config.MaxDocumentBytes)
			}
			if err := stream.Send(&pb.ProcessRequest{Payload: &pb.ProcessRequest_Content{Content: buffer[:read]}}); err != nil {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("send document processor content: %w", err)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return domainknowledge.ProcessingSummary{}, fmt.Errorf("read document content: %w", readErr)
		}
	}
	if err := stream.Send(&pb.ProcessRequest{Payload: &pb.ProcessRequest_Completed{Completed: &pb.ProcessInputCompleted{}}}); err != nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("complete document processor input: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("close document processor input: %w", err)
	}

	var metadata *pb.ProcessMetadata
	var completedSummary *domainknowledge.ProcessingSummary
	receivedChunks := 0
	expectedOrdinal := 0
	for {
		event, receiveErr := stream.Recv()
		if receiveErr != nil {
			if receiveErr == io.EOF {
				if completedSummary == nil {
					return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor stream ended before completion")
				}
				return *completedSummary, nil
			}
			return domainknowledge.ProcessingSummary{}, fmt.Errorf("receive document processor event: %w", receiveErr)
		}
		switch payload := event.GetPayload().(type) {
		case *pb.ProcessEvent_Metadata:
			if metadata != nil || receivedChunks > 0 || completedSummary != nil {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor metadata is out of order")
			}
			if err := validateMetadata(payload.Metadata, requestID, document, parsingProfile, chunkingProfile); err != nil {
				return domainknowledge.ProcessingSummary{}, err
			}
			metadata = payload.Metadata
		case *pb.ProcessEvent_ChunkBatch:
			if metadata == nil || completedSummary != nil {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor chunk batch arrived before metadata")
			}
			batch := make([]domainknowledge.ChunkDraft, 0, len(payload.ChunkBatch.GetChunks()))
			for _, value := range payload.ChunkBatch.GetChunks() {
				draft, err := chunkDraftDomain(value)
				if err != nil {
					return domainknowledge.ProcessingSummary{}, fmt.Errorf("map document chunk: %w", err)
				}
				if draft.Ordinal != expectedOrdinal {
					return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor chunk ordinal %d, expected %d", draft.Ordinal, expectedOrdinal)
				}
				expectedOrdinal++
				batch = append(batch, draft)
			}
			if len(batch) == 0 {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor returned an empty chunk batch")
			}
			if err := sink.Accept(ctx, batch); err != nil {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("accept document chunk batch: %w", err)
			}
			receivedChunks += len(batch)
		case *pb.ProcessEvent_Completed:
			if metadata == nil || completedSummary != nil {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor completed before metadata")
			}
			if int(payload.Completed.GetChunkCount()) != receivedChunks {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor completed with %d chunks after sending %d", payload.Completed.GetChunkCount(), receivedChunks)
			}
			summary := domainknowledge.ProcessingSummary{
				DocumentID:              metadata.GetDocumentId(),
				DocumentVersion:         metadata.GetDocumentVersion(),
				ContentType:             metadata.GetContentType(),
				ParserID:                metadata.GetParserId(),
				ParserVersion:           metadata.GetParserVersion(),
				ChunkingStrategyID:      metadata.GetChunkingStrategyId(),
				ChunkingStrategyVersion: metadata.GetChunkingStrategyVersion(),
				ChunkCount:              receivedChunks,
				Warnings:                append([]string(nil), payload.Completed.GetWarnings()...),
			}
			if err := summary.Validate(); err != nil {
				return domainknowledge.ProcessingSummary{}, fmt.Errorf("validate document processor summary: %w", err)
			}
			completedSummary = &summary
		default:
			return domainknowledge.ProcessingSummary{}, fmt.Errorf("document processor returned an empty event")
		}
	}
}

func validateMetadata(metadata *pb.ProcessMetadata, requestID string, document domainknowledge.Document, parsingProfile domainknowledge.ParsingProfile, chunkingProfile domainknowledge.ChunkingProfile) error {
	if metadata.GetRequestId() != requestID || metadata.GetDocumentId() != document.ID || metadata.GetDocumentVersion() != document.Version {
		return fmt.Errorf("document processor response identity does not match request")
	}
	if strings.TrimSpace(metadata.GetContentType()) != strings.TrimSpace(document.ContentType) {
		return fmt.Errorf("document processor content type %q does not match %q", metadata.GetContentType(), document.ContentType)
	}
	if metadata.GetParserId() != parsingProfile.ParserID || metadata.GetParserVersion() != parsingProfile.ParserVersion {
		return fmt.Errorf("document processor used parser %q version %q, expected %q version %q", metadata.GetParserId(), metadata.GetParserVersion(), parsingProfile.ParserID, parsingProfile.ParserVersion)
	}
	if metadata.GetChunkingStrategyId() != chunkingProfile.StrategyID || metadata.GetChunkingStrategyVersion() != chunkingProfile.StrategyVersion {
		return fmt.Errorf("document processor used chunking strategy %q version %q, expected %q version %q", metadata.GetChunkingStrategyId(), metadata.GetChunkingStrategyVersion(), chunkingProfile.StrategyID, chunkingProfile.StrategyVersion)
	}
	return nil
}

func workerReusable(err error) bool {
	if err == nil {
		return true
	}
	switch status.Code(err) {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.ResourceExhausted, codes.Unimplemented:
		return true
	default:
		return false
	}
}

func (processor *Processor) Close() error {
	if processor == nil || processor.pool == nil {
		return nil
	}
	return processor.pool.Close()
}
