import { useMemo } from "react";
import { Platform, StyleSheet, Text, View } from "react-native";

import { parseDiffRows, type DiffCell, type DiffLineKind, type DiffRow } from "../../utils/diff";

type Props = {
  diff: string;
  emptyText: string;
};

export function DiffViewer({ diff, emptyText }: Props) {
  const rows = useMemo(() => parseDiffRows(diff), [diff]);
  const stats = useMemo(() => diffStats(rows), [rows]);

  if (!diff.trim() || rows.length === 0) {
    return <Text style={styles.emptyText}>{emptyText}</Text>;
  }

  return (
    <View style={styles.diffViewer}>
      <View style={styles.diffHeader}>
        <Text style={styles.diffHeaderTitle}>行级差异</Text>
        <View style={styles.diffStats}>
          <Text style={styles.diffAddedStat}>+{stats.added}</Text>
          <Text style={styles.diffRemovedStat}>-{stats.removed}</Text>
        </View>
      </View>
      <View style={styles.diffTable}>
        {rows.map((row) => renderRow(row))}
      </View>
    </View>
  );
}

function renderRow(row: DiffRow) {
  if (row.type === "full") {
    if (row.cell.kind === "meta") {
      // The file path and change type are already shown by ChangeDetailPanel.
      return null;
    }
    if (row.cell.kind === "hunk") {
      return (
        <View key={row.id} style={styles.diffHunkRow}>
          <Text style={styles.diffHunkText}>{row.cell.text}</Text>
        </View>
      );
    }
    return <DiffCodeRow key={row.id} cell={row.cell} />;
  }

  const before = row.before;
  const after = row.after;
  if (before?.kind === "context" && after?.kind === "context" && before.text === after.text) {
    return <DiffCodeRow key={row.id} before={before} after={after} />;
  }

  return (
    <View key={row.id}>
      {before ? <DiffCodeRow before={before} /> : null}
      {after ? <DiffCodeRow after={after} /> : null}
    </View>
  );
}

function DiffCodeRow({ before, after, cell }: { before?: DiffCell; after?: DiffCell; cell?: DiffCell }) {
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
      <Text style={styles.diffLineNumber}>{oldLineNumber ?? ""}</Text>
      <Text style={styles.diffLineNumber}>{newLineNumber ?? ""}</Text>
      <Text style={[styles.diffMarker, diffMarkerStyle(kind)]}>{diffMarker(kind)}</Text>
      <Text selectable style={styles.diffCode}>{text || " "}</Text>
    </View>
  );
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

function diffMarkerStyle(kind: DiffLineKind) {
  switch (kind) {
    case "add": return styles.diffAddMarker;
    case "remove": return styles.diffRemoveMarker;
    default: return styles.diffContextMarker;
  }
}

const styles = StyleSheet.create({
  emptyText: {
    color: "#6c665f",
  },
  diffViewer: {
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    overflow: "hidden",
  },
  diffHeader: {
    alignItems: "center",
    backgroundColor: "#f5eefc",
    borderBottomColor: "#d7cfc2",
    borderBottomWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    minHeight: 38,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  diffHeaderTitle: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
    letterSpacing: 0.3,
  },
  diffStats: {
    flexDirection: "row",
    gap: 10,
  },
  diffAddedStat: {
    color: "#16834f",
    fontSize: 12,
    fontWeight: "900",
  },
  diffRemovedStat: {
    color: "#c43d32",
    fontSize: 12,
    fontWeight: "900",
  },
  diffTable: {
    backgroundColor: "#fffdf7",
  },
  diffHunkRow: {
    backgroundColor: "#fff4cc",
    borderBottomColor: "#eadca7",
    borderBottomWidth: 1,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  diffHunkText: {
    color: "#6f5b00",
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 10,
    fontWeight: "900",
    lineHeight: 15,
  },
  diffCodeRow: {
    alignItems: "flex-start",
    borderBottomColor: "#eee8df",
    borderBottomWidth: 1,
    flexDirection: "row",
    minHeight: 27,
    paddingRight: 8,
  },
  diffContextRow: {
    backgroundColor: "#fffdf7",
  },
  diffAddRow: {
    backgroundColor: "#e5f7e1",
    borderLeftColor: "#13a35d",
    borderLeftWidth: 3,
  },
  diffRemoveRow: {
    backgroundColor: "#ffe8e4",
    borderLeftColor: "#e64b40",
    borderLeftWidth: 3,
  },
  diffLineNumber: {
    color: "#989087",
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 10,
    lineHeight: 17,
    minWidth: 34,
    paddingHorizontal: 4,
    paddingTop: 5,
    textAlign: "right",
  },
  diffMarker: {
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 11,
    fontWeight: "900",
    lineHeight: 17,
    paddingTop: 5,
    textAlign: "center",
    width: 18,
  },
  diffAddMarker: {
    color: "#16834f",
  },
  diffRemoveMarker: {
    color: "#c43d32",
  },
  diffContextMarker: {
    color: "#989087",
  },
  diffCode: {
    color: "#25211d",
    flex: 1,
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 10,
    lineHeight: 17,
    minWidth: 0,
    paddingBottom: 5,
    paddingTop: 5,
  },
});
