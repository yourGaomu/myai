from ..adapter.parser.implementations import DOCXParser, HTMLParserAdapter, PDFParser, TextParser
from ..errors import ProcessingError
from ..models import ParsingProfile
from ..port.parser import Parser


class ParserRegistry:
    def __init__(self, parsers: list[Parser] | None = None) -> None:
        self._parsers = parsers or [
            TextParser("python-text", "1", frozenset({"text/plain"})),
            TextParser("python-markdown", "1", frozenset({"text/markdown", "text/x-markdown", "application/markdown"}), markdown=True),
            HTMLParserAdapter(),
            PDFParser(),
            DOCXParser(),
        ]

    def resolve(self, profile: ParsingProfile, content_type: str) -> Parser:
        for parser in self._parsers:
            if parser.parser_id == profile.parser_id and parser.version == profile.parser_version and parser.supports(content_type):
                return parser
        raise ProcessingError(f"unsupported parser {profile.parser_id!r} version {profile.parser_version!r} for {content_type!r}")

    def capabilities(self) -> list[tuple[str, str]]:
        return [(parser.parser_id, parser.version) for parser in self._parsers]
