from __future__ import annotations

from collections.abc import Iterator
from dataclasses import dataclass, field
from pathlib import Path


@dataclass(frozen=True)
class ParsingProfile:
    id: str
    name: str
    parser_id: str
    parser_version: str
    ocr: bool = False
    options: dict[str, str] = field(default_factory=dict)

@dataclass(frozen=True)
class ChunkingProfile:
    id: str
    name: str
    strategy_id: str
    strategy_version: str
    max_chunk_size: int
    overlap: int
    tokenizer: str = ""
    options: dict[str, str] = field(default_factory=dict)

@dataclass(frozen=True)
class DocumentSource:
    path: Path
    content_type: str
    size: int


@dataclass(frozen=True)
class Section:
    heading: str
    text: str
    start_offset: int
    end_offset: int
    page: int = 0


@dataclass(frozen=True)
class ParsedDocument:
    text: str
    sections: list[Section]


@dataclass(frozen=True)
class ChunkDraft:
    ordinal: int
    text: str
    start_offset: int
    end_offset: int
    source_page: int
    source_heading: str

@dataclass(frozen=True)
class ProcessedDocument:
    document_id: str
    document_version: int
    content_type: str
    parser_id: str
    parser_version: str
    chunking_strategy_id: str
    chunking_strategy_version: str
    chunks: Iterator[ChunkDraft]
    warnings: list[str]
