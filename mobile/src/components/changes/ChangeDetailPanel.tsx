import { ActivityIndicator, ScrollView, StyleSheet, Text, View } from "react-native";

import type { ChangeDiffResultPayload, HistoryDiffResultPayload } from "../../protocol";
import { shortID } from "../../utils/ids";
import { baseName, dirName } from "../../utils/toolDisplay";
import { fontFamily, font, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { useListStyles } from "../common/listStyles";
import { DiffViewer } from "./DiffViewer";

type Props = {
  changeDiff: ChangeDiffResultPayload | null;
  historyDiff: HistoryDiffResultPayload | null;
};

// 变更详情：单文件差异带吸顶 Hunk 上下跳转；历史快照支持连续多文件对比。
export function ChangeDetailPanel({ changeDiff, historyDiff }: Props) {
  const { palette } = useAppTheme();
  const list = useListStyles();
  const styles = useThemedStyles(makeStyles);

  if (changeDiff) {
    return (
      <View style={styles.singleRoot}>
        <Text numberOfLines={2} style={styles.path}>
          {changeDiff.path}
          {changeDiff.truncated ? " · 已截断" : ""}
        </Text>
        {changeDiff.binary ? (
          <Text style={list.empty}>{changeDiff.message || "二进制文件暂不支持差异查看。"}</Text>
        ) : (
          <DiffViewer diff={changeDiff.diff || ""} emptyText={changeDiff.message || "没有可显示的差异。"} standaloneScroll />
        )}
      </View>
    );
  }

  if (historyDiff) {
    const files = historyDiff.files || [];
    return (
      <ScrollView showsVerticalScrollIndicator={false} style={styles.historyScroll}>
        <Text style={list.hint}>
          快照 {shortID(historyDiff.checkpoint_id)} · {files.length} 个文件
          {historyDiff.message && files.length > 0 ? ` · ${historyDiff.message}` : ""}
        </Text>
        {files.length === 0 ? <Text style={list.empty}>{historyDiff.message || "没有可显示的差异。"}</Text> : null}
        {files.map((file) => {
          const dir = dirName(file.path);
          return (
            <View key={`${historyDiff.checkpoint_id}-${file.path}`} style={styles.file}>
              <View style={styles.fileHead}>
                <Text numberOfLines={1} style={styles.fileName}>
                  {baseName(file.path)}
                  {dir ? <Text style={list.meta}>  {dir}</Text> : null}
                </Text>
                <Text style={list.meta}>
                  {file.change_type || "已变更"}
                  {file.truncated ? " · 已截断" : ""}
                </Text>
              </View>
              {file.binary ? (
                <Text style={list.hint}>{file.message || "二进制文件暂不支持差异查看。"}</Text>
              ) : (
                <DiffViewer diff={file.diff || ""} emptyText={file.message || "没有可显示的差异。"} />
              )}
            </View>
          );
        })}
      </ScrollView>
    );
  }

  return (
    <View style={list.loading}>
      <ActivityIndicator color={palette.ink2} size="small" />
      <Text style={list.loadingText}>正在加载差异…</Text>
    </View>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    singleRoot: { flex: 1, minHeight: 0, paddingHorizontal: 16 },
    historyScroll: { flex: 1, paddingHorizontal: 16 },
    path: { color: palette.ink3, fontFamily: fontFamily.mono, fontSize: font.caption, lineHeight: 18, paddingBottom: 6, paddingTop: 4 },
    file: { borderTopColor: palette.hairline, borderTopWidth: 1, marginTop: 4, paddingTop: 12 },
    fileHead: { alignItems: "center", flexDirection: "row", gap: 10 },
    fileName: { color: palette.ink1, flex: 1, fontSize: font.body, fontWeight: weight.medium },
  });
