import { isAssetID } from "./assetImagePreview";
import type { AssetSummary } from "../protocol";
import { assetPublicURL, assetSummaryToSharedAsset, mergeSharedAssets, type SharedAsset } from "./toolAssets";

export type MarkdownBlock =
  | { type: "heading"; level: 1 | 2 | 3; text: string }
  | { type: "paragraph"; text: string }
  | { type: "list"; ordered: boolean; items: string[] }
  | { type: "code"; language: string; text: string }
  | { type: "table"; header: string[]; rows: string[][] }
  | { type: "quote"; lines: string[] };

export type MarkdownImage = { alt: string } & (
  | { kind: "asset"; assetID: string }
  | { kind: "external"; url: string }
  | { kind: "unsupported" }
);

export function markdownInlineMatches(text: string) {
  const tokens = /(`+[^`\n]*`+(?!`)|\*\*[^*]+\*\*|\*[^*]+\*|!?\[(?:\\.|[^\]\\\n])*\]\((?:<[^<>\n]*>|(?:[^()\s]|\([^()\s]*\))+)(?:\s+"[^"\n]*")?\)|https?:\/\/[^\s<>`"'，。！？；：、（）【】《》「」『』]+)/gi;
  return text.matchAll(tokens);
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
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  const blocks: MarkdownBlock[] = [];

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

    if (startsTable(lines, index)) {
      const header = splitTableRow(trimmed);
      const rows: string[][] = [];
      index += 2;
      while (index < lines.length && isTableRow(lines[index].trim())) rows.push(splitTableRow(lines[index++].trim()));
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
          ordered.test(candidate) || unordered.test(candidate) || startsTable(lines, index)) break;
      paragraphLines.push(lines[index++]);
    }
    blocks.push({ type: "paragraph", text: paragraphLines.join("\n") });
  }

  return blocks;
}

function startsTable(lines: string[], index: number) {
  return index + 1 < lines.length && isTableRow(lines[index].trim()) && isTableDivider(lines[index + 1].trim());
}

function isTableRow(line: string) {
  return line.includes("|") && line.replace(/\|/g, "").trim().length > 0;
}

function isTableDivider(line: string) {
  return line.includes("|") && splitTableRow(line).every((cell) => /^:?-{3,}:?$/.test(cell.trim()));
}

function splitTableRow(line: string) {
  return line.replace(/^\|/, "").replace(/\|$/, "").split("|").map((cell) => cell.trim());
}
