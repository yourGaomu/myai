from ..adapter.chunking.implementations import MarkdownChunkingStrategy, WindowChunkingStrategy
from ..errors import ProcessingError
from ..models import ChunkingProfile
from ..port.chunking import ChunkingStrategy


class ChunkingStrategyRegistry:
    def __init__(self, strategies: list[ChunkingStrategy] | None = None) -> None:
        self._strategies = strategies or [
            MarkdownChunkingStrategy(),
            WindowChunkingStrategy("character"),
            WindowChunkingStrategy("text"),
            WindowChunkingStrategy("html"),
            WindowChunkingStrategy("code"),
            WindowChunkingStrategy("token"),
        ]

    def resolve(self, profile: ChunkingProfile) -> ChunkingStrategy:
        for strategy in self._strategies:
            if strategy.strategy_id == profile.strategy_id and strategy.version == profile.strategy_version:
                return strategy
        raise ProcessingError(f"unsupported chunking strategy {profile.strategy_id!r} version {profile.strategy_version!r}")

    def capabilities(self) -> list[tuple[str, str]]:
        return [(strategy.strategy_id, strategy.version) for strategy in self._strategies]
