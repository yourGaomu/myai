import { Pressable, StyleSheet, Text, TextInput, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { ChatAttachment } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { attachmentKey, attachmentMeta, attachmentTitle, isUploadedAssetAttachment } from "../../utils/attachments";

type Props = {
  attachedFiles: ChatAttachment[];
  buttonFeedback: ButtonFeedback;
  canPause: boolean;
  messageInput: string;
  onChangeMessage: (value: string) => void;
  onPause: () => void;
  onRemoveAttachedFile: (key: string) => void;
  onSend: () => void;
  onUploadFile: () => void;
  pendingPause: boolean;
  pendingSend: boolean;
  pendingUpload: boolean;
};

export function Composer({
  attachedFiles,
  buttonFeedback,
  canPause,
  messageInput,
  onChangeMessage,
  onPause,
  onRemoveAttachedFile,
  onSend,
  onUploadFile,
  pendingPause,
  pendingSend,
  pendingUpload,
}: Props) {
  return (
    <View style={styles.composer}>
      {attachedFiles.length > 0 ? (
        <View style={styles.attachmentTray}>
          {attachedFiles.map((file) => (
            <View key={attachmentKey(file)} style={[styles.attachmentChip, isUploadedAssetAttachment(file) && styles.uploadedAttachmentChip]}>
              <View style={styles.flex}>
                <Text style={styles.attachmentTitle}>{attachmentTitle(file)}</Text>
                <Text numberOfLines={2} style={styles.attachmentMeta}>{attachmentMeta(file)}</Text>
              </View>
              <Pressable
                onPress={() => onRemoveAttachedFile(attachmentKey(file))}
                style={({ pressed }) => buttonFeedback(styles.attachmentRemove, pressed)}
              >
                <Text style={styles.attachmentRemoveText}>移除</Text>
              </Pressable>
            </View>
          ))}
        </View>
      ) : null}
      <View style={styles.composerInputRow}>
        <TextInput
          multiline
          onChangeText={onChangeMessage}
          placeholder="输入消息，@引用文件，/使用命令"
          placeholderTextColor="#776f66"
          style={styles.messageInput}
          value={messageInput}
        />
      </View>
      <View style={styles.composerActions}>
        <Pressable
          disabled={pendingUpload}
          onPress={onUploadFile}
          style={({ pressed }) => buttonFeedback([styles.uploadButton, pendingUpload && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingUpload} text={pendingUpload ? "上传中" : "文件"} />
        </Pressable>
        {!pendingSend ? (
          <Pressable
            onPress={onSend}
            style={({ pressed }) => buttonFeedback(styles.sendButton, pressed)}
          >
            <ButtonContent text="发送" />
          </Pressable>
        ) : null}
        {canPause ? (
          <Pressable
            disabled={pendingPause}
            onPress={onPause}
            style={({ pressed }) => buttonFeedback([styles.pauseButton, pendingPause && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingPause} text={pendingPause ? "暂停中" : "暂停"} />
          </Pressable>
        ) : null}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  composer: {
    gap: 7,
  },
  attachmentTray: {
    gap: 8,
  },
  attachmentChip: {
    alignItems: "center",
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  uploadedAttachmentChip: {
    backgroundColor: "#b9e9b0",
  },
  attachmentTitle: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  attachmentMeta: {
    color: "#6c665f",
    fontSize: 11,
    marginTop: 2,
  },
  attachmentRemove: {
    backgroundColor: "#ff7f68",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    paddingHorizontal: 9,
    paddingVertical: 6,
  },
  attachmentRemoveText: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  composerInputRow: {
    backgroundColor: "#fffdf7",
    borderColor: "#d7cfc2",
    borderRadius: 16,
    borderWidth: 1,
    elevation: 2,
    overflow: "hidden",
    shadowColor: "#171613",
    shadowOffset: { height: 2, width: 0 },
    shadowOpacity: 0.08,
    shadowRadius: 4,
    width: "100%",
  },
  composerActions: {
    alignItems: "center",
    flexDirection: "row",
    gap: 7,
    width: "100%",
  },
  messageInput: {
    backgroundColor: "transparent",
    color: "#12100e",
    flex: 1,
    minWidth: 0,
    includeFontPadding: false,
    lineHeight: 22,
    maxHeight: 120,
    minHeight: 58,
    paddingBottom: 8,
    paddingHorizontal: 12,
    paddingTop: 10,
    textAlignVertical: "top",
  },
  sendButton: {
    alignItems: "center",
    backgroundColor: "#ff7f68",
    borderColor: "#25231f",
    borderRadius: 12,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 42,
    minWidth: 78,
    paddingHorizontal: 14,
  },
  uploadButton: {
    alignItems: "center",
    backgroundColor: "#b9e9b0",
    borderColor: "#25231f",
    borderRadius: 12,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 42,
    minWidth: 78,
    paddingHorizontal: 12,
  },
  pauseButton: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#25231f",
    borderRadius: 12,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 42,
    minWidth: 78,
    paddingHorizontal: 12,
  },
  disabledButton: {
    opacity: 0.45,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
});
