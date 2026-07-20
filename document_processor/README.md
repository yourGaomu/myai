# MyAI Document Processor

The document processor is a pool of Python gRPC workers managed by the Go application. It parses and chunks documents, but never accesses MongoDB, MinIO, Milvus, sessions, or indexing job state.

## Install

```powershell
python -m pip install -r requirements.txt
```

`grpcio` and `protobuf` are runtime dependencies. `PyMuPDF` and `python-docx` provide PDF and DOCX support.

## Managed Worker

Workers are normally started and stopped by Go. A standalone worker can be started for diagnostics:

```powershell
$env:MYAI_DOCUMENT_PROCESSOR_TOKEN = "diagnostic-token"
python main.py worker --worker-id diagnostic --address 127.0.0.1:0
```

The first stdout line is a JSON readiness message. Runtime diagnostics are written to stderr.

## Protocol

The source of truth is `api/documentprocessor/v1/document_processor.proto`.

`ProcessDocument` is a bidirectional gRPC stream:

```text
Go -> Start metadata
Go -> Content chunks
Go -> Input completed
Python -> Processing metadata
Python -> ChunkBatch(s)
Python -> Completed
```

Document content is streamed in bounded binary messages. Python writes each request to a private temporary file so PDF and DOCX parsers can use seekable input without keeping a second raw byte copy in memory. Chunking strategies return iterators and results are sent in batches.

Offsets are UTF-8 byte offsets. Go recalculates every Chunk content hash, creates stable IDs, persists chunks, and owns retries and job state.

## Structure

```text
app/
├── port/                  # Parser and ChunkingStrategy interfaces
├── adapter/               # Parser and chunker implementations
├── registry/              # Implementation selection
├── grpc/                  # gRPC transport adapter
├── generated/             # Generated Protobuf DTOs
├── models.py
└── service.py             # Parse + Chunk application service
```

## Generate DTOs

Go:

```powershell
protoc --proto_path=. --go_out=. --go_opt=module=myai --go-grpc_out=. --go-grpc_opt=module=myai api/documentprocessor/v1/document_processor.proto
```

Python:

```powershell
python -m grpc_tools.protoc -I api --python_out=document_processor/app/generated --grpc_python_out=document_processor/app/generated api/documentprocessor/v1/document_processor.proto
```

## Test

```powershell
python -m unittest discover -s tests -v
$env:MYAI_RUN_PYTHON_PROCESSOR_TEST = "1"
go test ./core/adapter/documentprocessor/grpc -v
```
