import { Fragment, useState, type ComponentProps } from "react";
import { ActivityIndicator, Linking, Pressable, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { AssetSummary, FileEntry, FileReadResultPayload } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { formatBytes, formatShortTime } from "../../utils/format";
import { palette } from "../../constants/theme";
import { list } from "../common/listStyles";
import { Segmented } from "../common/Segmented";
import { FilePreview } from "./FilePreview";

type FeatherName = ComponentProps<typeof Feather>["name"];

type Props = {
  assets: AssetSummary[];
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  fileEntries: FileEntry[];
  fileParent: string;
  filePath: string;
  filePreview: FileReadResultPayload | null;
  onGoToParent: () => void;
  onOpenFileEntry: (entry: FileEntry) => void;
  pendingAssets: boolean;
  pendingFiles: boolean;
  // 预览由外层决定（顶栏返回要回到列表），这里只负责渲染
  showPreview: boolean;
};

// 文件页：工作区目录 / 会话资源两组，点文件进入预览。顶栏的刷新和返回由外层提供。
export function FilesPanel(props: Props) {
  const { assets, clientToken, fileEntries, fileParent, filePath, pendingFiles } = props;
  const [tab, setTab] = useState<"workspace" | "assets">("workspace");

  if (props.showPreview) {
    return <FilePreview loading={pendingFiles} preview={props.filePreview} />;
  }

  const segments = filePath.split(/[/\\]/).filter(Boolean);

  return (
    <View>
      <Segmented
        onChange={setTab}
        options={[{ key: "workspace", label: "工作区" }, { key: "assets", label: "会话资源" }]}
        value={tab}
      />

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
            {fileEntries.length === 0 ? (
              pendingFiles ? (
                <Loading text="正在加载目录…" />
              ) : (
                <Text style={list.empty}>{clientToken ? "这个目录是空的" : "请先完成配对"}</Text>
              )
            ) : (
              fileEntries.map((entry, index) => (
                <Fragment key={entry.path}>
                  {index > 0 || fileParent ? <View style={list.divider} /> : null}
                  <Row
                    accent={entry.type === "dir"}
                    disabled={pendingFiles}
                    icon={entry.type === "dir" ? "folder" : "file-text"}
                    onPress={() => props.onOpenFileEntry(entry)}
                    sub={[entry.type === "dir" ? "" : formatBytes(entry.size || 0), entry.modified_at ? formatShortTime(entry.modified_at) : ""].filter(Boolean).join(" · ")}
                    title={entry.name}
                  />
                </Fragment>
              ))
            )}
          </View>
        </>
      ) : (
        <>
          <Text style={list.hint}>本会话上传和生成的文件，点开用浏览器查看。</Text>
          <View style={list.bleed}>
            {assets.length === 0 ? (
              props.pendingAssets ? <Loading text="正在加载…" /> : <Text style={list.empty}>{clientToken ? "还没有会话资源" : "请先完成配对"}</Text>
            ) : (
              assets.map((asset, index) => (
                <Fragment key={asset.id || asset.short_url}>
                  {index > 0 ? <View style={list.divider} /> : null}
                  <Row
                    icon={(asset.content_type || "").startsWith("image/") ? "image" : "file"}
                    onPress={() => void Linking.openURL(asset.short_url)}
                    sub={[asset.size !== undefined ? formatBytes(asset.size) : "", asset.created_at ? formatShortTime(asset.created_at) : ""].filter(Boolean).join(" · ")}
                    title={asset.file_name || asset.path || "共享文件"}
                    trailing="external-link"
                  />
                </Fragment>
              ))
            )}
          </View>
        </>
      )}
    </View>
  );
}

function Row({ accent, disabled, icon, onPress, sub, title, trailing = "chevron-right" }: {
  accent?: boolean;
  disabled?: boolean;
  icon: FeatherName;
  onPress: () => void;
  sub?: string;
  title: string;
  trailing?: FeatherName;
}) {
  return (
    <Pressable disabled={disabled} onPress={onPress} style={({ pressed }) => [list.row, pressed && list.pressed]}>
      <View style={[list.iconBox, accent && list.iconBoxAccent]}>
        <Feather color={accent ? palette.brandLight : palette.ink2} name={icon} size={17} />
      </View>
      <View style={list.body}>
        <Text numberOfLines={1} style={list.title}>{title}</Text>
        {sub ? <Text numberOfLines={1} style={list.sub}>{sub}</Text> : null}
      </View>
      <Feather color={palette.ink4} name={trailing} size={16} />
    </Pressable>
  );
}

function Loading({ text }: { text: string }) {
  return (
    <View style={list.loading}>
      <ActivityIndicator color={palette.ink2} size="small" />
      <Text style={list.loadingText}>{text}</Text>
    </View>
  );
}
