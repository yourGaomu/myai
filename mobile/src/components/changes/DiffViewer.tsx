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
    color: "#8c857b",
    fontSize: 12,
    padding: 10,
  },
  diffViewer: {
    backgroundColor: "#1e1e1e",
    borderColor: "#25231f",
    borderRadius: 12,
    borderWidth: 2,
    overflow: "hidden",
  },
  diffHeader: {
    alignItems: "center",
    backgroundColor: "#282725",
    borderBottomColor: "#3a3834",
    borderBottomWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    minHeight: 34,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  diffHeaderTitle: {
    color: "#f7f5f0",
    fontSize: 11,
    fontWeight: "900",
    letterSpacing: 0.3,
  },
  diffStats: {
    flexDirection: "row",
    gap: 8,
  },
  diffAddedStat: {
    color: "#7ee787",
    fontSize: 12,
    fontWeight: "900",
  },
  diffRemovedStat: {
    color: "#ffa198",
    fontSize: 12,
    fontWeight: "900",
  },
  diffTable: {
    backgroundColor: "#1e1e1e",
  },
  diffHunkRow: {
    backgroundColor: "#2d2a24",
    borderBottomColor: "#3e3a32",
    borderBottomWidth: 1,
    paddingHorizontal: 10,
    paddingVertical: 5,
  },
  diffHunkText: {
    color: "#ffd84f",
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 10.5,
    fontWeight: "800",
    lineHeight: 15,
  },
  diffCodeRow: {
    alignItems: "flex-start",
    borderBottomColor: "#2a2926",
    borderBottomWidth: 1,
    flexDirection: "row",
    minHeight: 24,
    paddingRight: 8,
  },
  diffContextRow: {
    backgroundColor: "#1e1e1e",
  },
  diffAddRow: {
    backgroundColor: "rgba(46, 160, 67, 0.20)",
  },
  diffRemoveRow: {
    backgroundColor: "rgba(248, 81, 73, 0.20)",
  },
  diffLineNumber: {
    color: "#6e7681",
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 10,
    lineHeight: 16,
    minWidth: 32,
    paddingHorizontal: 4,
    paddingTop: 4,
    textAlign: "right",
  },
  diffMarker: {
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 11,
    fontWeight: "900",
    lineHeight: 16,
    paddingTop: 4,
    textAlign: "center",
    width: 16,
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
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 10.5,
    lineHeight: 16,
    minWidth: 0,
    paddingBottom: 4,
    paddingTop: 4,
  },
});
