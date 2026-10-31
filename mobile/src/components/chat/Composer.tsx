import { ActivityIndicator, Image, Platform, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";
import { BlurView } from "expo-blur";
import { LinearGradient } from "expo-linear-gradient";

import type { ChatAttachment } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { attachmentKey, attachmentTitle, isImageAttachment, isUploadedAssetAttachment } from "../../utils/attachments";
import { font, glass, palette, radius, shadow, space, weight } from "../../constants/theme";

type Props = {
  attachedFiles: ChatAttachment[];
  buttonFeedback: ButtonFeedback;
  canPause: boolean;
  messageInput: string;
  onChangeMessage: (value: string) => void;
  onFocus?: () => void;
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
  onFocus,
  onPause,
  onRemoveAttachedFile,
  onSend,
  onUploadFile,
  pendingPause,
  pendingSend,
  pendingUpload,
}: Props) {
  const isBusy = pendingSend || canPause || pendingPause;
  const showPauseButton = canPause || pendingPause;

  return (
    <View style={styles.composerWrapper}>
      {/* 悬浮附件预览条 / 暂留区 */}
      {attachedFiles.length > 0 || pendingUpload ? (
        <ScrollView
          contentContainerStyle={styles.attachmentTray}
          horizontal
          showsHorizontalScrollIndicator={false}
        >
          {attachedFiles.map((file) => {
            const isImg = isImageAttachment(file);
            const imageUri = isUploadedAssetAttachment(file) ? file.local_uri || file.short_url : undefined;
            return (
              <View
                key={attachmentKey(file)}
                style={[
                  styles.attachmentChip,
                  isUploadedAssetAttachment(file) && styles.uploadedAttachmentChip,
                  isImg && styles.imageAttachmentChip,
                ]}
              >
                {isImg && imageUri ? (
                  <Image
                    resizeMode="cover"
                    source={{ uri: imageUri }}
                    style={styles.attachmentThumbnail}
                  />
                ) : (
                  <Feather color={palette.ink3} name="file-text" size={14} />
                )}
                <View style={styles.attachmentInfo}>
                  <Text numberOfLines={1} style={styles.attachmentTitle}>
                    {attachmentTitle(file)}
                  </Text>
                  {isImg ? (
                    <Text style={styles.attachmentSubtext}>图片附件</Text>
                  ) : null}
                </View>
                <Pressable
                  accessibilityLabel="移除附件"
                  onPress={() => onRemoveAttachedFile(attachmentKey(file))}
                  style={({ pressed }) => buttonFeedback(styles.attachmentRemove, pressed)}
                >
                  <Feather color={palette.err} name="x" size={12} />
                </Pressable>
              </View>
            );
          })}
          {pendingUpload ? (
            <View style={[styles.attachmentChip, styles.uploadingChip]}>
              <ActivityIndicator color={palette.brand} size="small" />
              <Text style={styles.uploadingText}>正在上传文件...</Text>
            </View>
          ) : null}
        </ScrollView>
      ) : null}

      {/* 单行玻璃输入胶囊 */}
      <BlurView intensity={glass.intensity} style={styles.composerBox} tint="dark">
        {/* 左侧上传附件按钮：无背景无边框，只是一个图标 */}
        <Pressable
          accessibilityLabel="附加文件"
          disabled={pendingUpload}
          onPress={onUploadFile}
          style={({ pressed }) =>
            buttonFeedback([styles.iconButton, pendingUpload && styles.disabledBtn], pressed)
          }
        >
          {pendingUpload ? (
            <ActivityIndicator color={palette.brand} size="small" />
          ) : (
            <Feather color={palette.ink3} name="paperclip" size={19} />
          )}
        </Pressable>

        {/* 中间输入框 */}
        <TextInput
          accessibilityLabel="输入需求"
          multiline
          onChangeText={onChangeMessage}
          onFocus={onFocus}
          onKeyPress={(e: any) => {
            if (
              Platform.OS === "web" &&
              e.nativeEvent.key === "Enter" &&
              !e.nativeEvent.shiftKey
            ) {
              e.preventDefault();
              if (!isBusy && (messageInput.trim() || attachedFiles.length > 0)) {
                onSend();
              }
            }
          }}
          placeholder="输入需求，AI 将控制电脑执行…"
          placeholderTextColor={palette.ink4}
          style={styles.input}
          value={messageInput}
        />

        {/* 右侧动作按钮：空闲为箭头发送，执行中为方形暂停 */}
        {showPauseButton ? (
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
              <ActivityIndicator color={palette.err} size="small" />
            ) : (
              <Feather color={palette.err} name="square" size={13} />
            )}
          </Pressable>
        ) : (
          <Pressable
            accessibilityLabel="发送需求"
            disabled={isBusy}
            onPress={onSend}
            style={({ pressed }) => buttonFeedback([isBusy && styles.disabledBtn], pressed)}
          >
            <LinearGradient
              colors={[palette.brandLight, palette.brandDeep]}
              end={{ x: 1, y: 1 }}
              start={{ x: 0.15, y: 0 }}
              style={styles.sendBtn}
            >
              <Feather color={palette.inkInverse} name="arrow-up" size={15} strokeWidth={2.6} />
            </LinearGradient>
          </Pressable>
        )}
      </BlurView>
    </View>
  );
}

