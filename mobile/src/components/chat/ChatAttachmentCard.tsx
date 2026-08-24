import { Text, View, type StyleProp, type ViewStyle } from "react-native";

import type { ChatAttachment } from "../../types/chat";
import { formatBytes } from "../../utils/format";
import { isUploadedAssetAttachment } from "../../utils/attachments";
import { SharedAssetCard } from "./SharedAssetCard";
import { styles } from "./styles";

type Props = {
  attachment: ChatAttachment;
  buttonFeedback: (style: StyleProp<ViewStyle>, active?: boolean) => StyleProp<ViewStyle>;
};

export function ChatAttachmentCard({ attachment, buttonFeedback }: Props) {
  if (isUploadedAssetAttachment(attachment)) {
    return (
      <SharedAssetCard
        asset={{
          code: attachment.code,
          contentType: attachment.content_type,
          expiresAt: attachment.expires_at,
          fileName: attachment.file_name || "已上传文件",
          shortURL: attachment.short_url,
          size: attachment.size,
        }}
        buttonFeedback={buttonFeedback}
        previewURL={attachment.local_uri}
      />
    );
  }

  const title = attachment.name || attachment.path || "工作区文件";
  const extension = title.includes(".") ? title.split(".").pop()?.slice(0, 4).toUpperCase() : "文件";

  return (
    <View style={styles.workspaceAttachmentCard}>
      <View style={styles.workspaceAttachmentIcon}>
        <Text style={styles.workspaceAttachmentIconText}>{extension || "文件"}</Text>
      </View>
      <View style={styles.flex}>
        <Text numberOfLines={1} style={styles.assetTitle}>{title}</Text>
        <Text numberOfLines={1} style={styles.assetPath}>{attachment.path}</Text>
        <Text style={styles.assetMeta}>{formatBytes(attachment.size)} / 已附加</Text>
      </View>
    </View>
  );
}
