import { isAssetID } from "./assetImagePreview";
import type { AssetSummary } from "../protocol";
import { assetPublicURL, assetSummaryToSharedAsset, mergeSharedAssets, type SharedAsset } from "./toolAssets";

export type MarkdownBlock =
  | { type: "heading"; level: 1 | 2 | 3; text: string }
  | { type: "paragraph"; text: string }
  | { type: "list"; ordered: boolean; items: string[] }
  | { type: "code"; language: string; text: string }
  | { type: "math"; text: string; source: string }
  | { type: "table"; header: string[]; rows: string[][] }
  | { type: "quote"; lines: string[] };

export type MarkdownImage = { alt: string } & (
  | { kind: "asset"; assetID: string }
  | { kind: "external"; url: string }
  | { kind: "unsupported" }
);

type MarkdownInlineMatch = [string] & { index: number };

export function* markdownInlineMatches(text: string): IterableIterator<MarkdownInlineMatch> {
  const protectedMatches = protectedInlineMatches(text);
  // Emphasis still wraps these tokens, but its asterisks cannot come from inside them.
  let masked = "";
  let end = 0;
  for (const match of protectedMatches) {
    const token = match[0];
    const urlEnding = /^https?:\/\//i.test(token) ? token.match(/\*+$/)?.[0] ?? "" : "";
    masked += text.slice(end, match.index) + " ".repeat(token.length - urlEnding.length) + urlEnding;
    end = match.index + token.length;
  }
  masked += text.slice(end);

  const emphasis = /\*\*[^*]+\*\*|\*[^*]+\*/g;
  let next = 0;
  let protectedEnd = 0;
  for (let match = emphasis.exec(masked); match; match = emphasis.exec(masked)) {
    while (next < protectedMatches.length && protectedMatches[next].index < match.index) {
      const token = protectedMatches[next++];
      protectedEnd = token.index + token[0].length;
      yield token;
    }
    if (match.index < protectedEnd) {
      emphasis.lastIndex = protectedEnd;
      continue;
    }
    end = emphasis.lastIndex;
    yield inlineMatch(text, match.index, end);
    while (next < protectedMatches.length && protectedMatches[next].index < end) next += 1;
  }
  while (next < protectedMatches.length) yield protectedMatches[next++];
}

function inlineMatch(text: string, start: number, end: number): MarkdownInlineMatch {
  const token: [string] = [text.slice(start, end)];
  return Object.assign(token, { index: start });
}

function protectedInlineMatches(text: string) {
  const starts = /`+|\$+|\\\(|!?\[|https?:\/\//gi;
  const linkOrURL = /!?\[(?:\\.|[^\]\\\n])*\]\((?:<[^<>\n]*>|(?:[^()\s]|\([^()\s]*\))+)(?:\s+"[^"\n]*")?\)|https?:\/\/[^\s<>`"'，。！？；：、（）【】《》「」『』]+/iy;
  const matches: MarkdownInlineMatch[] = [];
  for (let start = starts.exec(text); start; start = starts.exec(text)) {
    const marker = start[0];
    const index = start.index;
    let end: number | undefined;
    if (marker[0] === "`") {
      const backticks = /`+|[\r\n]/g;
      backticks.lastIndex = starts.lastIndex;
      for (let close = backticks.exec(text); close?.[0][0] === "`"; close = backticks.exec(text)) {
        if (close[0].length === marker.length) {
          end = backticks.lastIndex;
          break;
        }
      }
    } else if (marker === "$" || marker === "\\(") {
      end = inlineMathEnd(text, index, marker);
    } else if (marker[0] !== "$") {
      linkOrURL.lastIndex = index;
      if (linkOrURL.test(text)) end = linkOrURL.lastIndex;
    }
    if (end !== undefined) {
      matches.push(inlineMatch(text, index, end));
      starts.lastIndex = end;
    }
  }
  return matches;
}