const styles = StyleSheet.create({
  composerWrapper: {
    gap: space.md,
    width: "100%",
  },
  attachmentTray: {
    flexDirection: "row",
    gap: space.sm,
    paddingBottom: 2,
  },
  attachmentChip: {
    alignItems: "center",
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 6,
    paddingHorizontal: space.md,
    paddingVertical: space.xs,
    ...shadow.card,
  },
  uploadedAttachmentChip: {
    backgroundColor: palette.okWash,
    borderColor: palette.okBorder,
  },
  imageAttachmentChip: {
    backgroundColor: palette.surface3,
    gap: 7,
    paddingLeft: space.xs,
  },
  attachmentThumbnail: {
    backgroundColor: palette.surface4,
    borderRadius: radius.xs,
    height: 28,
    width: 28,
  },
  attachmentInfo: {
    justifyContent: "center",
    maxWidth: 140,
  },
  attachmentSubtext: {
    color: palette.ok,
    fontSize: font.micro,
    fontWeight: weight.medium,
    marginTop: 1,
  },
  uploadingChip: {
    borderStyle: "dashed",
    paddingHorizontal: space.lg,
  },
  uploadingText: {
    color: palette.brand,
    fontSize: font.caption,
    fontWeight: weight.semibold,
    marginLeft: 3,
  },
  attachmentTitle: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.semibold,
    maxWidth: 160,
  },
  attachmentRemove: {
    paddingHorizontal: 2,
  },
  composerBox: {
    alignItems: "flex-end",
    backgroundColor: palette.glassTintStrong,
    borderColor: palette.glassEdge,
    borderRadius: radius.xl,
    borderWidth: 1,
    flexDirection: "row",
    gap: 7,
    overflow: "hidden",
    paddingBottom: 7,
    paddingLeft: 9,
    paddingRight: 7,
    paddingTop: 7,
    width: "100%",
    ...shadow.float,
  },
  iconButton: {
    alignItems: "center",
    height: 30,
    justifyContent: "center",
    width: 30,
  },
  input: {
    backgroundColor: palette.transparent,
    color: palette.ink1,
    flex: 1,
    fontSize: font.body,
    fontWeight: weight.regular,
    includeFontPadding: false,
    lineHeight: 20,
    maxHeight: 88,
    minHeight: 30,
    paddingHorizontal: 0,
    paddingVertical: Platform.OS === "ios" ? 6 : 3,
    textAlignVertical: "center",
  },
  actionBtn: {
    alignItems: "center",
    borderRadius: 15,
    height: 30,
    justifyContent: "center",
    width: 30,
  },
  sendBtn: {
    alignItems: "center",
    borderRadius: 15,
    height: 30,
    justifyContent: "center",
    width: 30,
    ...shadow.brandGlow,
  },
  pauseBtn: {
    backgroundColor: palette.errWash,
    borderColor: palette.errBorder,
    borderWidth: 1,
  },
  disabledBtn: {
    opacity: 0.5,
  },
});
