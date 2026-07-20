from __future__ import annotations

from collections.abc import Iterator
from dataclasses import dataclass

from ...errors import ProcessingError
from ...models import ChunkDraft, ChunkingProfile, ParsedDocument, Section


def _validate(profile: ChunkingProfile) -> None:
    if profile.max_chunk_size < 1:
        raise ProcessingError("max_chunk_size must be positive")
    if profile.overlap < 0 or profile.overlap >= profile.max_chunk_size:
        raise ProcessingError("overlap must be between 0 and max_chunk_size - 1")


def _split_window(text: str, base_offset: int, heading: str, page: int, profile: ChunkingProfile, start_ordinal: int) -> Iterator[ChunkDraft]:
    step = profile.max_chunk_size - profile.overlap
    start = 0
    ordinal = start_ordinal
    previous_start = 0
    start_byte_offset = 0
    while start < len(text):
        if start > previous_start:
            start_byte_offset += len(text[previous_start:start].encode("utf-8"))
        end = min(len(text), start + profile.max_chunk_size)
        value = text[start:end]
        if value.strip():
            yield ChunkDraft(
                ordinal=ordinal,
                text=value,
                start_offset=base_offset + start_byte_offset,
                end_offset=base_offset + start_byte_offset + len(value.encode("utf-8")),
                source_page=page,
                source_heading=heading,
            )
            ordinal += 1
        if end >= len(text):
            break
        previous_start = start
        start += step


@dataclass(frozen=True)
class WindowChunkingStrategy:
    strategy_id: str
    version: str = "1"

    def split(self, document: ParsedDocument, profile: ChunkingProfile) -> Iterator[ChunkDraft]:
        _validate(profile)
        section = document.sections[0] if document.sections else Section("", document.text, 0, len(document.text.encode("utf-8")), 0)
        return _split_window(document.text, 0, section.heading, section.page, profile, 0)


@dataclass(frozen=True)
class MarkdownChunkingStrategy:
    strategy_id: str = "markdown"
    version: str = "1"

    def split(self, document: ParsedDocument, profile: ChunkingProfile) -> Iterator[ChunkDraft]:
        _validate(profile)
        if not document.sections:
            return _split_window(document.text, 0, "", 0, profile, 0)

        def generate() -> Iterator[ChunkDraft]:
            ordinal = 0
            for section in document.sections:
                for chunk in _split_window(section.text, section.start_offset, section.heading, section.page, profile, ordinal):
                    yield chunk
                    ordinal = chunk.ordinal + 1

        return generate()
