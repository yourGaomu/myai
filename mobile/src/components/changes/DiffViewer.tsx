import { useEffect, useMemo, useRef, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { parseDiffRows, type DiffCell, type DiffLineKind, type DiffRow } from "../../utils/diff";
import { fontFamily, font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";

type Props = {
  diff: string;
  emptyText: string;
  standaloneScroll?: boolean;
};

type UnifiedLine = {
  id: string;
  isChunkStart?: boolean;
  kind: DiffLineKind;
  newNo?: number;
  oldNo?: number;
  text: string;
};

// 统一 diff：吸顶工具栏（+X / -Y、换行切换、↑ 上一处 / ↓ 下一处差异定位）+ 清晰浅底色行读区。
export function DiffViewer({ diff, emptyText, standaloneScroll = false }: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const [wrap, setWrap] = useState(true);
  const [viewport, setViewport] = useState(0);
  const [activeChunkIndex, setActiveChunkIndex] = useState(0);

  const verticalScrollRef = useRef<ScrollView>(null);
  const chunkOffsetsRef = useRef<Record<string, number>>({});

  const rows = useMemo(() => parseDiffRows(diff), [diff]);
  const lines = useMemo(() => unifiedLines(rows), [rows]);
  const stats = useMemo(() => diffStats(rows), [rows]);
  const chunkIDs = useMemo(() => lines.filter((line) => line.isChunkStart).map((line) => line.id), [lines]);

  useEffect(() => {
    setActiveChunkIndex(0);
    chunkOffsetsRef.current = {};
  }, [diff]);

  if (!diff.trim() || rows.length === 0) {
    return <Text style={styles.empty}>{emptyText}</Text>;
  }

  const jumpToChunk = (nextIndex: number) => {
    if (chunkIDs.length === 0) return;
    const wrapped = (nextIndex + chunkIDs.length) % chunkIDs.length;
    setActiveChunkIndex(wrapped);
    const y = chunkOffsetsRef.current[chunkIDs[wrapped]];
    if (typeof y === "number") {
      verticalScrollRef.current?.scrollTo({ y: Math.max(0, y - 8), animated: true });
    }
  };

  const body = (
    <View style={[styles.body, !wrap && viewport ? { minWidth: viewport } : null]}>
      {lines.map((line) => {
        const recordLayout = line.isChunkStart
          ? (y: number) => {
              chunkOffsetsRef.current[line.id] = y;
            }
          : undefined;

        if (line.kind === "hunk") {
          return (
            <View
              key={line.id}
              onLayout={recordLayout ? (e) => recordLayout(e.nativeEvent.layout.y) : undefined}
            >
              <Text numberOfLines={1} style={styles.hunk}>
                {line.text}
              </Text>
            </View>
          );
        }

        return (
          <View
            key={line.id}
            onLayout={recordLayout ? (e) => recordLayout(e.nativeEvent.layout.y) : undefined}
          >
            <Line kind={line.kind} newNo={line.newNo} oldNo={line.oldNo} styles={styles} text={line.text} wrap={wrap} />
          </View>
        );
      })}
    </View>
  );

  const codeRegion = wrap ? (
    body
  ) : (
    <ScrollView horizontal onLayout={(event) => setViewport(event.nativeEvent.layout.width)} showsHorizontalScrollIndicator={false}>
      {body}
    </ScrollView>
  );

  return (
    <View style={[styles.viewer, standaloneScroll && styles.viewerFlex]}>
      <View style={styles.head}>
        <Text style={styles.add}>+{stats.added}</Text>
        <Text style={styles.del}>−{stats.removed}</Text>

        {standaloneScroll && chunkIDs.length > 0 ? (
          <View style={styles.chunkNav}>
            <Pressable
              accessibilityLabel="上一处差异"
              hitSlop={4}
              onPress={() => jumpToChunk(activeChunkIndex - 1)}
              style={({ pressed }) => [styles.chunkBtn, pressed && styles.pressed]}
            >
              <Feather color={palette.ink2} name="chevron-up" size={14} />
            </Pressable>
            <Text style={styles.chunkText}>{activeChunkIndex + 1}/{chunkIDs.length}</Text>
            <Pressable
              accessibilityLabel="下一处差异"
              hitSlop={4}
              onPress={() => jumpToChunk(activeChunkIndex + 1)}
              style={({ pressed }) => [styles.chunkBtn, pressed && styles.pressed]}
            >
              <Feather color={palette.ink2} name="chevron-down" size={14} />
            </Pressable>
          </View>
        ) : null}

        <View style={styles.flex} />

        <Pressable hitSlop={6} onPress={() => setWrap((value) => !value)} style={({ pressed }) => [styles.toggle, pressed && styles.pressed]}>
          <Feather color={palette.ink3} name={wrap ? "align-left" : "arrow-right"} size={13} />
          <Text style={styles.toggleText}>{wrap ? "自动换行" : "不换行"}</Text>
        </Pressable>
      </View>

      {standaloneScroll ? (
        <ScrollView ref={verticalScrollRef} showsVerticalScrollIndicator style={styles.verticalScroll}>
          {codeRegion}
        </ScrollView>
      ) : (
        codeRegion
      )}
    </View>
  );
}

// parseDiffRows 为左右分栏把删除和新增两两配对；单栏显示还原成「先删后增」的顺序，并标记每个连续改动块的起点用于上下跳转
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

  // 为每个从非变更转入 add/remove（或 hunk 头）的起点标注跳转锚点
  let inDiffCluster = false;
  for (const line of out) {
    const isChange = line.kind === "add" || line.kind === "remove";
    if (isChange && !inDiffCluster) {
      line.isChunkStart = true;
      inDiffCluster = true;
    } else if (!isChange) {
      inDiffCluster = false;
    }
  }
  return out;
}

function Line({
  kind,
  newNo,
  oldNo,
  styles,
  text,
  wrap,
}: {
  kind: DiffLineKind;
  newNo?: number;
  oldNo?: number;
  styles: ReturnType<typeof makeStyles>;
  text: string;
  wrap: boolean;
}) {
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

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    empty: { color: palette.ink2, fontSize: font.label, paddingVertical: 16 },
    viewer: { marginHorizontal: -16 },
    viewerFlex: { flex: 1, minHeight: 0 },
    head: {
      alignItems: "center",
      backgroundColor: palette.canvas,
      borderBottomColor: palette.hairline,
      borderBottomWidth: 1,
      flexDirection: "row",
      gap: 8,
      minHeight: 40,
      paddingHorizontal: 16,
    },
    add: { color: palette.diffAdd, fontFamily: fontFamily.mono, fontSize: font.caption, fontWeight: weight.semibold },
    del: { color: palette.diffDel, fontFamily: fontFamily.mono, fontSize: font.caption, fontWeight: weight.semibold },
    chunkNav: {
      alignItems: "center",
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 4,
      marginLeft: 6,
      paddingHorizontal: 6,
      paddingVertical: 2,
    },
    chunkBtn: { alignItems: "center", height: 24, justifyContent: "center", width: 24 },
    chunkText: { color: palette.ink2, fontFamily: fontFamily.mono, fontSize: 11 },
    flex: { flex: 1 },
    toggle: { alignItems: "center", borderRadius: radius.xs, flexDirection: "row", gap: 4, paddingHorizontal: 6, paddingVertical: 4 },
    pressed: { backgroundColor: palette.pressed },
    toggleText: { color: palette.ink3, fontSize: font.caption },
    verticalScroll: { flex: 1 },
    body: { paddingBottom: 16 },
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