function inlineMathEnd(text: string, start: number, opener: "$" | "\\("): number | undefined {
  if (isEscapedMarkdown(text, start)) return undefined;
  const bodyStart = start + opener.length;
  if (opener === "$" && (!text[bodyStart] || /\s/.test(text[bodyStart]))) return undefined;

  const delimiters = opener === "$" ? /\$+|[\r\n]/g : /\\[()]|[\r\n]/g;
  delimiters.lastIndex = bodyStart;
  for (let match = delimiters.exec(text); match; match = delimiters.exec(text)) {
    const close = match[0];
    if (close === "\n" || close === "\r") return undefined;
    const marker = unescapedMarker(text, match, opener === "$");
    if (!marker) continue;
    if (opener === "$") {
      // A closing dollar cannot touch whitespace on its left or a digit on its right.
      // In particular, two adjacent prices do not become one formula.
      if (marker.text !== "$" || marker.index === bodyStart || /\s/.test(text[marker.index - 1]) || /\d/.test(text[delimiters.lastIndex] ?? "")) return undefined;
    } else if (marker.text !== "\\)" || !text.slice(bodyStart, marker.index).trim()) {
      return undefined;
    }
    return delimiters.lastIndex;
  }
  return undefined;
}

function unescapedMarker(text: string, match: RegExpExecArray, dollars: boolean) {
  if (!isEscapedMarkdown(text, match.index)) return { text: match[0], index: match.index };
  // In a dollar run only its first character can be escaped.
  if (dollars && match[0].length > 1) return { text: match[0].slice(1), index: match.index + 1 };
  return undefined;
}

export function markdownInlineMath(token: string): { text: string; display?: boolean } | undefined {
  const opener = token.startsWith("\\(") ? "\\(" : token.startsWith("$") ? "$" : undefined;
  if (!opener || inlineMathEnd(token, 0, opener) !== token.length) return undefined;
  return { text: token.slice(opener.length, -opener.length).trim() };
}

export function isEscapedMarkdown(text: string, offset: number) {
  let slashes = 0;
  while (offset > 0 && text[--offset] === "\\") slashes += 1;
  return slashes % 2 === 1;
}

export function markdownLinkTarget(text: string) {
  return text.replace(/\s+"[^"\n]*"$/, "").replace(/^<(.*)>$/, "$1");
}

export function webURL(text: string) {
  if (!/^https?:\/\//i.test(text) || /[\s\u0000-\u001f\u007f]/.test(text)) return null;
  return assetPublicURL(text) || null;
}

export function markdownImage(token: string): MarkdownImage | undefined {
  const match = /^!\[((?:\\.|[^\]\\\n])*)\]\((.*)\)$/.exec(token);
  if (!match) return undefined;
  const alt = match[1].replace(/\\([\\\[\]])/g, "$1").trim();
  const target = markdownLinkTarget(match[2]);
  if (target.startsWith("asset:ast_") && target.length > "asset:ast_".length && isAssetID(target.slice(6))) {
    return { kind: "asset", assetID: target.slice(6), alt };
  }
  const url = webURL(target);
  return url ? { kind: "external", url, alt } : { kind: "unsupported", alt };
}

export function collectMarkdownAssetImages(blocks: readonly MarkdownBlock[]) {
  const references = new Map<string, string>();
  function visit(text: string) {
    for (const match of markdownInlineMatches(text)) {
      if (isEscapedMarkdown(text, match.index)) continue;
      const token = match[0];
      const image = markdownImage(token);
      if (image?.kind === "asset") {
        if (!references.get(image.assetID)) references.set(image.assetID, image.alt);
      } else if (token.startsWith("**")) {
        visit(token.slice(2, -2));
      } else if (token.startsWith("*")) {
        visit(token.slice(1, -1));
      }
    }
  }
  for (const block of blocks) {
    switch (block.type) {
      case "heading":
      case "paragraph": visit(block.text); break;
      case "list": block.items.forEach(visit); break;
      case "quote":
        for (const [assetID, alt] of collectMarkdownAssetImages(parseMarkdownBlocks(block.lines.join("\n")))) {
          if (!references.get(assetID)) references.set(assetID, alt);
        }
        break;
      case "table":
        block.header.forEach(visit);
        block.rows.forEach((row) => row.forEach(visit));
        break;
    }
  }
  return references;
}

