from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import tempfile
import threading
from concurrent import futures
from pathlib import Path
from typing import Iterator

import grpc

from ..errors import ProcessingError
from ..models import ChunkDraft, ChunkingProfile, DocumentSource, ParsingProfile
from ..service import ProcessingService

_GENERATED_ROOT = Path(__file__).resolve().parents[1] / "generated"
if str(_GENERATED_ROOT) not in sys.path:
    sys.path.insert(0, str(_GENERATED_ROOT))

from documentprocessor.v1 import document_processor_pb2 as pb  # noqa: E402
from documentprocessor.v1 import document_processor_pb2_grpc as pb_grpc  # noqa: E402


PROTOCOL_VERSION = "v1"
TOKEN_HEADER = "x-myai-worker-token"


class DocumentProcessorServicer(pb_grpc.DocumentProcessorServiceServicer):
    def __init__(
        self,
        service: ProcessingService,
        worker_id: str,
        token: str,
        shutdown_event: threading.Event,
        max_input_bytes: int,
        chunk_batch_size: int,
        max_chunk_batch_bytes: int,
        temp_dir: str,
    ) -> None:
        self._service = service
        self._worker_id = worker_id
        self._token = token
        self._shutdown_event = shutdown_event
        self._max_input_bytes = max_input_bytes
        self._chunk_batch_size = chunk_batch_size
        self._max_chunk_batch_bytes = max_chunk_batch_bytes
        self._temp_dir = temp_dir or None
        self._process_lock = threading.Lock()

    def ProcessDocument(self, request_iterator: Iterator[pb.ProcessRequest], context: grpc.ServicerContext) -> Iterator[pb.ProcessEvent]:
        self._authorize(context)
        if not self._process_lock.acquire(blocking=False):
            context.abort(grpc.StatusCode.RESOURCE_EXHAUSTED, "worker is already processing a document")

        temp_path: Path | None = None
        try:
            start: pb.ProcessStart | None = None
            input_completed = False
            total_bytes = 0
            with tempfile.NamedTemporaryFile(mode="wb", delete=False, dir=self._temp_dir, prefix="myai-document-", suffix=".bin") as source_file:
                temp_path = Path(source_file.name)
                for request in request_iterator:
                    payload = request.WhichOneof("payload")
                    if payload == "start":
                        if start is not None or total_bytes > 0:
                            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "process start must be the first and only start message")
                        start = request.start
                    elif payload == "content":
                        if start is None or input_completed:
                            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "document content is out of order")
                        total_bytes += len(request.content)
                        if total_bytes > self._max_input_bytes:
                            context.abort(grpc.StatusCode.RESOURCE_EXHAUSTED, "document exceeds configured size limit")
                        source_file.write(request.content)
                    elif payload == "completed":
                        if start is None or input_completed:
                            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "document completion is out of order")
                        input_completed = True
                    else:
                        context.abort(grpc.StatusCode.INVALID_ARGUMENT, "process request payload is required")

            if start is None or not input_completed:
                context.abort(grpc.StatusCode.INVALID_ARGUMENT, "incomplete document processor request")

            result = self._service.process(
                start.document_id,
                start.document_version,
                DocumentSource(temp_path, start.content_type, total_bytes),
                _parsing_profile(start.parsing_profile),
                _chunking_profile(start.chunking_profile),
            )
            yield pb.ProcessEvent(
                metadata=pb.ProcessMetadata(
                    request_id=start.request_id,
                    document_id=result.document_id,
                    document_version=result.document_version,
                    content_type=result.content_type,
                    parser_id=result.parser_id,
                    parser_version=result.parser_version,
                    chunking_strategy_id=result.chunking_strategy_id,
                    chunking_strategy_version=result.chunking_strategy_version,
                )
            )

            batch: list[pb.ChunkDraft] = []
            batch_bytes = 0
            chunk_count = 0
            for chunk in result.chunks:
                if not context.is_active():
                    return
                chunk_message = _chunk_draft(chunk)
                chunk_bytes = len(chunk.text.encode("utf-8")) + len(chunk.source_heading.encode("utf-8")) + 128
                if chunk_bytes > self._max_chunk_batch_bytes:
                    context.abort(grpc.StatusCode.RESOURCE_EXHAUSTED, "a generated chunk exceeds the gRPC message limit")
                if batch and (len(batch) >= self._chunk_batch_size or batch_bytes + chunk_bytes > self._max_chunk_batch_bytes):
                    yield pb.ProcessEvent(chunk_batch=pb.ChunkBatch(chunks=batch))
                    batch = []
                    batch_bytes = 0
                batch.append(chunk_message)
                batch_bytes += chunk_bytes
                chunk_count += 1
            if batch:
                yield pb.ProcessEvent(chunk_batch=pb.ChunkBatch(chunks=batch))
            if chunk_count == 0:
                context.abort(grpc.StatusCode.FAILED_PRECONDITION, "document produced no chunks")
            yield pb.ProcessEvent(completed=pb.ProcessCompleted(chunk_count=chunk_count, warnings=result.warnings))
        except ProcessingError as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        except grpc.RpcError:
            raise
        except Exception as exc:
            logging.exception("document processing failed")
            context.abort(grpc.StatusCode.INTERNAL, str(exc))
        finally:
            if temp_path is not None:
                temp_path.unlink(missing_ok=True)
            self._process_lock.release()

    def Health(self, request: pb.HealthRequest, context: grpc.ServicerContext) -> pb.HealthResponse:
        self._authorize(context)
        return pb.HealthResponse(status="ok", protocol_version=PROTOCOL_VERSION, worker_id=self._worker_id)

    def GetCapabilities(self, request: pb.CapabilitiesRequest, context: grpc.ServicerContext) -> pb.CapabilitiesResponse:
        self._authorize(context)
        return pb.CapabilitiesResponse(
            protocol_version=PROTOCOL_VERSION,
            parsers=[pb.Capability(id=value[0], version=value[1]) for value in self._service.parser_capabilities()],
            chunking_strategies=[pb.Capability(id=value[0], version=value[1]) for value in self._service.chunking_capabilities()],
        )

    def Shutdown(self, request: pb.ShutdownRequest, context: grpc.ServicerContext) -> pb.ShutdownResponse:
        self._authorize(context)
        self._shutdown_event.set()
        return pb.ShutdownResponse()

    def _authorize(self, context: grpc.ServicerContext) -> None:
        metadata = dict(context.invocation_metadata())
        if not self._token or metadata.get(TOKEN_HEADER) != self._token:
            context.abort(grpc.StatusCode.UNAUTHENTICATED, "invalid worker token")


