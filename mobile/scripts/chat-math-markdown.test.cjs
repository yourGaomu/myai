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

const {
  collectMarkdownAssetImages,
  markdownInlineMatches,
  markdownInlineMath,
  parseMarkdownBlocks,
} = require("../src/utils/chatMarkdown.ts");

function matches(text) {
  return [...markdownInlineMatches(text)].map((match) => ({ index: match.index, token: match[0] }));
}

function inlineMath(text) {
  return matches(text).flatMap(({ token }) => {
    const math = markdownInlineMath(token);
    return math ? [math] : [];
  });
}

const paragraph = (text) => ({ type: "paragraph", text });
const mathBlock = (text, source) => ({ type: "math", text, source });

test("inline math preserves token offsets and supports CJK adjacency", () => {
  const dollar = String.raw`$\frac{a_i}{\sqrt{\beta}}$`;
  const paren = String.raw`\( \sum_{j=1}^{n} x_j^2 \)`;
  const text = `🙂计算${dollar}和${paren}即可。`;
  assert.deepEqual(matches(text), [
    { index: text.indexOf(dollar), token: dollar },
    { index: text.indexOf(paren), token: paren },
  ]);
  assert.deepEqual(inlineMath(text), [{ text: dollar.slice(1, -1) }, { text: paren.slice(2, -2).trim() }]);
  assert.deepEqual(inlineMath("$-3$、$2$、$3+x$。"), [{ text: "-3" }, { text: "2" }, { text: "3+x" }]);
});

test("single-line and multiline standalone display formulas split neighboring paragraphs", () => {
  const one = String.raw`  $$\sqrt{2}$$ `;
  const bracket = String.raw`\[\frac{1}{2}\]`;
  const multiline = ["$$", String.raw`\begin{bmatrix}`, String.raw`  1 & 2 \\`, "  3 & 4", String.raw`\end{bmatrix}`, "$$"];
  const bracketLines = [String.raw`\[`, String.raw`\sum_{j=1}^{n} j`, String.raw`\]`];
  const text = ["Before", one, bracket, ...multiline, ...bracketLines, "After"].join("\r\n");
  assert.deepEqual(parseMarkdownBlocks(text), [
    paragraph("Before"),
    mathBlock(String.raw`\sqrt{2}`, one),
    mathBlock(String.raw`\frac{1}{2}`, bracket),
    mathBlock(multiline.slice(1, -1).join("\n"), multiline.join("\r\n")),
    mathBlock(bracketLines[1], bracketLines.join("\r\n")),
    paragraph("After"),
  ]);
});

test("display delimiters can share lines with formula content and preserve escaped markers", () => {
  const dollar = String.raw`$$\text{\$}$$`;
  const bracket = String.raw`\[a \\] + b
  + c\]  `;
  assert.deepEqual(parseMarkdownBlocks(`${dollar}\n${bracket}`), [
    mathBlock(String.raw`\text{\$}`, dollar),
    mathBlock(String.raw`a \\] + b
  + c`, bracket),
  ]);
  const mixedLines = "$$x +\ny$$";
  assert.deepEqual(parseMarkdownBlocks(mixedLines), [mathBlock("x +\ny", mixedLines)]);
});

test("currency, escaped delimiters, empty spans, and non-standalone displays stay literal", () => {
  const literal = [
    "Costs $5, $10.50 or $20.",
    "Prices $5-$10 and -$2.50 vs. -$4.",
    "Payment is US$5 and then US$8.",
    "$ x$ or $x $ or $x$2",
    String.raw`\$x\$ or \\(x\\) or \( \)`,
    String.raw`\$$5 and \$$10`,
    "Words $$x^2$$ after",
    String.raw`Words \[x^2\] after`,
    String.raw`\\[x^2\]`,
    "$$$$ or $$$x$$$",
    "$$x$$ after",
    String.raw`\[x\] after`,
    "$$  $$",
    String.raw`\[ \]`,
  ];
  for (const text of literal) {
    assert.deepEqual(inlineMath(text), [], text);
    assert.deepEqual(parseMarkdownBlocks(text), [paragraph(text)], text);
  }
  assert.deepEqual(inlineMath(String.raw`A fee of $5, then $x + \$y$ and \\(plain\\).`), [{ text: String.raw`x + \$y` }]);
  assert.deepEqual(inlineMath(String.raw`\\$a$ and \\\(b\)`), [{ text: "a" }, { text: "b" }]);
  assert.deepEqual(inlineMath(String.raw`$a\$$`), [{ text: String.raw`a\$` }]);
  for (const token of ["$$x$$", "$x\n+y$", "$x$ after", String.raw`\(x\) after`, "$", "bold"]) {
    assert.equal(markdownInlineMath(token), undefined, token);
  }
});

