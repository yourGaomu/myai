import { type ReactNode, useMemo } from "react";
import { ScrollView, Text, View } from "react-native";

import { styles } from "./styles";

type MarkdownBlock =
  | { type: "heading"; level: 1 | 2 | 3; text: string }
  | { type: "paragraph"; text: string }
  | { type: "list"; ordered: boolean; items: string[] }
  | { type: "code"; language: string; text: string }
  | { type: "table"; header: string[]; rows: string[][] }
  | { type: "quote"; lines: string[] };

export function MarkdownText({ text }: { text: string }) {
  const blocks = useMemo(() => parseMarkdownBlocks(text), [text]);

  if (blocks.length === 0) {
    return <Text style={styles.messageText}>{text}</Text>;
  }

  return (
    <View style={styles.markdownRoot}>
      {blocks.map((block, index) => renderMarkdownBlock(block, index))}
    </View>
  );
}

function renderMarkdownBlock(block: MarkdownBlock, index: number) {
  if (block.type === "heading") {
    return (
      <Text
        key={`heading-${index}`}
        style={[
          styles.markdownHeading,
          block.level === 1 && styles.markdownHeading1,
          block.level === 2 && styles.markdownHeading2,
          block.level === 3 && styles.markdownHeading3,
        ]}
      >
        {renderMarkdownInline(block.text, `heading-${index}`)}
      </Text>
    );
  }

  if (block.type === "paragraph") {
    return (
      <Text key={`paragraph-${index}`} style={styles.markdownParagraph}>
        {renderMarkdownInline(block.text, `paragraph-${index}`)}
      </Text>
    );
  }

  if (block.type === "list") {
    return (
      <View key={`list-${index}`} style={styles.markdownList}>
        {block.items.map((item, itemIndex) => (
          <View key={`list-${index}-${itemIndex}`} style={styles.markdownListItem}>
            <Text style={styles.markdownListMarker}>{block.ordered ? `${itemIndex + 1}.` : "•"}</Text>
            <Text style={styles.markdownListText}>{renderMarkdownInline(item, `list-${index}-${itemIndex}`)}</Text>
          </View>
        ))}
      </View>
    );
  }

  if (block.type === "code") {
    return (
      <View key={`code-${index}`} style={styles.markdownCodeBlock}>
        {block.language ? <Text style={styles.markdownCodeLabel}>{block.language}</Text> : null}
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.markdownCodeScroll}>
          <Text selectable style={styles.markdownCodeText}>
            {block.text}
          </Text>
        </ScrollView>
      </View>
    );
  }

  if (block.type === "table") {
    return <MarkdownTable key={`table-${index}`} header={block.header} rows={block.rows} />;
  }

  return (
    <View key={`quote-${index}`} style={styles.markdownQuote}>
      <View style={styles.markdownQuoteBar} />
      <View style={styles.flex}>
        {block.lines.map((line, lineIndex) => (
          <Text key={`quote-${index}-${lineIndex}`} style={styles.markdownQuoteText}>
            {renderMarkdownInline(line, `quote-${index}-${lineIndex}`)}
          </Text>
        ))}
      </View>
    </View>
  );
}

function MarkdownTable({ header, rows }: { header: string[]; rows: string[][] }) {
  const columnCount = Math.max(header.length, ...rows.map((row) => row.length), 0);
  const columns = Array.from({ length: columnCount }, (_, index) => index);

  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.markdownTableScroll}>
      <View style={styles.markdownTable}>
        <View style={styles.markdownTableHeaderRow}>
          {columns.map((columnIndex) => (
            <View key={`header-${columnIndex}`} style={[styles.markdownTableCell, styles.markdownTableHeaderCell]}>
              <Text style={styles.markdownTableCellText}>{header[columnIndex] || ""}</Text>
            </View>
          ))}
        </View>
        {rows.map((row, rowIndex) => (
          <View key={`row-${rowIndex}`} style={[styles.markdownTableRow, rowIndex % 2 === 1 && styles.markdownTableRowAlt]}>
            {columns.map((columnIndex) => (
              <View key={`row-${rowIndex}-${columnIndex}`} style={styles.markdownTableCell}>
                <Text style={styles.markdownTableCellText}>{row[columnIndex] || ""}</Text>
              </View>
            ))}
          </View>
        ))}
      </View>
    </ScrollView>
  );
}