export function assistantReplyAssets(text: string, produced: readonly SharedAsset[] = [], sessionAssets: readonly AssetSummary[] = []) {
  const images = collectMarkdownAssetImages(parseMarkdownBlocks(text));
  // The catalog can enrich metadata, but only a produced asset or an explicit
  // image reference may add a card to this reply.
  const ordered = new Map<string, SharedAsset>([...images].map(([assetID]) => [assetID, { assetID }]));
  for (const asset of produced) if (!ordered.has(asset.assetID)) ordered.set(asset.assetID, asset);
  const catalog = sessionAssets.filter((asset) => asset.asset_id && ordered.has(asset.asset_id)).map(assetSummaryToSharedAsset);
  return mergeSharedAssets([...ordered.values()], produced, catalog).map((asset) => ({
    asset: images.get(asset.assetID) ? { ...asset, fileName: images.get(asset.assetID) } : asset,
    imageReference: images.has(asset.assetID),
  }));
}

export function parseMarkdownBlocks(text: string): MarkdownBlock[] {
  const sourceLines = text.split("\n");
  const lines = sourceLines.map((line, index) => index < sourceLines.length - 1 ? line.replace(/\r$/, "") : line);
  const blocks: MarkdownBlock[] = [];

  const displays = new Map<number, ReturnType<typeof displayMathAt>>();
  function displayAt(index: number) {
    if (!displays.has(index)) displays.set(index, displayMathAt(lines, sourceLines, index));
    return displays.get(index);
  }

  for (let index = 0; index < lines.length; ) {
    const trimmed = lines[index].trim();
    if (!trimmed) {
      index += 1;
      continue;
    }

    const fence = trimmed.match(/^(`{3,}|~{3,})(.*)$/);
    if (fence) {
      const marker = fence[1];
      const codeLines: string[] = [];
      const closesFence = new RegExp(`^${marker[0]}{${marker.length},}\\s*$`);
      index += 1;
      while (index < lines.length && !closesFence.test(lines[index].trim())) codeLines.push(lines[index++]);
      if (index < lines.length) index += 1;
      blocks.push({ type: "code", language: fence[2].trim(), text: codeLines.join("\n") });
      continue;
    }
    if (/^(?: {4}|\t)/.test(lines[index])) {
      const codeLines: string[] = [];
      while (index < lines.length && /^(?: {4}|\t)/.test(lines[index])) {
        codeLines.push(lines[index++].replace(/^(?: {4}|\t)/, ""));
      }
      blocks.push({ type: "code", language: "", text: codeLines.join("\n") });
      continue;
    }

    const display = displayAt(index);
    if (display) {
      blocks.push(display.block);
      index = display.next;
      continue;
    }

    const heading = trimmed.match(/^(#{1,3})\s+(.+)$/);
    if (heading) {
      blocks.push({ type: "heading", level: heading[1].length as 1 | 2 | 3, text: heading[2] });
      index += 1;
      continue;
    }

    if (trimmed.startsWith(">")) {
      const quoteLines: string[] = [];
      while (index < lines.length && lines[index].trim().startsWith(">")) {
        quoteLines.push(lines[index++].trim().replace(/^>\s?/, ""));
      }
      blocks.push({ type: "quote", lines: quoteLines });
      continue;
    }

    const header = tableHeaderAt(lines, index);
    if (header) {
      const rows: string[][] = [];
      index += 2;
      while (index < lines.length) {
        const row = splitTableRow(lines[index].trim());
        if (!row || displayAt(index)) break;
        rows.push(row);
        index += 1;
      }
      blocks.push({ type: "table", header, rows });
      continue;
    }

    const ordered = /^\d+\.\s+(.+)$/;
    const unordered = /^[-*]\s+(.+)$/;
    if (ordered.test(trimmed) || unordered.test(trimmed)) {
      const isOrdered = ordered.test(trimmed);
      const pattern = isOrdered ? ordered : unordered;
      const items: string[] = [];
      while (index < lines.length) {
        const match = pattern.exec(lines[index].trim());
        if (!match) break;
        items.push(match[1]);
        index += 1;
      }
      blocks.push({ type: "list", ordered: isOrdered, items });
      continue;
    }

    const paragraphLines = [lines[index++]];
    while (index < lines.length) {
      const candidate = lines[index].trim();
      if (!candidate || /^(?:`{3,}|~{3,}|>|#{1,3}\s+)/.test(candidate) ||
          ordered.test(candidate) || unordered.test(candidate) || displayAt(index) || tableHeaderAt(lines, index)) break;
      paragraphLines.push(lines[index++]);
    }
    blocks.push({ type: "paragraph", text: paragraphLines.join("\n") });
  }

  return blocks;
}

