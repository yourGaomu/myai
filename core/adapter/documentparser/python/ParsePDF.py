import json
import sys


def truncate(text, max_chars):
    if len(text) <= max_chars:
        return text, False
    return text[:max_chars], True


def parse_pdf(file_path):
    try:
        from pypdf import PdfReader
    except Exception as exc:
        return {
            "supported": False,
            "kind": "pdf",
            "text": "",
            "truncated": False,
            "message": "Python package pypdf is not installed: " + str(exc),
            "metadata": {},
        }

    reader = PdfReader(file_path)
    parts = []
    for index, page in enumerate(reader.pages):
        text = page.extract_text() or ""
        if text.strip():
            parts.append("[Page {}]\n{}".format(index + 1, text))

    return {
        "supported": True,
        "kind": "pdf",
        "text": "\n\n".join(parts),
        "truncated": False,
        "message": "",
        "metadata": {
            "pages": str(len(reader.pages)),
        },
    }


def main():
    request = json.load(sys.stdin)
    file_type = request.get("file_type", "")
    max_chars = int(request.get("max_chars") or 12000)

    if file_type == "pdf":
        result = parse_pdf(request["file_path"])
    else:
        result = {
            "supported": False,
            "kind": file_type or "unknown",
            "text": "",
            "truncated": False,
            "message": "unsupported file type",
            "metadata": {},
        }

    text, truncated = truncate(result.get("text", ""), max_chars)
    result["text"] = text
    result["truncated"] = bool(result.get("truncated")) or truncated
    result.setdefault("metadata", {})

    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
