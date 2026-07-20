from collections.abc import Iterator
from typing import Protocol

from ..models import ChunkDraft, ChunkingProfile, ParsedDocument


class ChunkingStrategy(Protocol):
    strategy_id: str
    version: str

    def split(self, document: ParsedDocument, profile: ChunkingProfile) -> Iterator[ChunkDraft]: ...
