from typing import Protocol

from ..models import DocumentSource, ParsedDocument, ParsingProfile


class Parser(Protocol):
    parser_id: str
    version: str

    def supports(self, content_type: str) -> bool: ...

    def parse(self, source: DocumentSource, profile: ParsingProfile) -> ParsedDocument: ...
