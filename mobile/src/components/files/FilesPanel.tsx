import { Linking, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { AssetSummary, FileEntry, FileReadResultPayload } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { formatBytes, formatDateTime } from "../../utils/format";

type Props = {
  buttonFeedback: ButtonFeedback;
  assets: AssetSummary[];
  clientToken: string;
  fileEntries: FileEntry[];
  fileParent: string;
  filePath: string;
  filePreview: FileReadResultPayload | null;
  filePreviewAttached: boolean;
  onAttachFilePreview: () => void;
  onGoToParent: () => void;
  onOpenFileEntry: (entry: FileEntry) => void;
  onRefreshAssets: () => void;
  onRefresh: () => void;
  pendingAssets: boolean;
  pendingFiles: boolean;
};

export function FilesPanel({
  buttonFeedback,
  assets,
  clientToken,
  fileEntries,
  fileParent,
  filePath,
  filePreview,
  filePreviewAttached,
  onAttachFilePreview,
  onGoToParent,
  onOpenFileEntry,
  onRefreshAssets,
  onRefresh,
  pendingAssets,
  pendingFiles,
}: Props) {
  return (
    <View style={styles.container}>
      {/* 1. 会话资源卡片 */}
      <View style={styles.panel}>
        <View style={styles.panelHeader}>
          <View style={styles.flex}>
            <Text style={styles.panelTitle}>会话资源</Text>
            <Text style={styles.pathText}>{assets.length === 0 ? "当前对话中的共享文件" : `${assets.length} 个共享资产`}</Text>
          </View>
          <Pressable
            disabled={pendingAssets}
            onPress={onRefreshAssets}
            style={({ pressed }) => buttonFeedback([styles.primaryButton, pendingAssets && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingAssets} text={pendingAssets ? "加载中" : "刷新"} />
          </Pressable>
        </View>

        {assets.length === 0 ? (
          <Text style={styles.emptyText}>{clientToken ? "还没有共享资源" : "请先完成配对"}</Text>
        ) : (
          <View style={styles.assetList}>
            {assets.map((asset) => (
              <View key={asset.id || asset.short_url} style={styles.assetRow}>
                <Text style={styles.assetEmoji}>📦</Text>
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.fileName}>
                    {asset.file_name || asset.path || "Shared file"}
                  </Text>
                  <Text numberOfLines={1} style={styles.fileMeta}>
                    {[asset.path || "", asset.size !== undefined ? formatBytes(asset.size) : ""]
                      .filter(Boolean)
                      .join(" · ")}
                  </Text>
                </View>
                <Pressable
                  onPress={() => void Linking.openURL(asset.short_url)}
                  style={({ pressed }) => buttonFeedback(styles.actionButton, pressed)}
                >
                  <Text style={styles.actionButtonText}>打开</Text>
                </Pressable>
              </View>
            ))}
          </View>
        )}
      </View>

      {/* 2. 工作区文件卡片 */}
      <View style={styles.panel}>
        <View style={styles.panelHeader}>
          <View style={styles.flex}>
            <Text style={styles.panelTitle}>工作区文件</Text>
            <Text style={styles.pathText}>PC Agent Workspace</Text>
          </View>
          <Pressable
            disabled={pendingFiles}
            onPress={onRefresh}
            style={({ pressed }) => buttonFeedback([styles.actionButton, pendingFiles && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingFiles} text={pendingFiles ? "加载中" : "刷新目录"} />
          </Pressable>
        </View>

        {/* 路径面包屑导航栏 */}
        <View style={styles.breadcrumbBar}>
          <Text numberOfLines={1} style={[styles.breadcrumbText, styles.flex]}>
            📁 {filePath || "工作区根目录"}
          </Text>
          <Pressable
            disabled={!fileParent || pendingFiles}
            onPress={onGoToParent}
            style={({ pressed }) => buttonFeedback([styles.smallParentButton, (!fileParent || pendingFiles) && styles.disabledButton], pressed)}
          >
            <Text style={styles.smallParentButtonText}>上一级</Text>
          </Pressable>
        </View>

        {/* 文件列表 */}
        <View style={styles.fileList}>
          {fileEntries.length === 0 ? (
            <Text style={styles.emptyText}>{clientToken ? "还没有加载文件" : "请先完成配对"}</Text>
          ) : (
            fileEntries.map((entry) => (
              <Pressable
                key={entry.path}
                disabled={pendingFiles}
                onPress={() => onOpenFileEntry(entry)}
                style={({ pressed }) => buttonFeedback([styles.fileRow, pendingFiles && styles.disabledButton], pressed)}
              >
                <Text style={styles.fileEmoji}>{entry.type === "dir" ? "📁" : "📄"}</Text>
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.fileName}>{entry.name}</Text>
                </View>
                <Text style={styles.fileSizeText}>{entry.type === "dir" ? "目录" : formatBytes(entry.size || 0)}</Text>
              </Pressable>
            ))
          )}
        </View>
      </View>

      {/* 3. 代码详情预览卡片 */}
      {filePreview ? (
        <View style={styles.panel}>
          <View style={styles.panelHeader}>
            <View style={styles.flex}>
              <Text numberOfLines={1} style={styles.panelTitle}>
                📄 {filePreview.name}
              </Text>
              <Text numberOfLines={1} style={styles.pathText}>
                {filePreview.path}
              </Text>
            </View>
            <Pressable
              disabled={filePreview.binary || filePreviewAttached}
              onPress={onAttachFilePreview}
              style={({ pressed }) =>
                buttonFeedback([styles.primaryButton, (filePreview.binary || filePreviewAttached) && styles.disabledButton], pressed)
              }
            >
              <Text style={styles.primaryButtonText}>{filePreviewAttached ? "已附加" : "附加到对话"}</Text>
            </Pressable>
          </View>
          {filePreview.binary ? (
            <Text style={styles.emptyText}>二进制文件暂不支持预览。</Text>
          ) : (
            <View style={styles.darkCodeContainer}>
              <ScrollView horizontal showsHorizontalScrollIndicator={true}>
                <Text selectable style={styles.darkCodeText}>
                  {filePreview.content || ""}
                  {filePreview.truncated ? "\n\n[truncated]" : ""}
                </Text>
              </ScrollView>
            </View>
          )}
        </View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    gap: 12,
  },
  panel: {
    backgroundColor: "#fffdf7",
    borderColor: "#25231f",
    borderRadius: 16,
    borderWidth: 2,
    elevation: 3,
    gap: 10,
    padding: 12,
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 3 },
    shadowOpacity: 0.1,
    shadowRadius: 2,
  },
  panelHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
    justifyContent: "space-between",
  },
  panelTitle: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  pathText: {
    color: "#6c665f",
    fontSize: 11.5,
    fontWeight: "700",
    marginTop: 2,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  primaryButton: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    justifyContent: "center",
    minHeight: 34,
    paddingHorizontal: 12,
    paddingVertical: 6,
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.12,
    shadowRadius: 1,
  },
  primaryButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  actionButton: {
    alignItems: "center",
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    justifyContent: "center",
    minHeight: 32,
    paddingHorizontal: 10,
    paddingVertical: 5,
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.1,
    shadowRadius: 1,
  },
  actionButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  disabledButton: {
    opacity: 0.45,
  },
  breadcrumbBar: {
    alignItems: "center",
    backgroundColor: "#f8f1e5",
    borderColor: "#d7cfc2",
    borderRadius: 8,
    borderWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  breadcrumbText: {
    color: "#4a453e",
    fontFamily: "monospace",
    fontSize: 12,
    fontWeight: "700",
  },
  smallParentButton: {
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 1.5,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  smallParentButtonText: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  assetList: {
    gap: 8,
  },
  assetRow: {
    alignItems: "center",
    backgroundColor: "#f8f1e5",
    borderColor: "#d7cfc2",
    borderRadius: 10,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 9,
  },
  assetEmoji: {
    fontSize: 16,
  },
  fileRow: {
    alignItems: "center",
    borderBottomColor: "#eee8df",
    borderBottomWidth: 1,
    flexDirection: "row",
    gap: 8,
    minHeight: 38,
    paddingHorizontal: 4,
    paddingVertical: 6,
  },
  fileEmoji: {
    fontSize: 15,
  },
  fileName: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "800",
  },
  fileMeta: {
    color: "#6c665f",
    fontSize: 11,
    marginTop: 2,
  },
  fileSizeText: {
    color: "#777066",
    fontSize: 11,
    fontWeight: "700",
  },
  fileList: {
    maxHeight: 280,
  },
  darkCodeContainer: {
    backgroundColor: "#1e1e1e",
    borderColor: "#25231f",
    borderRadius: 10,
    borderWidth: 1.5,
    maxHeight: 220,
    overflow: "hidden",
    padding: 10,
  },
  darkCodeText: {
    color: "#f7f5f0",
    fontFamily: "monospace",
    fontSize: 11.5,
    lineHeight: 17,
  },
  emptyText: {
    color: "#6c665f",
    fontSize: 12,
    paddingVertical: 8,
    textAlign: "center",
  },
});

function assetKind(asset: AssetSummary) {
  const contentType = (asset.content_type || "").toLowerCase();
  if (contentType.startsWith("image/")) {
    return "图片";
  }
  const name = `${asset.file_name || ""} ${asset.path || ""}`.toLowerCase();
  const extension = name.match(/\.([a-z0-9]+)(\s|$)/)?.[1];
  return extension ? extension.slice(0, 3).toUpperCase() : "文件";
}
