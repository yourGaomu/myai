import { useMemo, useState } from "react";
import { Platform, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import { parseDiffRows, type DiffCell, type DiffLineKind, type DiffRow } from "../../utils/diff";

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
          <Text style={styles.wrapToggleText}>{wrapLines ? "↩ 自动换行" : "⇄ 横向滚动"}</Text>
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
    color: "#8c857b",
    fontSize: 12,
    padding: 10,
  },
  diffViewer: {
    backgroundColor: "#141413",
    borderColor: "#12100e",
    borderRadius: 12,
    borderWidth: 2,
    overflow: "hidden",
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  diffHeader: {
    alignItems: "center",
    backgroundColor: "#22211f",
    borderBottomColor: "#383632",
    borderBottomWidth: 1.5,
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
    color: "#f7f5f0",
    fontSize: 11.5,
    fontWeight: "900",
    letterSpacing: 0.3,
  },
  diffStats: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  diffAddedStat: {
    color: "#7ee787",
    fontFamily: monoFont,
    fontSize: 11.5,
    fontWeight: "900",
  },
  diffRemovedStat: {
    color: "#ffa198",
    fontFamily: monoFont,
    fontSize: 11.5,
    fontWeight: "900",
  },
  diffRatioRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 2,
    marginLeft: 2,
  },
  diffRatioBlock: {
    borderRadius: 1.5,
    height: 7,
    width: 7,
  },
  diffRatioAdd: {
    backgroundColor: "#3fb950",
  },
  diffRatioRemove: {
    backgroundColor: "#f85149",
  },
  diffRatioNeutral: {
    backgroundColor: "#484f58",
  },
  wrapToggleBtn: {
    backgroundColor: "#32302c",
    borderColor: "#524e48",
    borderRadius: 6,
    borderWidth: 1,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  wrapToggleText: {
    color: "#ffd84f",
    fontSize: 10.5,
    fontWeight: "800",
  },
  diffTable: {
    backgroundColor: "#141413",
  },
  diffTableNoWrap: {
    minWidth: "100%",
  },
  diffHunkRow: {
    alignItems: "center",
    backgroundColor: "#25221b",
    borderBottomColor: "#38342b",
    borderBottomWidth: 1,
    borderTopColor: "#38342b",
    borderTopWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 8,
    paddingVertical: 5,
  },
  diffHunkPill: {
    backgroundColor: "rgba(255, 216, 79, 0.16)",
    borderColor: "rgba(255, 216, 79, 0.4)",
    borderRadius: 4,
    borderWidth: 1,
    paddingHorizontal: 6,
    paddingVertical: 1,
  },
  diffHunkPillText: {
    color: "#ffd84f",
    fontFamily: monoFont,
    fontSize: 10,
    fontWeight: "900",
  },
  diffHunkContext: {
    color: "#a8a299",
    flex: 1,
    fontFamily: monoFont,
    fontSize: 10.5,
    fontWeight: "700",
  },
  diffCodeRow: {
    alignItems: "stretch",
    borderBottomColor: "#1f1e1c",
    borderBottomWidth: 1,
    borderLeftColor: "transparent",
    borderLeftWidth: 3,
    flexDirection: "row",
    minHeight: 23,
    paddingRight: 10,
  },
  diffContextRow: {
    backgroundColor: "#141413",
  },
  diffAddRow: {
    backgroundColor: "rgba(46, 160, 67, 0.18)",
    borderLeftColor: "#3fb950",
  },
  diffRemoveRow: {
    backgroundColor: "rgba(248, 81, 73, 0.18)",
    borderLeftColor: "#f85149",
  },
  diffGutter: {
    alignItems: "flex-start",
    backgroundColor: "rgba(255, 255, 255, 0.02)",
    borderRightColor: "#2b2926",
    borderRightWidth: 1,
    flexDirection: "row",
    paddingRight: 2,
  },
  diffAddGutter: {
    backgroundColor: "rgba(46, 160, 67, 0.12)",
  },
  diffRemoveGutter: {
    backgroundColor: "rgba(248, 81, 73, 0.12)",
  },
  diffLineNumber: {
    color: "#6e7681",
    fontFamily: monoFont,
    fontSize: 9.5,
    lineHeight: 16,
    minWidth: 27,
    paddingHorizontal: 3,
    paddingTop: 3.5,
    textAlign: "right",
  },
  diffMarker: {
    fontFamily: monoFont,
    fontSize: 11,
    fontWeight: "900",
    lineHeight: 16,
    paddingTop: 3.5,
    textAlign: "center",
    width: 18,
  },
  diffAddMarker: {
    color: "#7ee787",
  },
  diffRemoveMarker: {
    color: "#ffa198",
  },
  diffContextMarker: {
    color: "#6e7681",
  },
  diffCode: {
    color: "#e6edf3",
    flex: 1,
    fontFamily: monoFont,
    fontSize: 11,
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
