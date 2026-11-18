import { Fragment, useMemo, useState, type ComponentProps } from "react";
import { ActivityIndicator, Linking, Pressable, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { AssetSummary, ChangeEntry, FileEntry, FileReadResultPayload } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { copyToClipboard } from "../../utils/clipboard";
import { formatBytes, formatShortTime } from "../../utils/format";
import { assetSummaryToSharedAsset } from "../../utils/toolAssets";
import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { useListStyles } from "../common/listStyles";
import { Segmented } from "../common/Segmented";
import { FilePreview } from "./FilePreview";

type FeatherName = ComponentProps<typeof Feather>["name"];

type Props = {
  assets: AssetSummary[];
  buttonFeedback: ButtonFeedback;
  changes?: ChangeEntry[];
  clientToken: string;
  fileEntries: FileEntry[];
  fileParent: string;
  filePath: string;
  filePreview: FileReadResultPayload | null;
  onGoToParent: () => void;
  onOpenFileEntry: (entry: FileEntry) => void;
  pendingAssets: boolean;
  pendingFiles: boolean;
  showPreview: boolean;
};

// 文件页：支持文件名检索、叠加工作区变更角标 (M/A)、长按复制路径、会话共享资源浏览。
export function FilesPanel(props: Props) {
  const { palette } = useAppTheme();
  const list = useListStyles();
  const styles = useThemedStyles(makeStyles);
  const { assets, changes = [], clientToken, fileEntries, fileParent, filePath, pendingFiles } = props;
  const [tab, setTab] = useState<"workspace" | "assets">("workspace");
  const [query, setQuery] = useState("");
  const [copiedPath, setCopiedPath] = useState("");

  const changeByPath = useMemo(() => {
    const map = new Map<string, ChangeEntry>();
    for (const c of changes) {
      map.set(c.path.replace(/\\/g, "/"), c);
    }
    return map;
  }, [changes]);

  const filteredEntries = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return fileEntries;
    return fileEntries.filter((entry) => entry.name.toLowerCase().includes(q) || entry.path.toLowerCase().includes(q));
  }, [fileEntries, query]);

  const filteredAssets = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return assets;
    return assets.filter((asset) =>
      `${asset.file_name || ""} ${asset.path || ""}`.toLowerCase().includes(q),
    );
  }, [assets, query]);

  if (props.showPreview) {
    return <FilePreview loading={pendingFiles} preview={props.filePreview} />;
  }

  const segments = filePath.split(/[/\\]/).filter(Boolean);

  const handleCopyPath = async (targetPath: string) => {
    if (await copyToClipboard(targetPath)) {
      setCopiedPath(targetPath);
      setTimeout(() => setCopiedPath(""), 1800);
    }
  };

  return (
    <View>
      <Segmented
        onChange={setTab}
        options={[{ key: "workspace", label: "工作区" }, { key: "assets", label: "会话资源" }]}
        value={tab}
      />

      <View style={styles.searchBox}>
        <Feather color={palette.ink3} name="search" size={15} />
        <TextInput
          onChangeText={setQuery}
          placeholder={tab === "workspace" ? "过滤当前目录文件名…" : "过滤会话资源名称…"}
          placeholderTextColor={palette.ink4}
          style={styles.searchInput}
          value={query}
        />
        {query ? (
          <Pressable hitSlop={6} onPress={() => setQuery("")}>
            <Feather color={palette.ink3} name="x" size={14} />
          </Pressable>
        ) : null}
      </View>

      {copiedPath ? <Text style={styles.copiedTip}>已复制路径：{copiedPath}</Text> : null}

      {tab === "workspace" ? (
        <>
          <View style={list.crumb}>
            <Feather color={palette.ink3} name="folder" size={13} />
            <Text numberOfLines={1} style={list.crumbText}>
              {segments.length === 0 ? <Text style={list.crumbLast}>工作区根目录</Text> : null}
              {segments.map((part, index) => (
                <Text key={`${part}-${index}`} style={index === segments.length - 1 ? list.crumbLast : null}>
                  {index > 0 ? " / " : ""}
                  {part}
                </Text>
              ))}
            </Text>
          </View>
          <View style={list.bleed}>
            {fileParent ? (
              <Row disabled={pendingFiles} icon="corner-left-up" onPress={props.onGoToParent} sub="上一级" title=".." />
            ) : null}
            {filteredEntries.length === 0 ? (
              pendingFiles ? (
                <Loading text="正在加载目录…" />
              ) : (
                <Text style={list.empty}>
                  {!clientToken ? "请先完成配对" : query ? "当前目录没有匹配项" : "这个目录是空的"}
                </Text>
              )
            ) : (
              filteredEntries.map((entry, index) => {
                const rel = entry.path.replace(/\\/g, "/");
                const exactChange = changeByPath.get(rel);
                const dirHasChanges =
                  entry.type === "dir" &&
                  changes.some((c) => c.path.replace(/\\/g, "/").startsWith(rel.endsWith("/") ? rel : `${rel}/`));
                const badgeText = exactChange
                  ? exactChange.untracked || exactChange.status === "added"
                    ? "新增"
                    : exactChange.deleted
                      ? "删除"
                      : "已修改"
                  : dirHasChanges
                    ? "有改动"
                    : undefined;
                const badgeTone =
                  exactChange?.untracked || exactChange?.status === "added"
                    ? "new"
                    : badgeText
                      ? "mod"
                      : undefined;

                return (
                  <Fragment key={entry.path}>
                    {index > 0 || fileParent ? <View style={list.divider} /> : null}
                    <Row
                      accent={entry.type === "dir"}
                      badge={badgeText}
                      badgeTone={badgeTone}
                      disabled={pendingFiles}
                      icon={entry.type === "dir" ? "folder" : "file-text"}
                      onLongPress={() => void handleCopyPath(entry.path)}
                      onPress={() => props.onOpenFileEntry(entry)}
                      sub={[
                        entry.type === "dir" ? "长按复制路径" : formatBytes(entry.size || 0),
                        entry.modified_at ? formatShortTime(entry.modified_at) : "",
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                      title={entry.name}
                    />
                  </Fragment>
                );
              })
            )}
          </View>
        </>
      ) : (
        <>
          <Text style={list.hint}>本会话上传和生成的文件；有分享链接时可点开，长按复制资源标识或链接。</Text>
          <View style={list.bleed}>
            {filteredAssets.length === 0 ? (
              props.pendingAssets ? (
                <Loading text="正在加载…" />
              ) : (
                <Text style={list.empty}>
                  {!clientToken ? "请先完成配对" : query ? "没有匹配的会话资源" : "还没有会话资源"}
                </Text>
              )
            ) : (
              filteredAssets.map((summary, index) => {
                const asset = assetSummaryToSharedAsset(summary);
                const copyValue = asset.shortURL || asset.assetID;
                return (
                  <Fragment key={summary.id || asset.assetID}>
                    {index > 0 ? <View style={list.divider} /> : null}
                    <Row
                      icon={(asset.contentType || "").startsWith("image/") ? "image" : "file"}
                      onLongPress={() => void handleCopyPath(copyValue)}
                      onPress={() => asset.shortURL ? void Linking.openURL(asset.shortURL) : void handleCopyPath(asset.assetID)}
                      sub={[
                        asset.size !== undefined ? formatBytes(asset.size) : "",
                        summary.created_at ? formatShortTime(summary.created_at) : "",
                        asset.shortURL ? "" : "已上传 · 长按复制资源标识",
                      ].filter(Boolean).join(" · ")}
                      title={asset.fileName || asset.path || "共享文件"}
                      trailing={asset.shortURL ? "external-link" : "file"}
                    />
                  </Fragment>
                );
              })
            )}
          </View>
        </>
      )}
    </View>
  );
}

function Row({
  accent,
  badge,
  badgeTone,
  disabled,
  icon,
  onLongPress,
  onPress,
  sub,
  title,
  trailing = "chevron-right",
}: {
  accent?: boolean;
  badge?: string;
  badgeTone?: "new" | "mod";
  disabled?: boolean;
  icon: FeatherName;
  onLongPress?: () => void;
  onPress: () => void;
  sub?: string;
  title: string;
  trailing?: FeatherName;
}) {
  const { palette } = useAppTheme();
  const list = useListStyles();

  return (
    <Pressable
      disabled={disabled}
      onLongPress={onLongPress}
      onPress={onPress}
      style={({ pressed }) => [list.row, pressed && list.pressed]}
    >
      <View style={[list.iconBox, accent && list.iconBoxAccent]}>
        <Feather color={accent ? palette.brandLight : palette.ink2} name={icon} size={17} />
      </View>
      <View style={list.body}>
        <Text numberOfLines={1} style={list.title}>{title}</Text>
        {sub ? <Text numberOfLines={1} style={list.sub}>{sub}</Text> : null}
      </View>
      {badge ? (
        <Text style={[list.tag, badgeTone === "new" ? list.tagNew : list.tagMod]}>{badge}</Text>
      ) : null}
      <Feather color={palette.ink4} name={trailing} size={16} />
    </Pressable>
  );
}

function Loading({ text }: { text: string }) {
  const { palette } = useAppTheme();
  const list = useListStyles();
  return (
    <View style={list.loading}>
      <ActivityIndicator color={palette.ink2} size="small" />
      <Text style={list.loadingText}>{text}</Text>
    </View>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    searchBox: {
      alignItems: "center",
      backgroundColor: palette.well,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 8,
      height: 38,
      marginBottom: 8,
      marginTop: 10,
      paddingHorizontal: 11,
    },
    searchInput: { color: palette.ink1, flex: 1, fontSize: font.label, padding: 0 },
    copiedTip: {
      color: palette.ok,
      fontSize: font.caption,
      fontWeight: weight.medium,
      marginBottom: 6,
    },
  });
