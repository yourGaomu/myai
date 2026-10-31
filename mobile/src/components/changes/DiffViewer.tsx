import { useMemo, useState } from "react";
import { Platform, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { parseDiffRows, type DiffCell, type DiffLineKind, type DiffRow } from "../../utils/diff";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

type Props = {
  diff: string;
  emptyText: string;
};

export function DiffViewer({ diff, emptyText }: Props) {
  const [wrapLines, setWrapLines] = useState(true);
  const rows = useMemo(() => parseDiffRows(diff), [diff]);
  const stats = useMemo(() => diffStats(rows), [rows]);
  const ratioBlocks = useMemo(() => buildDiffRatioBlocks(stats.added, stats.removed), [stats.added, stats.removed]);

  if (!diff.trim() || rows.length === 0) {
    return <Text style={styles.emptyText}>{emptyText}</Text>;
  }

  const tableContent = (
    <View style={[styles.diffTable, !wrapLines && styles.diffTableNoWrap]}>
      {rows.map((row) => renderRow(row, wrapLines))}
    </View>
  );

  return (
    <View style={styles.diffViewer}>
      <View style={styles.diffHeader}>
        <View style={styles.diffHeaderLeft}>
          <Text style={styles.diffHeaderTitle}>行级差异</Text>
          <View style={styles.diffStats}>
            <Text style={styles.diffAddedStat}>+{stats.added}</Text>
            <Text style={styles.diffRemovedStat}>-{stats.removed}</Text>
            <View style={styles.diffRatioRow}>
              {ratioBlocks.map((kind, idx) => (
                <View
                  key={idx}
                  style={[
                    styles.diffRatioBlock,
                    kind === "add"
                      ? styles.diffRatioAdd
                      : kind === "remove"
                        ? styles.diffRatioRemove
                        : styles.diffRatioNeutral,
                  ]}
                />
              ))}
            </View>
          </View>
        </View>
        <Pressable
          onPress={() => setWrapLines((prev) => !prev)}
          style={styles.wrapToggleBtn}
        >
          <Feather color={palette.ink2} name={wrapLines ? "corner-up-left" : "repeat"} size={14} />
          <Text style={styles.wrapToggleText}>{wrapLines ? "自动换行" : "横向滚动"}</Text>
        </Pressable>
      </View>

      {wrapLines ? (
        tableContent
      ) : (
        <ScrollView horizontal showsHorizontalScrollIndicator={true}>
          {tableContent}
        </ScrollView>
      )}
    </View>
  );
}

function renderRow(row: DiffRow, wrapLines: boolean) {
  if (row.type === "full") {
    if (row.cell.kind === "meta") {
      return null;
    }
    if (row.cell.kind === "hunk") {
      const { rangeText, contextText } = splitHunkHeader(row.cell.text);
      return (
        <View key={row.id} style={styles.diffHunkRow}>
          <View style={styles.diffHunkPill}>
            <Text style={styles.diffHunkPillText}>{rangeText}</Text>
          </View>
          {contextText ? (
            <Text numberOfLines={1} style={styles.diffHunkContext}>
              {contextText}
            </Text>
          ) : null}
        </View>
      );
    }
    return <DiffCodeRow key={row.id} cell={row.cell} wrapLines={wrapLines} />;
  }

  const before = row.before;
  const after = row.after;
  if (before?.kind === "context" && after?.kind === "context" && before.text === after.text) {
    return <DiffCodeRow key={row.id} before={before} after={after} wrapLines={wrapLines} />;
  }

  return (
    <View key={row.id}>
      {before ? <DiffCodeRow before={before} wrapLines={wrapLines} /> : null}
      {after ? <DiffCodeRow after={after} wrapLines={wrapLines} /> : null}
    </View>
  );
}

function DiffCodeRow({
  before,
  after,
  cell,
  wrapLines,
}: {
  before?: DiffCell;
  after?: DiffCell;
  cell?: DiffCell;
  wrapLines: boolean;
}) {
  const primary = before || after || cell;
  if (!primary) {
    return null;
  }

  const kind = primary.kind;
  const oldLineNumber = before?.lineNumber ?? (kind === "remove" ? primary.lineNumber : undefined);
  const newLineNumber = after?.lineNumber ?? (kind === "add" ? primary.lineNumber : undefined);
  const text = before?.text ?? after?.text ?? primary.text;

  return (
    <View style={[styles.diffCodeRow, diffRowStyle(kind)]}>
      <View style={[styles.diffGutter, diffGutterStyle(kind)]}>
        <Text style={styles.diffLineNumber}>{oldLineNumber ?? ""}</Text>
        <Text style={styles.diffLineNumber}>{newLineNumber ?? ""}</Text>
      </View>
      <Text style={[styles.diffMarker, diffMarkerStyle(kind)]}>{diffMarker(kind)}</Text>
      <Text
        numberOfLines={wrapLines ? undefined : 1}
        selectable
        style={[styles.diffCode, !wrapLines && styles.diffCodeNoWrap]}
      >
        {text || " "}
      </Text>
    </View>
  );
}

function splitHunkHeader(raw: string): { rangeText: string; contextText: string } {
  const match = raw.match(/^(@@\s*-[0-9,]+\s+\+[0-9,]+\s*@@)\s*(.*)$/);
  if (!match) {
    return { rangeText: raw, contextText: "" };
  }
  return {
    rangeText: match[1],
    contextText: match[2].trim(),
  };
}

function buildDiffRatioBlocks(added: number, removed: number): Array<"add" | "remove" | "neutral"> {
  const total = added + removed;
  const slots = 5;
  if (total === 0) {
    return Array(slots).fill("neutral");
  }
  let addCount = Math.round((added / total) * slots);
  if (added > 0 && addCount === 0) addCount = 1;
  if (removed > 0 && addCount === slots) addCount = slots - 1;
  const removeCount = slots - addCount;
  const result: Array<"add" | "remove" | "neutral"> = [];
  for (let i = 0; i < addCount; i += 1) result.push("add");
  for (let i = 0; i < removeCount; i += 1) result.push("remove");
  return result;
}

function diffStats(rows: DiffRow[]) {
  let added = 0;
  let removed = 0;
  rows.forEach((row) => {
    const cells = row.type === "full" ? [row.cell] : [row.before, row.after];
    cells.forEach((cell) => {
      if (cell?.kind === "add") added += 1;
      if (cell?.kind === "remove") removed += 1;
    });
  });
  return { added, removed };
}

function diffMarker(kind: DiffLineKind) {
  switch (kind) {
    case "add": return "+";
    case "remove": return "-";
    default: return " ";
  }
}

function diffRowStyle(kind: DiffLineKind) {
  switch (kind) {
    case "add": return styles.diffAddRow;
    case "remove": return styles.diffRemoveRow;
    default: return styles.diffContextRow;
  }
}

function diffGutterStyle(kind: DiffLineKind) {
  switch (kind) {
    case "add": return styles.diffAddGutter;
    case "remove": return styles.diffRemoveGutter;
    default: return null;
  }
}

function diffMarkerStyle(kind: DiffLineKind) {
  switch (kind) {
    case "add": return styles.diffAddMarker;
    case "remove": return styles.diffRemoveMarker;
    default: return styles.diffContextMarker;
  }
}

const monoFont = Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" });

const styles = StyleSheet.create({
  emptyText: {
    color: palette.ink3,
    fontSize: font.label,
    padding: 10,
  },
  diffViewer: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.md,
    borderWidth: 1,
    overflow: "hidden",
    ...shadow.float,
  },
  diffHeader: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderBottomColor: palette.hairline,
    borderBottomWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    minHeight: 38,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  diffHeaderLeft: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
  },
  diffHeaderTitle: {
    color: palette.ink2,
    fontSize: font.caption,
    fontWeight: weight.bold,
    letterSpacing: 0.3,
  },
  diffStats: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  diffAddedStat: {
    color: palette.diffAdd,
    fontFamily: monoFont,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  diffRemovedStat: {
    color: palette.diffDel,
    fontFamily: monoFont,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  diffRatioRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 2,
    marginLeft: 2,
  },
  diffRatioBlock: {
    borderRadius: radius.xxs,
    height: 7,
    width: 7,
  },
  diffRatioAdd: {
    backgroundColor: palette.diffAddBg,
  },
  diffRatioRemove: {
    backgroundColor: palette.diffDelBg,
  },
  diffRatioNeutral: {
    backgroundColor: palette.surface2,
  },
  wrapToggleBtn: {
    alignItems: "center",
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 5,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  wrapToggleText: {
    color: palette.ink2,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  diffTable: {
    backgroundColor: palette.surface1,
  },
  diffTableNoWrap: {
    minWidth: "100%",
  },
  diffHunkRow: {
    alignItems: "center",
    backgroundColor: palette.surface2,
    borderBottomColor: palette.hairline,
    borderBottomWidth: 1,
    borderTopColor: palette.hairline,
    borderTopWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 8,
    paddingVertical: 5,
  },
  diffHunkPill: {
    backgroundColor: palette.brandWashStrong,
    borderColor: palette.brandGlow,
    borderRadius: radius.xs,
    borderWidth: 1,
    paddingHorizontal: 6,
    paddingVertical: 1,
  },
  diffHunkPillText: {
    color: palette.brand,
    fontFamily: monoFont,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  diffHunkContext: {
    color: palette.ink2,
    flex: 1,
    fontFamily: monoFont,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  diffCodeRow: {
    alignItems: "stretch",
    borderBottomColor: palette.hairline,
    borderBottomWidth: 1,
    borderLeftColor: "transparent",
    borderLeftWidth: 1,
    flexDirection: "row",
    minHeight: 23,
    paddingRight: 10,
  },
  diffContextRow: {
    backgroundColor: palette.surface1,
  },
  diffAddRow: {
    backgroundColor: palette.diffAddBg,
    borderLeftColor: palette.diffAdd,
  },
  diffRemoveRow: {
    backgroundColor: palette.diffDelBg,
    borderLeftColor: palette.diffDel,
  },
  diffGutter: {
    alignItems: "flex-start",
    backgroundColor: palette.glassInner,
    borderRightColor: palette.hairline,
    borderRightWidth: 1,
    flexDirection: "row",
    paddingRight: 2,
  },
  diffAddGutter: {
    backgroundColor: palette.okWash,
  },
  diffRemoveGutter: {
    backgroundColor: palette.errWash,
  },
  diffLineNumber: {
    color: palette.ink4,
    fontFamily: monoFont,
    fontSize: font.micro,
    lineHeight: 16,
    minWidth: 27,
    paddingHorizontal: 3,
    paddingTop: 3.5,
    textAlign: "right",
  },
  diffMarker: {
    fontFamily: monoFont,
    fontSize: font.caption,
    fontWeight: weight.bold,
    lineHeight: 16,
    paddingTop: 3.5,
    textAlign: "center",
    width: 18,
  },
  diffAddMarker: {
    color: palette.diffAdd,
  },
  diffRemoveMarker: {
    color: palette.diffDel,
  },
  diffContextMarker: {
    color: palette.ink3,
  },
  diffCode: {
    color: palette.ink1,
    flex: 1,
    fontFamily: monoFont,
    fontSize: font.caption,
    lineHeight: 16,
    minWidth: 0,
    paddingBottom: 3.5,
    paddingTop: 3.5,
  },
  diffCodeNoWrap: {
    flex: 0,
    paddingRight: 16,
  },
});