function displayMathAt(lines: string[], sourceLines: string[], index: number): {
  block: Extract<MarkdownBlock, { type: "math" }>;
  next: number;
} | undefined {
  if (/^(?: {4}|\t)/.test(lines[index])) return undefined;
  const first = lines[index].trimStart();
  const dollars = first.startsWith("$$") && !first.startsWith("$$$");
  if (!dollars && !first.startsWith("\\[")) return undefined;
  const markers = dollars ? /\$+/g : /\\[\[\]]/g;
  const closing = dollars ? "$$" : "\\]";
  const body: string[] = [];
  for (let cursor = index; cursor < lines.length; cursor += 1) {
    const line = cursor === index ? first.slice(2) : lines[cursor];
    // An incomplete display opener must leave a following code fence available to Markdown.
    if (cursor > index && /^(?:`{3,}|~{3,})/.test(line.trimStart())) return undefined;
    markers.lastIndex = 0;
    for (let match = markers.exec(line); match; match = markers.exec(line)) {
      const marker = unescapedMarker(line, match, dollars);
      if (!marker || (dollars && marker.text.length === 1)) continue;
      if (marker.text !== closing || line.slice(markers.lastIndex).trim()) return undefined;
      body.push(line.slice(0, marker.index));
      const formula = body.join("\n").trim();
      if (!formula) return undefined;
      const source = [...sourceLines.slice(index, cursor), lines[cursor]].join("\n");
      return { block: { type: "math", text: formula, source }, next: cursor + 1 };
    }
    body.push(line);
  }
  return undefined;
}

function tableHeaderAt(lines: string[], index: number) {
  if (index + 1 >= lines.length) return undefined;
  const divider = splitTableRow(lines[index + 1].trim());
  if (!divider || !divider.every((cell) => /^:?-{3,}:?$/.test(cell))) return undefined;
  return splitTableRow(lines[index].trim());
}

function splitTableRow(line: string): string[] | undefined {
  if (!line.includes("|") || !line.replace(/\|/g, "").trim()) return undefined;
  const protectedMatches = protectedInlineMatches(line).filter((match) => /^[`$\\]/.test(match[0]));
  const cells: string[] = [];
  let start = 0;
  let next = 0;
  let hasPipe = false;
  for (let pipe = line.indexOf("|"); pipe !== -1; pipe = line.indexOf("|", pipe + 1)) {
    while (next < protectedMatches.length && protectedMatches[next].index + protectedMatches[next][0].length <= pipe) next += 1;
    if (protectedMatches[next]?.index <= pipe || isEscapedMarkdown(line, pipe)) continue;
    hasPipe = true;
    if (pipe !== 0) cells.push(line.slice(start, pipe).trim());
    start = pipe + 1;
  }
  if (!hasPipe) return undefined;
  if (start < line.length) cells.push(line.slice(start).trim());
  return cells;
}
