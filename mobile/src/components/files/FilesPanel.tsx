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
    <View style={[styles.panel, styles.filesPanel]}>
      <View style={styles.assetSection}>
        <View style={styles.panelHeader}>
          <View style={styles.flex}>
            <Text style={styles.panelTitle}>Session assets</Text>
            <Text style={styles.pathText}>{assets.length === 0 ? "Shared files from this conversation" : `${assets.length} shared file(s)`}</Text>
          </View>
          <Pressable
            disabled={pendingAssets}
            onPress={onRefreshAssets}
            style={({ pressed }) => buttonFeedback([styles.smallButton, pendingAssets && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingAssets} text={pendingAssets ? "Loading" : "Refresh"} />
          </Pressable>
        </View>

        {assets.length === 0 ? (
          <Text style={styles.emptyText}>{clientToken ? "No shared assets yet" : "Pair first"}</Text>
        ) : (
          <View style={styles.assetList}>
            {assets.map((asset) => (
              <View key={asset.id || asset.short_url} style={styles.assetRow}>
                <View style={styles.assetBadge}>
                  <Text style={styles.assetBadgeText}>{assetKind(asset)}</Text>
                </View>
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.fileName}>
                    {asset.file_name || asset.path || "Shared file"}
                  </Text>
                  {asset.path ? (
                    <Text numberOfLines={1} style={styles.fileMeta}>
                      {asset.path}
                    </Text>
                  ) : null}
                  <Text style={styles.fileMeta}>
                    {[asset.size !== undefined ? formatBytes(asset.size) : "", asset.content_type || "", asset.expires_at ? `expires ${formatDateTime(asset.expires_at)}` : ""]
                      .filter(Boolean)
                      .join(" / ")}
                  </Text>
                </View>
                <Pressable
                  onPress={() => void Linking.openURL(asset.short_url)}
                  style={({ pressed }) => buttonFeedback(styles.assetOpenButton, pressed)}
                >
                  <Text style={styles.assetOpenButtonText}>Open</Text>
                </Pressable>
              </View>
            ))}
          </View>
        )}
      </View>

      <View style={styles.panelHeader}>
        <View style={styles.flex}>
          <Text style={styles.panelTitle}>Files</Text>
          <Text style={styles.pathText}>{filePath}</Text>
        </View>
        <View style={styles.rowCompact}>
          <Pressable
            disabled={!fileParent || pendingFiles}
            onPress={onGoToParent}
            style={({ pressed }) => buttonFeedback([styles.smallButton, (!fileParent || pendingFiles) && styles.disabledButton], pressed)}
          >
            <Text style={styles.smallButtonText}>Up</Text>
          </Pressable>
          <Pressable
            disabled={pendingFiles}
            onPress={onRefresh}
            style={({ pressed }) => buttonFeedback([styles.smallButton, pendingFiles && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingFiles} text={pendingFiles ? "Loading" : "Refresh"} />
          </Pressable>
        </View>
      </View>

      <View style={styles.fileList}>
        {fileEntries.length === 0 ? (
          <Text style={styles.emptyText}>{clientToken ? "No files loaded" : "Pair first"}</Text>
        ) : (
          fileEntries.map((entry) => (
            <Pressable
              key={entry.path}
              disabled={pendingFiles}
              onPress={() => onOpenFileEntry(entry)}
              style={({ pressed }) => buttonFeedback([styles.fileRow, pendingFiles && styles.disabledButton], pressed)}
            >
              <Text style={styles.fileIcon}>{entry.type === "dir" ? "DIR" : "TXT"}</Text>
              <View style={styles.flex}>
                <Text style={styles.fileName}>{entry.name}</Text>
                <Text style={styles.fileMeta}>{entry.type === "dir" ? entry.path : `${formatBytes(entry.size || 0)} / ${entry.path}`}</Text>
              </View>
            </Pressable>
          ))
        )}
      </View>

      {filePreview ? (
        <View style={styles.previewBox}>
          <View style={styles.previewHeader}>
            <Text style={[styles.previewTitle, styles.flex]}>
              {filePreview.name} / {filePreview.language} / {formatBytes(filePreview.size)}
            </Text>
            <Pressable
              disabled={filePreview.binary || filePreviewAttached}
              onPress={onAttachFilePreview}
              style={({ pressed }) =>
                buttonFeedback([styles.previewButton, (filePreview.binary || filePreviewAttached) && styles.disabledButton], pressed)
              }
            >
              <Text style={styles.previewButtonText}>{filePreviewAttached ? "Attached" : "Attach"}</Text>
            </Pressable>
          </View>
          {filePreview.binary ? (
            <Text style={styles.emptyText}>Binary file preview is not available.</Text>
          ) : (
            <ScrollView horizontal>
              <Text style={styles.codeText}>
                {filePreview.content || ""}
                {filePreview.truncated ? "\n\n[truncated]" : ""}
              </Text>
            </ScrollView>
          )}
        </View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  panel: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    elevation: 2,
    gap: 10,
    padding: 12,
    shadowColor: "#12100e",
    shadowOffset: { width: 4, height: 4 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
  },
  filesPanel: {
    minHeight: 280,
  },
  assetSection: {
    borderBottomColor: "#12100e",
    borderBottomWidth: 3,
    gap: 10,
    paddingBottom: 12,
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
    fontSize: 12,
    fontWeight: "700",
    marginTop: 3,
  },
  rowCompact: {
    flexDirection: "row",
    gap: 8,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  smallButton: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  smallButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  disabledButton: {
    opacity: 0.45,
  },
  fileList: {
    gap: 8,
  },
  assetList: {
    gap: 8,
  },
  assetRow: {
    alignItems: "center",
    backgroundColor: "#e8f6ff",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  assetBadge: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    height: 36,
    justifyContent: "center",
    width: 44,
  },
  assetBadgeText: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  assetOpenButton: {
    backgroundColor: "#12100e",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  assetOpenButtonText: {
    color: "#fffaf0",
    fontSize: 12,
    fontWeight: "900",
  },
  fileRow: {
    alignItems: "center",
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  fileIcon: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
    width: 28,
  },
  fileName: {
    color: "#12100e",
    fontWeight: "900",
  },
  fileMeta: {
    color: "#6c665f",
    fontSize: 12,
    marginTop: 2,
  },
  previewBox: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 8,
    maxHeight: 360,
    padding: 12,
  },
  previewHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  previewTitle: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  previewButton: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  previewButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  codeText: {
    color: "#12100e",
    fontFamily: "monospace",
    fontSize: 12,
    lineHeight: 18,
  },
  emptyText: {
    color: "#6c665f",
  },
});

function assetKind(asset: AssetSummary) {
  const contentType = (asset.content_type || "").toLowerCase();
  if (contentType.startsWith("image/")) {
    return "IMG";
  }
  const name = `${asset.file_name || ""} ${asset.path || ""}`.toLowerCase();
  const extension = name.match(/\.([a-z0-9]+)(\s|$)/)?.[1];
  return extension ? extension.slice(0, 3).toUpperCase() : "FILE";
}
