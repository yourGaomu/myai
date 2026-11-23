import { type ReactNode, useMemo, useState } from "react";
import { Linking, Platform, Pressable, ScrollView, StyleSheet, Text, View, type TextStyle } from "react-native";
import { Feather } from "@expo/vector-icons";
import * as Haptics from "expo-haptics";

import { useAppTheme } from "../../theme/ThemeProvider";
import { copyToClipboard } from "../../utils/clipboard";
import { isEscapedMarkdown, markdownImage, markdownInlineMatches, markdownInlineMath, markdownLinkTarget, parseMarkdownBlocks, webURL, type MarkdownBlock } from "../../utils/chatMarkdown";
import { MathFormula } from "./MathFormula";
import { useChatStyles, type ChatStyles } from "./styles";

// tone="muted" 用于过程时间线里的思考文字：更小、更淡，和最终回答拉开层次。
export function MarkdownText({ text, tone = "default" }: { text: string; tone?: "default" | "muted" }) {
  const styles = useChatStyles();
  const blocks = useMemo(() => parseMarkdownBlocks(text), [text]);

  if (blocks.length === 0) {
    return (
      <Text selectable style={[styles.messageText, tone === "muted" && styles.markdownMutedText]}>
        {text}
      </Text>
    );
  }

  return (
    <View style={styles.markdownRoot}>
      {blocks.map((block, index) => renderMarkdownBlock(block, index, tone, styles))}
    </View>
  );
}

function renderMarkdownBlock(block: MarkdownBlock, index: number, tone: "default" | "muted", styles: ChatStyles) {
  const muted = tone === "muted" ? styles.markdownMutedText : null;
  if (block.type === "math") {
    return <MathFormula key={`math-${index}`} display source={block.source} text={block.text} textStyle={StyleSheet.flatten([styles.markdownParagraph, muted])} />;
  }

  if (block.type === "heading") {
    const headingStyle = [styles.markdownHeading, styles[`markdownHeading${block.level}`]];
    return (
      <Text key={`heading-${index}`} selectable style={headingStyle}>
        {renderMarkdownInline(block.text, `heading-${index}`, styles, StyleSheet.flatten(headingStyle))}
      </Text>
    );
  }

  if (block.type === "paragraph") {
    return (
      <Text key={`paragraph-${index}`} selectable style={[styles.markdownParagraph, muted]}>
        {renderMarkdownInline(block.text, `paragraph-${index}`, styles, StyleSheet.flatten([styles.markdownParagraph, muted]))}
      </Text>
    );
  }

  if (block.type === "list") {
    return (
      <View key={`list-${index}`} style={styles.markdownList}>
        {block.items.map((item, itemIndex) => (
          <View key={`list-${index}-${itemIndex}`} style={styles.markdownListItem}>
            <Text style={[styles.markdownListMarker, muted]}>{block.ordered ? `${itemIndex + 1}.` : "•"}</Text>
            <Text selectable style={[styles.markdownListText, muted]}>
              {renderMarkdownInline(item, `list-${index}-${itemIndex}`, styles, StyleSheet.flatten([styles.markdownListText, muted]))}
            </Text>
          </View>
        ))}
      </View>
    );
  }

  if (block.type === "code") {
    return <MarkdownCodeBlock key={`code-${index}`} language={block.language} styles={styles} text={block.text} />;
  }

  if (block.type === "table") {
    return <MarkdownTable key={`table-${index}`} header={block.header} rows={block.rows} styles={styles} />;
  }

  return (
    <View key={`quote-${index}`} style={styles.markdownQuote}>
      <View style={styles.markdownQuoteBar} />
      <View style={styles.flex}>
        {block.lines.map((line, lineIndex) => (
          <Text key={`quote-${index}-${lineIndex}`} selectable style={styles.markdownQuoteText}>
            {renderMarkdownInline(line, `quote-${index}-${lineIndex}`, styles, styles.markdownQuoteText)}
          </Text>
        ))}
      </View>
    </View>
  );
}

function MarkdownCodeBlock({ language, styles, text }: { language: string; styles: ChatStyles; text: string }) {
  const { palette } = useAppTheme();
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    if (await copyToClipboard(text)) {
      if (Platform.OS !== "web") {
        void Haptics.selectionAsync().catch(() => {});
      }
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    }
  };

  return (
    <View style={styles.markdownCodeBlock}>
      <View style={styles.markdownCodeHead}>
        <Text style={styles.markdownCodeLabel}>{language || "code"}</Text>
        <Pressable accessibilityRole="button" hitSlop={8} onPress={() => void handleCopy()} style={[styles.markdownCodeCopyBtn, copied && styles.markdownCodeCopyBtnCopied]}>
          <Feather color={copied ? palette.ok : palette.ink3} name={copied ? "check" : "copy"} size={12} />
          <Text style={[styles.markdownCodeCopyText, copied && { color: palette.ok }]}>{copied ? "已复制" : "复制"}</Text>
        </Pressable>
      </View>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.markdownCodeScroll}>
        <Text selectable style={styles.markdownCodeText}>
          {text}
        </Text>
      </ScrollView>
    </View>
  );
}

