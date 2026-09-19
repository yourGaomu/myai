import { ActivityIndicator, Platform, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

import type { ChatAttachment } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { attachmentKey, attachmentTitle, isUploadedAssetAttachment } from "../../utils/attachments";

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
  const isBusy = pendingSend || canPause || pendingPause;

  return (
    <View style={styles.composerWrapper}>
      {/* 悬浮附件预览条 */}
      {attachedFiles.length > 0 ? (
        <ScrollView
          contentContainerStyle={styles.attachmentTray}
          horizontal
          showsHorizontalScrollIndicator={false}
        >
          {attachedFiles.map((file) => (
            <View
              key={attachmentKey(file)}
              style={[
                styles.attachmentChip,
                isUploadedAssetAttachment(file) && styles.uploadedAttachmentChip,
              ]}
            >
              <Text style={styles.attachmentIcon}>📎</Text>
              <Text numberOfLines={1} style={styles.attachmentTitle}>
                {attachmentTitle(file)}
              </Text>
              <Pressable
                accessibilityLabel="移除附件"
                onPress={() => onRemoveAttachedFile(attachmentKey(file))}
                style={({ pressed }) =>
                  buttonFeedback(styles.attachmentRemove, pressed)
                }
              >
                <Text style={styles.attachmentRemoveText}>✕</Text>
              </Pressable>
            </View>
          ))}
        </ScrollView>
      ) : null}

      {/* 快捷任务提示条 (Agents-Anywhere Workbench Composer Layout) */}
      <View style={styles.composerHeaderRow}>
        <View style={styles.composerModeBadge}>
          <View style={styles.modeLiveDot} />
          <Text style={styles.modeText}>Agent 任务执行</Text>
        </View>
        {Platform.OS === "web" ? (
          <Text style={styles.composerHintText}>Enter 发送 · Shift+Enter 换行</Text>
        ) : null}
      </View>

      {/* 单行一体化输入胶囊 (Neo-Brutalism Composer Bar) */}
      <View style={styles.composerBox}>
        {/* 左侧上传附件按钮 */}
        <Pressable
          accessibilityLabel="附加文件"
          disabled={pendingUpload}
          onPress={onUploadFile}
          style={({ pressed }) =>
            buttonFeedback(
              [styles.uploadBtn, pendingUpload && styles.disabledBtn],
              pressed,
            )
          }
        >
          {pendingUpload ? (
            <ActivityIndicator color="#12100e" size="small" />
          ) : (
            <Text style={styles.uploadBtnIcon}>📎</Text>
          )}
        </Pressable>

        {/* 中间输入框 */}
        <TextInput
          accessibilityLabel="输入需求"
          multiline
          onChangeText={onChangeMessage}
          onKeyPress={(e: any) => {
            if (
              Platform.OS === "web" &&
              e.nativeEvent.key === "Enter" &&
              !e.nativeEvent.shiftKey
            ) {
              e.preventDefault();
              if (!isBusy && messageInput.trim()) {
                onSend();
              }
            }
          }}
          placeholder="输入需求，AI 将控制电脑执行..."
          placeholderTextColor="#8c857b"
          style={styles.input}
          value={messageInput}
        />

        {/* 右侧动作按钮：空闲为 ▶ 发送，执行中为 ■ 暂停 */}
        {isBusy ? (
          <Pressable
            accessibilityLabel="暂停任务"
            disabled={pendingPause}
            onPress={onPause}
            style={({ pressed }) =>
              buttonFeedback(
                [styles.actionBtn, styles.pauseBtn, pendingPause && styles.disabledBtn],
                pressed,
              )
            }
          >
            {pendingPause ? (
              <ActivityIndicator color="#ffffff" size="small" />
            ) : (
              <Text style={styles.pauseBtnText}>■</Text>
            )}
          </Pressable>
        ) : (
          <Pressable
            accessibilityLabel="发送需求"
            onPress={onSend}
            style={({ pressed }) =>
              buttonFeedback(
                [styles.actionBtn, styles.sendBtn],
                pressed,
              )
            }
          >
            <Text style={styles.sendBtnText}>▶</Text>
          </Pressable>
        )}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  composerWrapper: {
    gap: 4,
    width: "100%",
  },
  composerHeaderRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 4,
    paddingBottom: 2,
  },
  composerModeBadge: {
    alignItems: "center",
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 1.5,
    flexDirection: "row",
    gap: 5,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  modeLiveDot: {
    backgroundColor: "#2e8b38",
    borderRadius: 3,
    height: 6,
    width: 6,
  },
  modeText: {
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
  },
  composerHintText: {
    color: "#8c857b",
    fontSize: 10,
    fontWeight: "700",
  },
  attachmentTray: {
    flexDirection: "row",
    gap: 6,
    paddingBottom: 2,
  },
  attachmentChip: {
    alignItems: "center",
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 1.5,
    elevation: 2,
    flexDirection: "row",
    gap: 5,
    paddingHorizontal: 8,
    paddingVertical: 4,
    shadowColor: "#12100e",
    shadowOffset: { height: 1.5, width: 0 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  uploadedAttachmentChip: {
    backgroundColor: "#b9e9b0",
  },
  attachmentIcon: {
    fontSize: 11,
  },
  attachmentTitle: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "800",
    maxWidth: 160,
  },
  attachmentRemove: {
    paddingHorizontal: 2,
  },
  attachmentRemoveText: {
    color: "#d94b34",
    fontSize: 11,
    fontWeight: "900",
  },
  composerBox: {
    alignItems: "center",
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 14,
    borderWidth: 2,
    elevation: 3,
    flexDirection: "row",
    gap: 6,
    paddingHorizontal: 8,
    paddingVertical: 5,
    shadowColor: "#12100e",
    shadowOffset: { height: 2, width: 0 },
    shadowOpacity: 1,
    shadowRadius: 0,
    width: "100%",
  },
  uploadBtn: {
    alignItems: "center",
    backgroundColor: "#f8f1e5",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 1.5,
    height: 32,
    justifyContent: "center",
    width: 32,
  },
  uploadBtnIcon: {
    color: "#12100e",
    fontSize: 14,
  },
  input: {
    backgroundColor: "transparent",
    color: "#12100e",
    flex: 1,
    fontSize: 13,
    fontWeight: "700",
    includeFontPadding: false,
    lineHeight: 18,
    maxHeight: 72,
    minHeight: 32,
    paddingHorizontal: 4,
    paddingVertical: Platform.OS === "ios" ? 6 : 2,
    textAlignVertical: "center",
  },
  actionBtn: {
    alignItems: "center",
    borderRadius: 8,
    borderWidth: 1.5,
    height: 32,
    justifyContent: "center",
    width: 32,
  },
  sendBtn: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    shadowColor: "#12100e",
    shadowOffset: { height: 1.5, width: 0 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  sendBtnText: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
    marginLeft: 1,
  },
  pauseBtn: {
    backgroundColor: "#25231f",
    borderColor: "#12100e",
  },
  pauseBtnText: {
    color: "#ffffff",
    fontSize: 12,
    fontWeight: "900",
  },
  disabledBtn: {
    opacity: 0.5,
  },
});
