const assert = require("node:assert/strict");
const fs = require("node:fs");
const { test } = require("node:test");
const ts = require("typescript");

require.extensions[".ts"] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    fileName: filename,
  });
  module._compile(outputText, filename);
};

const { renderMathDrawing } = require("../src/utils/mathDrawing.ts");

test("typesets fractions, roots, Greek letters, subscripts, sums and matrices as self-contained SVG", () => {
  for (const formula of [String.raw`\frac{\alpha_1}{\sqrt{2}}`, String.raw`\sum_{i=1}^n i^2`, String.raw`\begin{pmatrix}a&b\\c&d\end{pmatrix}`]) {
    const drawing = renderMathDrawing(formula, true);
    assert.ok(drawing, formula);
    assert.match(drawing.xml, /^<svg[^>]+viewBox=/);
    assert.match(drawing.xml, /<path /);
    assert.doesNotMatch(drawing.xml, /<(use|image|script)\b|\bhref=/i);
    assert.ok(drawing.widthEm > 0 && drawing.heightEm > 0);
    assert.ok(drawing.depthEm >= 0);
  }
});

test("keeps inline and display layouts distinct and falls back for invalid or oversized source", () => {
  const sum = String.raw`\sum_{i=0}^n x_i`;
  const inline = renderMathDrawing(sum, false);
  const display = renderMathDrawing(sum, true);
  assert.ok(inline && display);
  assert.notEqual(inline.xml, display.xml);
  assert.equal(renderMathDrawing(sum, false), inline);
  assert.equal(renderMathDrawing(String.raw`\frac{1}{`, false), null);
  assert.equal(renderMathDrawing(String.raw`\href{https://example.com}{x}`, false), null);
  assert.equal(renderMathDrawing(String.raw`\eqref{foo}`, false), null);
  assert.equal(renderMathDrawing("x".repeat(2049), false), null);
  assert.ok(renderMathDrawing("x+1", false));
});
