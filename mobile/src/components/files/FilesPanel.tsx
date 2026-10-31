import { Linking, Platform, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { ButtonContent } from "../common/ButtonContent";
import type { AssetSummary, FileEntry, FileReadResultPayload } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { formatBytes, formatDateTime } from "../../utils/format";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

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
                <Feather color={palette.brand} name="package" size={18} />
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
            <ButtonContent color={palette.ink1} loading={pendingFiles} text={pendingFiles ? "加载中" : "刷新目录"} />
          </Pressable>
        </View>

        {/* 路径面包屑导航栏 */}
        <View style={styles.breadcrumbBar}>
          <View style={[styles.iconRow, styles.flex]}>
            <Feather color={palette.brand} name="folder" size={16} />
            <Text numberOfLines={1} style={[styles.breadcrumbText, styles.flex]}>
              {filePath || "工作区根目录"}
            </Text>
          </View>
          <Pressable
            disabled={!fileParent || pendingFiles}
            onPress={onGoToParent}
            style={({ pressed }) => buttonFeedback([styles.smallParentButton, (!fileParent || pendingFiles) && styles.disabledButton], pressed)}
          >
            <Feather color={palette.inkInverse} name="chevron-left" size={15} />
            <Text style={styles.smallParentButtonText}>返回上一级</Text>
          </Pressable>
        </View>

        {/* 文件列表 */}
        <View style={styles.fileList}>
          {fileEntries.length === 0 ? (
            <Text style={styles.emptyText}>{clientToken ? "还没有加载文件" : "请先完成配对"}</Text>
          ) : (
            fileEntries.map((entry, index) => (
              <Pressable
                key={entry.path}
                disabled={pendingFiles}
                onPress={() => onOpenFileEntry(entry)}
                style={({ pressed }) =>
                  buttonFeedback(
                    [styles.fileRow, index === fileEntries.length - 1 && styles.fileRowLast, pendingFiles && styles.disabledButton],
                    pressed,
                  )
                }
              >
                <Feather
                  color={entry.type === "dir" ? palette.brand : palette.ink2}
                  name={entry.type === "dir" ? "folder" : "file-text"}
                  size={18}
                />
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.fileName}>{entry.name}</Text>
                </View>
                {entry.type === "dir" ? (
                  <View style={styles.dirRowRight}>
                    <Text style={styles.dirTag}>目录</Text>
                    <Feather color={palette.ink3} name="chevron-right" size={16} />
                  </View>
                ) : (
                  <Text style={styles.fileSizeText}>{formatBytes(entry.size || 0)}</Text>
                )}
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
              <View style={styles.iconRow}>
                <Feather color={palette.brand} name="file-text" size={16} />
                <Text numberOfLines={1} style={[styles.panelTitle, styles.flex]}>
                  {filePreview.name}
                </Text>
              </View>
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
              <ScrollView nestedScrollEnabled style={styles.darkCodeScroll}>
                <ScrollView horizontal showsHorizontalScrollIndicator={true}>
                  <Text selectable style={styles.darkCodeText}>
                    {filePreview.content || ""}
                    {filePreview.truncated ? "\n\n[truncated]" : ""}
                  </Text>
                </ScrollView>
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
  iconRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  panel: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.lg,
    borderWidth: 1,
    gap: 10,
    padding: 12,
    ...shadow.float,
  },
  panelHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
    justifyContent: "space-between",
  },
  panelTitle: {
    color: palette.ink1,
    fontSize: font.title,
    fontWeight: weight.bold,
  },
  pathText: {
    color: palette.ink2,
    fontSize: font.caption,
    fontWeight: weight.bold,
    marginTop: 2,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  primaryButton: {
    alignItems: "center",
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 34,
    paddingHorizontal: 12,
    paddingVertical: 6,
    ...shadow.float,
  },
  primaryButtonText: {
    color: palette.inkInverse,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  actionButton: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 32,
    paddingHorizontal: 10,
    paddingVertical: 5,
    ...shadow.float,
  },
  actionButtonText: {
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  disabledButton: {
    opacity: 0.45,
  },
  breadcrumbBar: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.md,
    borderWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 7,
    ...shadow.float,
  },
  breadcrumbText: {
    color: palette.ink1,
    fontFamily: "monospace",
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  smallParentButton: {
    alignItems: "center",
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 3,
    paddingHorizontal: 8,
    paddingVertical: 4,
    ...shadow.float,
  },
  smallParentButtonText: {
    color: palette.inkInverse,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  assetList: {
    gap: 8,
  },
  assetRow: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.md,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 10,
    ...shadow.float,
  },
  assetEmoji: {
    fontSize: font.title,
  },
  fileRow: {
    alignItems: "center",
    borderBottomColor: palette.hairline,
    borderBottomWidth: 1,
    flexDirection: "row",
    gap: 8,
    minHeight: 40,
    paddingHorizontal: 4,
    paddingVertical: 8,
  },
  fileRowLast: {
    borderBottomWidth: 0,
  },
  fileEmoji: {
    fontSize: font.bodyLg,
  },
  fileName: {
    color: palette.ink1,
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  fileMeta: {
    color: palette.ink2,
    fontSize: font.caption,
    marginTop: 2,
  },
  dirRowRight: {
    alignItems: "center",
    flexDirection: "row",
    gap: 4,
  },
  dirTag: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.xs,
    borderWidth: 1,
    color: palette.ink2,
    fontSize: font.micro,
    fontWeight: weight.bold,
    paddingHorizontal: 4,
    paddingVertical: 1,
  },
  dirChevron: {
    color: palette.ink3,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  fileSizeText: {
    color: palette.ink2,
    fontFamily: "monospace",
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  fileList: {
    gap: 0,
  },
  darkCodeContainer: {
    backgroundColor: palette.surface3,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.md,
    borderWidth: 1,
    overflow: "hidden",
    padding: 10,
  },
  darkCodeScroll: {
    maxHeight: 280,
  },
  darkCodeText: {
    color: palette.ink2,
    fontFamily: "monospace",
    fontSize: font.caption,
    lineHeight: 17,
  },
  emptyText: {
    color: palette.ink2,
    fontSize: font.label,
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
