import { useMemo } from "react";
import { Platform, StyleSheet, Text, View } from "react-native";

import { formatDiffLine, parseDiffRows, type DiffLineKind } from "../../utils/diff";

type Props = {
  diff: string;
  emptyText: string;
};

export function DiffViewer({ diff, emptyText }: Props) {
  const rows = useMemo(() => parseDiffRows(diff), [diff]);

  if (!diff.trim()) {
    return <Text style={styles.emptyText}>{emptyText}</Text>;
  }

  if (rows.length === 0) {
    return <Text style={styles.emptyText}>{emptyText}</Text>;
  }

  return (
    <View style={styles.diffStack}>
      {rows.map((row) =>
        row.type === "full" ? (
          <View key={row.id} style={[styles.diffBlock, diffBlockStyle(row.cell.kind)]}>
            <Text style={styles.diffBlockLabel}>
              {row.cell.kind === "hunk" ? "Hunk" : row.cell.kind === "meta" ? "Meta" : "Line"}
            </Text>
            <Text style={[styles.diffLine, diffLineStyle(row.cell.kind)]}>
              {row.cell.kind === "hunk" || row.cell.kind === "meta" ? row.cell.text : formatDiffLine(row.cell)}
            </Text>
          </View>
        ) : row.before?.kind === "context" && row.after?.kind === "context" && row.before.text === row.after.text ? (
          <View key={row.id} style={[styles.diffBlock, styles.diffContextBlock]}>
            <Text style={styles.diffBlockLabel}>Context</Text>
            <Text style={[styles.diffLine, diffLineStyle(row.before.kind)]}>{formatDiffLine(row.before)}</Text>
          </View>
        ) : (
          <View key={row.id} style={styles.diffPairCard}>
            {row.before ? (
              <View style={[styles.diffBlock, styles.diffBeforeBlock]}>
                <View style={styles.diffBlockHeader}>
                  <Text style={[styles.diffBlockPill, styles.diffBeforePill]}>BEFORE</Text>
                  <Text style={styles.diffBlockLabel}>{row.before.kind === "remove" ? "Removed line" : "Original line"}</Text>
                </View>
                <Text style={[styles.diffLine, diffLineStyle(row.before.kind)]}>{formatDiffLine(row.before)}</Text>
              </View>
            ) : null}
            {row.after ? (
              <View style={[styles.diffBlock, styles.diffAfterBlock]}>
                <View style={styles.diffBlockHeader}>
                  <Text style={[styles.diffBlockPill, styles.diffAfterPill]}>AFTER</Text>
                  <Text style={styles.diffBlockLabel}>{row.after.kind === "add" ? "Added line" : "New line"}</Text>
                </View>
                <Text style={[styles.diffLine, diffLineStyle(row.after.kind)]}>{formatDiffLine(row.after)}</Text>
              </View>
            ) : null}
          </View>
        ),
      )}
    </View>
  );
}

function diffLineStyle(kind: DiffLineKind) {
  switch (kind) {
    case "add":
      return styles.diffLineAdd;
    case "remove":
      return styles.diffLineRemove;
    case "hunk":
      return styles.diffLineHunk;
    case "meta":
      return styles.diffLineMeta;
    default:
      return styles.diffLineContext;
  }
}

function diffBlockStyle(kind: DiffLineKind) {
  switch (kind) {
    case "add":
      return styles.diffAfterBlock;
    case "remove":
      return styles.diffBeforeBlock;
    case "hunk":
      return styles.diffHunkBlock;
    case "meta":
      return styles.diffMetaBlock;
    default:
      return styles.diffContextBlock;
  }
}

const styles = StyleSheet.create({
  emptyText: {
    color: "#6c665f",
  },
  diffStack: {
    gap: 8,
  },
  diffPairCard: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    gap: 8,
    padding: 8,
  },
  diffBlock: {
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    gap: 6,
    padding: 8,
  },
  diffBlockHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  diffBlockPill: {
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 2,
    color: "#12100e",
    fontSize: 9,
    fontWeight: "900",
    letterSpacing: 0,
    paddingHorizontal: 7,
    paddingVertical: 2,
  },
  diffBeforePill: {
    backgroundColor: "#ffb7a7",
  },
  diffAfterPill: {
    backgroundColor: "#b9e9b0",
  },
  diffBlockLabel: {
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
  },
  diffBeforeBlock: {
    backgroundColor: "#ffe3dc",
  },
  diffAfterBlock: {
    backgroundColor: "#e2f7dc",
  },
  diffContextBlock: {
    backgroundColor: "#f5eefc",
  },
  diffHunkBlock: {
    backgroundColor: "#fff4cc",
  },
  diffMetaBlock: {
    backgroundColor: "#ece2f5",
  },
  diffLine: {
    color: "#12100e",
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 10,
    lineHeight: 15,
  },
  diffLineAdd: {
    backgroundColor: "#dff6d8",
  },
  diffLineRemove: {
    backgroundColor: "#ffd9d1",
    textDecorationLine: "line-through",
  },
  diffLineContext: {
    color: "#39332d",
  },
  diffLineHunk: {
    color: "#12100e",
    fontWeight: "900",
  },
  diffLineMeta: {
    color: "#6c665f",
    fontWeight: "800",
  },
});
