import { useMemo, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { parseDiffRows, type DiffCell, type DiffLineKind, type DiffRow } from "../../utils/diff";
import { fontFamily, font, palette, radius } from "../../constants/theme";

type Props = {
  diff: string;
  emptyText: string;
};

// 统一 diff：两列行号 + 符号列 + 代码。只给行底色上色，正文保持主文字色，便于阅读。
export function DiffViewer({ diff, emptyText }: Props) {
  const [wrap, setWrap] = useState(true);
  const [viewport, setViewport] = useState(0);
  const rows = useMemo(() => parseDiffRows(diff), [diff]);
  const lines = useMemo(() => unifiedLines(rows), [rows]);
  const stats = useMemo(() => diffStats(rows), [rows]);

  if (!diff.trim() || rows.length === 0) {
    return <Text style={styles.empty}>{emptyText}</Text>;
  }

  const body = (
    <View style={[styles.body, !wrap && viewport ? { minWidth: viewport } : null]}>
      {lines.map((line) =>
        line.kind === "hunk" ? (
          <Text key={line.id} numberOfLines={1} style={styles.hunk}>
            {line.text}
          </Text>
        ) : (
          <Line key={line.id} kind={line.kind} newNo={line.newNo} oldNo={line.oldNo} text={line.text} wrap={wrap} />
        ),
      )}
    </View>
  );

  return (
    <View style={styles.viewer}>
      <View style={styles.head}>
        <Text style={styles.add}>+{stats.added}</Text>
        <Text style={styles.del}>−{stats.removed}</Text>
        <View style={styles.flex} />
        <Pressable hitSlop={6} onPress={() => setWrap((value) => !value)} style={({ pressed }) => [styles.toggle, pressed && styles.pressed]}>
          <Feather color={palette.ink3} name={wrap ? "align-left" : "arrow-right"} size={13} />
          <Text style={styles.toggleText}>{wrap ? "自动换行" : "不换行"}</Text>
        </Pressable>
      </View>
      {wrap ? (
        body
      ) : (
        <ScrollView horizontal onLayout={(event) => setViewport(event.nativeEvent.layout.width)} showsHorizontalScrollIndicator={false}>
          {body}
        </ScrollView>
      )}
    </View>
  );
}

type UnifiedLine = { id: string; kind: DiffLineKind; newNo?: number; oldNo?: number; text: string };

// parseDiffRows 为左右分栏把删除和新增两两配对；单栏显示要还原成「先删后增」的顺序
function unifiedLines(rows: DiffRow[]): UnifiedLine[] {
  const out: UnifiedLine[] = [];
  let removed: UnifiedLine[] = [];
  let added: UnifiedLine[] = [];
  const flush = () => {
    out.push(...removed, ...added);
    removed = [];
    added = [];
  };
  const text = (value: string) => value.replace(/\t/g, "  ");
  for (const row of rows) {
    if (row.type === "full") {
      if (row.cell.kind === "meta") continue;
      flush();
      out.push({ id: row.id, kind: row.cell.kind, text: row.cell.text });
      continue;
    }
    const { after, before } = row;
    if (before?.kind === "context" || after?.kind === "context") {
      flush();
      const cell = before || after;
      out.push({ id: row.id, kind: "context", newNo: after?.lineNumber, oldNo: before?.lineNumber, text: text(cell?.text || "") });
      continue;
    }
    if (before) removed.push({ id: row.id + "-b", kind: before.kind, oldNo: before.lineNumber, text: text(before.text) });
    if (after) added.push({ id: row.id + "-a", kind: after.kind, newNo: after.lineNumber, text: text(after.text) });
  }
  flush();
  return out;
}

function Line({ kind, newNo, oldNo, text, wrap }: { kind: DiffLineKind; newNo?: number; oldNo?: number; text: string; wrap: boolean }) {
  const sign = kind === "add" ? "+" : kind === "remove" ? "−" : "";
  return (
    <View style={[styles.line, kind === "add" && styles.lineAdd, kind === "remove" && styles.lineDel]}>
      <Text style={styles.no}>{oldNo ?? ""}</Text>
      <Text style={styles.no}>{newNo ?? ""}</Text>
      <Text style={[styles.sign, kind === "add" && styles.signAdd, kind === "remove" && styles.signDel]}>{sign}</Text>
      <Text numberOfLines={wrap ? undefined : 1} selectable style={[styles.code, wrap && styles.codeWrap]}>
        {text || " "}
      </Text>
    </View>
  );
}

function diffStats(rows: DiffRow[]) {
  let added = 0;
  let removed = 0;
  rows.forEach((row) => {
    const cells: (DiffCell | undefined)[] = row.type === "full" ? [row.cell] : [row.before, row.after];
    cells.forEach((cell) => {
      if (cell?.kind === "add") added += 1;
      if (cell?.kind === "remove") removed += 1;
    });
  });
  return { added, removed };
}

const lineHeight = 21;

const styles = StyleSheet.create({
  empty: { color: palette.ink2, fontSize: font.label, paddingVertical: 16 },
  viewer: { marginHorizontal: -16 },
  head: { alignItems: "center", flexDirection: "row", gap: 8, minHeight: 36, paddingHorizontal: 16 },
  add: { color: palette.diffAdd, fontFamily: fontFamily.mono, fontSize: font.caption },
  del: { color: palette.diffDel, fontFamily: fontFamily.mono, fontSize: font.caption },
  flex: { flex: 1 },
  toggle: { alignItems: "center", borderRadius: radius.xs, flexDirection: "row", gap: 4, paddingHorizontal: 6, paddingVertical: 4 },
  pressed: { backgroundColor: palette.pressed },
  toggleText: { color: palette.ink3, fontSize: font.caption },
  body: { paddingBottom: 6 },
  hunk: { color: palette.ink3, fontFamily: fontFamily.mono, fontSize: font.caption - 0.5, paddingHorizontal: 16, paddingVertical: 6 },
  line: { alignItems: "flex-start", flexDirection: "row", paddingRight: 16 },
  lineAdd: { backgroundColor: palette.diffAddBg },
  lineDel: { backgroundColor: palette.diffDelBg },
  no: { color: palette.ink4, fontFamily: fontFamily.mono, fontSize: font.micro - 0.5, lineHeight, paddingRight: 6, textAlign: "right", width: 32 },
  sign: { color: palette.ink4, fontFamily: fontFamily.mono, fontSize: font.caption, lineHeight, textAlign: "center", width: 16 },
  signAdd: { color: palette.diffAdd },
  signDel: { color: palette.diffDel },
  code: { color: palette.ink1, fontFamily: fontFamily.mono, fontSize: font.caption, lineHeight },
  codeWrap: { flex: 1 },
});