test("partial streamed delimiters fall back to paragraphs and activate when closed", () => {
  const dollar = String.raw`$\frac{x}{y}`;
  const paren = String.raw`\(\beta`;
  assert.deepEqual(matches(`${dollar}\n${paren}`), []);
  assert.deepEqual(matches(`${dollar}\n$`), []);
  assert.deepEqual(matches(`${paren}\n\\)`), []);
  assert.deepEqual(inlineMath(`${dollar}$ and ${paren}\\)`), [{ text: dollar.slice(1) }, { text: paren.slice(2) }]);

  for (const [opener, closer] of [["$$", "$$"], ["\\[", "\\]"]]) {
    const body = String.raw`\sqrt{x}`;
    const pending = `${opener}\n${body}`;
    assert.deepEqual(parseMarkdownBlocks(`Before\n${pending}`), [paragraph(`Before\n${pending}`)]);
    const complete = `${pending}\n${closer}`;
    assert.deepEqual(parseMarkdownBlocks(`Before\n${complete}\nAfter`), [paragraph("Before"), mathBlock(body, complete), paragraph("After")]);
  }
  assert.deepEqual(parseMarkdownBlocks("$$\n```txt\n$$\n```"), [
    paragraph("$$"), { type: "code", language: "txt", text: "$$" },
  ]);
});

test("fenced, indented, and inline code and link destinations keep literal delimiters", () => {
  const fenced = ["```latex", "$$x$$", String.raw`\[y\]`, "```"].join("\n");
  const indented = "    $$x$$\n\t\\[y\\]";
  assert.deepEqual(parseMarkdownBlocks(`${fenced}\n\n${indented}`), [
    { type: "code", language: "latex", text: "$$x$$\n\\[y\\]" },
    { type: "code", language: "", text: "$$x$$\n\\[y\\]" },
  ]);
  const tokens = ["`$x$`", "``a ` $y$``", "[$label$](https://example.test/$path$)", "https://example.test/$url$", "$z$"];
  assert.deepEqual(matches(tokens.join(" ")).map(({ token }) => token), tokens);
  assert.deepEqual(inlineMath(tokens.join(" ")), [{ text: "z" }]);
  assert.deepEqual(matches("**https://example.test/$path$**").map(({ token }) => token), ["**https://example.test/$path$**"]);
});

test("emphasis can wrap formulas without interpreting TeX asterisks as Markdown", () => {
  const tokens = ["**value $a*b$**", "$a *b* c$", "*next*", "**`a*b`**"];
  assert.deepEqual(matches(tokens.join(" ")).map(({ token }) => token), tokens);
  assert.deepEqual(inlineMath(tokens[0].slice(2, -2)), [{ text: "a*b" }]);
});

test("formulas remain available in headings, lists, and recursive quotes", () => {
  const blocks = parseMarkdownBlocks("# Rate $r$\n- Item \\(x\\)\n> Quoted $y$\n> \\[z\\]");
  assert.deepEqual(blocks, [
    { type: "heading", level: 1, text: "Rate $r$" },
    { type: "list", ordered: false, items: [String.raw`Item \(x\)`] },
    { type: "quote", lines: ["Quoted $y$", String.raw`\[z\]`] },
  ]);
  assert.deepEqual(inlineMath(blocks[0].text), [{ text: "r" }]);
  assert.deepEqual(inlineMath(blocks[1].items[0]), [{ text: "x" }]);
  const quoted = parseMarkdownBlocks(blocks[2].lines.join("\n"));
  assert.deepEqual(inlineMath(quoted[0].text), [{ text: "y" }]);
  assert.deepEqual(quoted[1], mathBlock("z", blocks[2].lines[1]));
});

test("table column boundaries ignore formula, code and escaped pipes", () => {
  const header = ["$|x|$", "Norm"];
  const row = [String.raw`\(\left| x \right|\)`, "`a|b`", String.raw`c\|d`];
  const text = [
    `| ${header.join(" | ")} |`,
    "| --- | :---: |",
    `| ${row.join(" | ")} |`,
    "| $unclosed | next |",
    "$$|z|$$",
  ].join("\n");
  assert.deepEqual(parseMarkdownBlocks(text), [
    { type: "table", header, rows: [row, ["$unclosed", "next"]] },
    mathBlock("|z|", "$$|z|$$"),
  ]);
  assert.deepEqual(inlineMath(row[0]), [{ text: String.raw`\left| x \right|` }]);
  const noTable = "$|x|$\n--- | ---";
  assert.deepEqual(parseMarkdownBlocks(noTable), [paragraph(noTable)]);
});

test("asset extraction skips formulas and code but finds neighboring and nested images", () => {
  const text = [
    "$![math](asset:ast_inline)$ **$a*b$ ![normal](asset:ast_normal)**",
    String.raw`\(![math](asset:ast_bracket_inline)\)`,
    "$$",
    "![math](asset:ast_display)",
    "$$",
    "> \\[![math](asset:ast_quoted_display)\\]",
    "> ![quote](asset:ast_quote)",
    "",
    "| Formula | Image |",
    "| --- | --- |",
    "| $|x|$ | ![cell](asset:ast_cell) |",
    "",
    "`![code](asset:ast_code)`",
    "$pending ![still visible](asset:ast_pending)",
  ].join("\n");
  assert.deepEqual([...collectMarkdownAssetImages(parseMarkdownBlocks(text))], [
    ["ast_normal", "normal"], ["ast_quote", "quote"], ["ast_cell", "cell"], ["ast_pending", "still visible"],
  ]);
});
