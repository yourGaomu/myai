package documentparser

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTextPreservesUnicodeAndCharacterLimit(t *testing.T) {
	result := Parse(context.Background(), Request{
		FileName: "note.md", ContentType: "Text/Markdown; charset=utf-8", Data: []byte("你好世界"), MaxChars: 2,
	})
	if !result.Supported || result.Kind != "text" || result.Text != "你好" || !result.Truncated || result.ContentType != "text/markdown" || result.Size != 12 {
		t.Fatalf("text result = %#v", result)
	}
	full := Parse(context.Background(), Request{FileName: "empty.txt", ContentType: "text/plain"})
	if !full.Supported || full.Text != "" || full.Truncated {
		t.Fatalf("empty text result = %#v", full)
	}
}

func TestParseUnsupportedBinaryKeepsMetadata(t *testing.T) {
	result := Parse(context.Background(), Request{FileName: "image.png", ContentType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47, 0}})
	if result.Supported || result.Kind != "image" || result.Message == "" || result.Metadata["head_hex"] != "89504e4700" {
		t.Fatalf("binary result = %#v", result)
	}
}

func TestParsePDFFallsBackWhenPythonIsUnavailable(t *testing.T) {
	t.Setenv("MYAI_PYTHON_BIN", filepath.Join(t.TempDir(), "missing-python"))
	result := Parse(context.Background(), Request{FileName: "guide.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")})
	if result.Supported || result.Kind != "pdf" || result.Message == "" || result.Size != 8 || result.Metadata["extension"] != ".pdf" {
		t.Fatalf("PDF fallback = %#v", result)
	}
}

func TestParsePDFWithPythonBackend(t *testing.T) {
	binary, err := exec.LookPath(pythonBinary())
	if err != nil {
		t.Skipf("Python backend unavailable: %v", err)
	}
	if err := exec.Command(binary, "-I", "-c", "import pypdf").Run(); err != nil {
		t.Skipf("Python pypdf backend unavailable: %v", err)
	}
	data := onePagePDF("Hello PDF")
	result := Parse(context.Background(), Request{FileName: "guide.pdf", ContentType: "application/pdf", Data: data, MaxChars: 100})
	if !result.Supported || result.Kind != "pdf" || !strings.Contains(result.Text, "Hello PDF") || result.Metadata["pages"] != "1" || result.Truncated {
		t.Fatalf("PDF result = %#v", result)
	}
	limited := Parse(context.Background(), Request{FileName: "guide.pdf", ContentType: "application/pdf", Data: data, MaxChars: 5})
	if !limited.Supported || !limited.Truncated || len([]rune(limited.Text)) != 5 {
		t.Fatalf("limited PDF result = %#v", limited)
	}
}

func onePagePDF(text string) []byte {
	content := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	}
	var document bytes.Buffer
	document.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for index, object := range objects {
		offsets[index] = document.Len()
		fmt.Fprintf(&document, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := document.Len()
	fmt.Fprintf(&document, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&document, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&document, "trailer\n<< /Root 1 0 R /Size %d >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return document.Bytes()
}