function renderMarkdownInline(text: string, keyPrefix: string): ReactNode[] {
  const parts: ReactNode[] = [];
  const pattern = /(\*\*[^*]+\*\*|`[^`]+`|\*[^*]+\*)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let partIndex = 0;

  while ((match = pattern.exec(text))) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }

    const token = match[0];
    if (token.startsWith("**")) {
      parts.push(
        <Text key={`${keyPrefix}-${partIndex}`} style={styles.markdownStrong}>
          {token.slice(2, -2)}
        </Text>,
      );
    } else if (token.startsWith("`")) {
      parts.push(
        <Text key={`${keyPrefix}-${partIndex}`} style={styles.markdownInlineCode}>
          {token.slice(1, -1)}
        </Text>,
      );
    } else {
      parts.push(
        <Text key={`${keyPrefix}-${partIndex}`} style={styles.markdownEmphasis}>
          {token.slice(1, -1)}
        </Text>,
      );
    }

    lastIndex = match.index + token.length;
    partIndex += 1;
  }

  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }

  return parts;
}

function parseMarkdownBlocks(text: string): MarkdownBlock[] {
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  const blocks: MarkdownBlock[] = [];

  for (let index = 0; index < lines.length; ) {
    const line = lines[index];
    const trimmed = line.trim();

    if (!trimmed) {
      index += 1;
      continue;
    }

    const fenceMatch = trimmed.match(/^```(\S*)\s*$/);
    if (fenceMatch) {
      const language = fenceMatch[1] || "";
      const codeLines: string[] = [];
      index += 1;
      while (index < lines.length && !lines[index].trim().startsWith("```")) {
        codeLines.push(lines[index]);
        index += 1;
      }
      if (index < lines.length) {
        index += 1;
      }
      blocks.push({ type: "code", language, text: codeLines.join("\n") });
      continue;
    }

    const headingMatch = trimmed.match(/^(#{1,3})\s+(.*)$/);
    if (headingMatch) {
      blocks.push({
        type: "heading",
        level: headingMatch[1].length as 1 | 2 | 3,
        text: headingMatch[2],
      });
      index += 1;
      continue;
    }

    if (trimmed.startsWith(">")) {
      const quoteLines: string[] = [];
      while (index < lines.length) {
        const quoteLine = lines[index].trim();
        if (!quoteLine.startsWith(">")) {
          break;
        }
        quoteLines.push(quoteLine.replace(/^>\s?/, ""));
        index += 1;
      }
      blocks.push({ type: "quote", lines: quoteLines });
      continue;
    }

    const listMatch = trimmed.match(/^(\d+\.|[-*+])\s+(.*)$/);
    if (listMatch) {
      const ordered = /^\d+\./.test(listMatch[1]);
      const items: string[] = [];
      while (index < lines.length) {
        const itemMatch = lines[index].trim().match(/^(\d+\.|[-*+])\s+(.*)$/);
        if (!itemMatch || /^\d+\./.test(itemMatch[1]) !== ordered) {
          break;
        }
        items.push(itemMatch[2]);
        index += 1;
      }
      blocks.push({ type: "list", ordered, items });
      continue;
    }

    if (line.includes("|") && index + 1 < lines.length && isTableDivider(lines[index + 1].trim())) {
      const header = parseMarkdownTableRow(line);
      const tableRows: string[][] = [];
      index += 2;
      while (index < lines.length) {
        const rowLine = lines[index].trim();
        if (!rowLine || !rowLine.includes("|")) {
          break;
        }
        if (isTableDivider(rowLine)) {
          index += 1;
          continue;
        }
        tableRows.push(parseMarkdownTableRow(rowLine));
        index += 1;
      }
      blocks.push({ type: "table", header, rows: tableRows });
      continue;
    }

    const paragraphLines: string[] = [line];
    index += 1;
    while (index < lines.length) {
      const nextLine = lines[index];
      const nextTrimmed = nextLine.trim();
      if (
        !nextTrimmed ||
        nextTrimmed.startsWith(">") ||
        nextTrimmed.startsWith("```") ||
        /^(#{1,3})\s+/.test(nextTrimmed) ||
        /^(\d+\.|[-*+])\s+/.test(nextTrimmed) ||
        (nextLine.includes("|") && index + 1 < lines.length && isTableDivider(lines[index + 1].trim()))
      ) {
        break;
      }
      paragraphLines.push(nextLine);
      index += 1;
    }
    blocks.push({ type: "paragraph", text: paragraphLines.join(" ") });
  }

  return blocks;
}

function parseMarkdownTableRow(line: string) {
  return line
    .trim()
    .replace(/^\|/, "")
    .replace(/\|$/, "")
    .split("|")
    .map((cell) => cell.trim());
}

function isTableDivider(line: string) {
  return /^\|?(?:\s*:?-{3,}:?\s*\|)+\s*$/.test(line.trim());
}