def _parsing_profile(value: pb.ParsingProfile) -> ParsingProfile:
    return ParsingProfile(
        id=value.id,
        name=value.name,
        parser_id=value.parser_id,
        parser_version=value.parser_version,
        ocr=value.ocr,
        options=dict(value.options),
    )


def _chunking_profile(value: pb.ChunkingProfile) -> ChunkingProfile:
    return ChunkingProfile(
        id=value.id,
        name=value.name,
        strategy_id=value.strategy_id,
        strategy_version=value.strategy_version,
        max_chunk_size=value.max_chunk_size,
        overlap=value.overlap,
        tokenizer=value.tokenizer,
        options=dict(value.options),
    )


def _chunk_draft(value: ChunkDraft) -> pb.ChunkDraft:
    return pb.ChunkDraft(
        ordinal=value.ordinal,
        text=value.text,
        start_offset=value.start_offset,
        end_offset=value.end_offset,
        source_page=value.source_page,
        source_heading=value.source_heading,
    )


def serve(args: argparse.Namespace) -> None:
    if args.max_input_bytes < 1 or args.grpc_max_message_bytes < 128 * 1024 or args.chunk_batch_size < 1:
        raise ValueError("worker size limits must be positive and gRPC messages must allow protocol overhead")
    if args.temp_dir:
        Path(args.temp_dir).mkdir(parents=True, exist_ok=True)
    shutdown_event = threading.Event()
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=4, thread_name_prefix=f"document-processor-{args.worker_id}"),
        maximum_concurrent_rpcs=4,
        options=(
            ("grpc.max_receive_message_length", args.grpc_max_message_bytes),
            ("grpc.max_send_message_length", args.grpc_max_message_bytes),
        ),
    )
    token = os.environ.get("MYAI_DOCUMENT_PROCESSOR_TOKEN", "")
    pb_grpc.add_DocumentProcessorServiceServicer_to_server(
        DocumentProcessorServicer(
            ProcessingService(),
            args.worker_id,
            token,
            shutdown_event,
            args.max_input_bytes,
            args.chunk_batch_size,
            max(1024, args.grpc_max_message_bytes - 64 * 1024),
            args.temp_dir,
        ),
        server,
    )

    bound_port = server.add_insecure_port(args.address)
    if bound_port == 0:
        raise RuntimeError(f"failed to bind document processor worker to {args.address}")
    endpoint = args.address
    if args.address.endswith(":0"):
        endpoint = f"{args.address[:-2]}:{bound_port}"

    server.start()
    print(json.dumps({"event": "ready", "protocol": PROTOCOL_VERSION, "worker_id": args.worker_id, "endpoint": endpoint}), flush=True)
    shutdown_event.wait()
    server.stop(args.shutdown_grace_seconds).wait()


def configure_parser(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--worker-id", required=True)
    parser.add_argument("--address", default="127.0.0.1:0")
    parser.add_argument("--max-input-bytes", type=int, default=512 * 1024 * 1024)
    parser.add_argument("--grpc-max-message-bytes", type=int, default=8 * 1024 * 1024)
    parser.add_argument("--chunk-batch-size", type=int, default=64)
    parser.add_argument("--temp-dir", default="")
    parser.add_argument("--shutdown-grace-seconds", type=float, default=5.0)