function MarkdownTable({ header, rows, styles }: { header: string[]; rows: string[][]; styles: ChatStyles }) {
  const columnCount = Math.max(header.length, ...rows.map((row) => row.length), 0);
  const columns = Array.from({ length: columnCount }, (_, index) => index);

  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.markdownTableScroll}>
      <View style={styles.markdownTable}>
        <View style={styles.markdownTableHeaderRow}>
          {columns.map((columnIndex) => (
            <View key={`header-${columnIndex}`} style={[styles.markdownTableCell, styles.markdownTableHeaderCell]}>
              <Text selectable style={styles.markdownTableCellText}>
                {renderMarkdownInline(header[columnIndex] || "", `table-header-${columnIndex}`, styles, styles.markdownTableCellText)}
              </Text>
            </View>
          ))}
        </View>
        {rows.map((row, rowIndex) => (
          <View key={`row-${rowIndex}`} style={[styles.markdownTableRow, rowIndex % 2 === 1 && styles.markdownTableRowAlt]}>
            {columns.map((columnIndex) => (
              <View key={`row-${rowIndex}-${columnIndex}`} style={styles.markdownTableCell}>
                <Text selectable style={styles.markdownTableCellText}>
                  {renderMarkdownInline(row[columnIndex] || "", `table-${rowIndex}-${columnIndex}`, styles, styles.markdownTableCellText)}
                </Text>
              </View>
            ))}
          </View>
        ))}
      </View>
    </ScrollView>
  );
}

function MarkdownLink({ children, styles, url }: { children: ReactNode; styles: ChatStyles; url: string }) {
  const [failed, setFailed] = useState(false);
  const openLink = async () => {
    setFailed(false);
    try {
      await Linking.openURL(url);
    } catch {
      setFailed(true);
    }
  };

  return (
    <Text accessibilityHint={url} accessibilityRole="link" onPress={openLink} style={styles.markdownLink}>
      {children}
      {failed ? <Text accessibilityLiveRegion="polite" style={styles.markdownLinkError}>（打开失败：{url}，可长按复制）</Text> : null}
    </Text>
  );
}

function trimURL(text: string) {
  let end = text.length;
  while (end > 0) {
    const last = text[end - 1];
    if (/[.,!?;:]/.test(last)) {
      end -= 1;
      continue;
    }
    const opening = last === ")" ? "(" : last === "]" ? "[" : last === "}" ? "{" : "";
    if (!opening) break;
    const value = text.slice(0, end);
    if (value.split(last).length <= value.split(opening).length) break;
    end -= 1;
  }
  return text.slice(0, end);
}

function renderMarkdownInline(text: string, keyPrefix: string, styles: ChatStyles, textStyle: TextStyle, allowLinks = true): ReactNode[] {
  const parts: ReactNode[] = [];
  let lastIndex = 0;
  let partIndex = 0;

  for (const match of markdownInlineMatches(text)) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }

    const token = match[0];
    const key = `${keyPrefix}-${partIndex}`;
    const formula = markdownInlineMath(token);
    if (isEscapedMarkdown(text, match.index)) {
      parts.push(token);
    } else if (formula) {
      parts.push(<MathFormula key={key} source={token} text={formula.text} textStyle={textStyle} />);
    } else if (token.startsWith("**")) {
      parts.push(
        <Text key={key} style={styles.markdownStrong}>
          {renderMarkdownInline(token.slice(2, -2), key, styles, textStyle, allowLinks)}
        </Text>,
      );
    } else if (token.startsWith("`")) {
      const fence = token.match(/^`+/)?.[0].length || 1;
      parts.push(
        <Text key={key} style={styles.markdownInlineCode}>
          {token.slice(fence, -fence)}
        </Text>,
      );
    } else if (token.startsWith("*")) {
      parts.push(
        <Text key={key} style={styles.markdownEmphasis}>
          {renderMarkdownInline(token.slice(1, -1), key, styles, textStyle, allowLinks)}
        </Text>,
      );
    } else if (!allowLinks) {
      parts.push(token);
    } else if (token.startsWith("![")) {
      const image = markdownImage(token);
      if (image?.kind === "external") {
        parts.push(<MarkdownLink key={key} styles={styles} url={image.url}>{image.alt || "查看图片"}</MarkdownLink>);
      } else if (image) {
        parts.push(image.alt || "图片");
      } else {
        parts.push(token);
      }
    } else if (token.startsWith("[")) {
      const link = token.match(/^\[((?:\\.|[^\]\\\n])+)\]\((.*)\)$/);
      const url = link && webURL(markdownLinkTarget(link[2]));
      if (link && url) {
        parts.push(<MarkdownLink key={key} styles={styles} url={url}>{renderMarkdownInline(link[1], `${key}-label`, styles, { ...textStyle, color: styles.markdownLink.color }, false)}</MarkdownLink>);
      } else {
        parts.push(token);
      }
    } else {
      const target = trimURL(token);
      const url = webURL(target);
      if (url) {
        parts.push(<MarkdownLink key={key} styles={styles} url={url}>{target}</MarkdownLink>, token.slice(target.length));
      } else {
        parts.push(token);
      }
    }

    lastIndex = match.index + token.length;
    partIndex += 1;
  }

  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }

  return parts;
}
