import unittest
from pathlib import Path
from tempfile import TemporaryDirectory

from app.errors import ProcessingError
from app.models import ChunkingProfile, DocumentSource, ParsingProfile
from app.service import ProcessingService


class ProcessingServiceTest(unittest.TestCase):
    def test_markdown_is_parsed_and_chunked(self) -> None:
        with self._source("# Title\n\nThis is document content.", "text/markdown") as source:
            result = ProcessingService().process(
                "document-1",
                1,
                source,
                ParsingProfile("parsing-1", "Markdown", "python-markdown", "1"),
                ChunkingProfile("chunking-1", "Markdown", "markdown", "1", 16, 2),
            )
            chunks = list(result.chunks)
            self.assertEqual(result.parser_id, "python-markdown")
            self.assertEqual(result.chunking_strategy_id, "markdown")
            self.assertGreater(len(chunks), 1)
            self.assertEqual(chunks[0].source_heading, "Title")

    def test_unsupported_parser_is_rejected(self) -> None:
        with self._source("content", "text/markdown") as source:
            with self.assertRaises(ProcessingError):
                ProcessingService().process(
                    "document-1",
                    1,
                    source,
                    ParsingProfile("parsing-1", "Unknown", "unknown", "1"),
                    ChunkingProfile("chunking-1", "Text", "text", "1", 100, 0),
                )

    def test_offsets_are_utf8_byte_offsets(self) -> None:
        with self._source("你好世界", "text/plain") as source:
            result = ProcessingService().process(
                "document-1",
                1,
                source,
                ParsingProfile("parsing-1", "Text", "python-text", "1"),
                ChunkingProfile("chunking-1", "Text", "text", "1", 2, 0),
            )
            chunks = list(result.chunks)
            self.assertEqual([(chunk.start_offset, chunk.end_offset) for chunk in chunks], [(0, 6), (6, 12)])

    def _source(self, content: str, content_type: str):
        return _DocumentSourceContext(content, content_type)


class _DocumentSourceContext:
    def __init__(self, content: str, content_type: str) -> None:
        self._content = content
        self._content_type = content_type
        self._directory: TemporaryDirectory[str] | None = None

    def __enter__(self) -> DocumentSource:
        self._directory = TemporaryDirectory()
        path = Path(self._directory.name) / "source.bin"
        payload = self._content.encode("utf-8")
        path.write_bytes(payload)
        return DocumentSource(path, self._content_type, len(payload))

    def __exit__(self, exc_type, exc_value, traceback) -> None:
        if self._directory is not None:
            self._directory.cleanup()


if __name__ == "__main__":
    unittest.main()
