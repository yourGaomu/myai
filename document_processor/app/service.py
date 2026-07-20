from __future__ import annotations

from .errors import ProcessingError
from .models import ChunkingProfile, DocumentSource, ParsedDocument, ParsingProfile, ProcessedDocument
from .registry.chunking_registry import ChunkingStrategyRegistry
from .registry.parser_registry import ParserRegistry


class ProcessingService:
    def __init__(self, parser_registry: ParserRegistry | None = None, chunking_registry: ChunkingStrategyRegistry | None = None) -> None:
        self._parser_registry = parser_registry or ParserRegistry()
        self._chunking_registry = chunking_registry or ChunkingStrategyRegistry()

    def process(
        self,
        document_id: str,
        document_version: int,
        source: DocumentSource,
        parsing_profile: ParsingProfile,
        chunking_profile: ChunkingProfile,
    ) -> ProcessedDocument:
        if not document_id:
            raise ProcessingError("document id is required")
        if document_version < 1:
            raise ProcessingError("document version must be positive")
        if not source.content_type:
            raise ProcessingError("content type is required")
        if source.size < 0:
            raise ProcessingError("document size must not be negative")
        if not source.path.is_file():
            raise ProcessingError("document source file does not exist")
        parser = self._parser_registry.resolve(parsing_profile, source.content_type)
        parsed: ParsedDocument = parser.parse(source, parsing_profile)
        strategy = self._chunking_registry.resolve(chunking_profile)
        chunks = strategy.split(parsed, chunking_profile)
        warnings: list[str] = []
        if parsing_profile.ocr:
            warnings.append("OCR requested; parser-specific OCR integration is not enabled in this build")
        return ProcessedDocument(
            document_id=document_id,
            document_version=document_version,
            content_type=source.content_type,
            parser_id=parser.parser_id,
            parser_version=parser.version,
            chunking_strategy_id=strategy.strategy_id,
            chunking_strategy_version=strategy.version,
            chunks=chunks,
            warnings=warnings,
        )

    def parser_capabilities(self) -> list[tuple[str, str]]:
        return self._parser_registry.capabilities()

    def chunking_capabilities(self) -> list[tuple[str, str]]:
        return self._chunking_registry.capabilities()
