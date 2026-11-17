import { useState } from "react";
import { ActivityIndicator, Image, Platform, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { ChatAttachment } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { attachmentKey, attachmentTitle, isImageAttachment, isUploadedAssetAttachment } from "../../utils/attachments";
import { radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { BottomSheet } from "../common/BottomSheet";

type Props = {
  attachedFiles: ChatAttachment[];
  buttonFeedback: ButtonFeedback;
  canPause: boolean;
  messageInput: string;
  modelLabel: string;
  onBrowseKnowledge?: () => void;
  onBrowseWorkspace: () => void;
  onChangeMessage: (value: string) => void;
  onFocus?: () => void;
  onOpenPermission?: () => void;
  onOpenSession: () => void;
  onPause: () => void;
  onRemoveAttachedFile: (key: string) => void;
  onSend: () => void;
  onTogglePlan: () => void;
  onUploadFile: (kind?: "all" | "image") => void;
  pendingPause: boolean;
  pendingSend: boolean;
  pendingUpload: boolean;
  permissionSummary?: string;
  planMode: boolean;
  planPending: boolean;
};

// 输入区：Plan 模式微光外框 + 待处理授权警示条 + 多模态 4 项附件面板（文件/图片/工作区/知识库）。
export function Composer(props: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const { attachedFiles, canPause, messageInput, pendingPause, pendingSend, pendingUpload, planMode } = props;
  const [focused, setFocused] = useState(false);
  const [attachOpen, setAttachOpen] = useState(false);
  const busy = pendingSend || canPause || pendingPause;
  const showStop = canPause || pendingPause;
  const ready = !busy && (messageInput.trim() !== "" || attachedFiles.length > 0);
  const placeholder = busy
    ? "MyAI 正在执行…"
    : planMode
      ? "描述目标，MyAI 会先规划再请求你执行"
      : "输入需求，MyAI 会在电脑上执行";

  const attachOptions = [
    {
      icon: "file-plus" as const,
      label: "手机本地文件",
      sub: "选择文档、压缩包或日志并上传到资源库",
      onPress: () => props.onUploadFile("all"),
    },
    {
      icon: "image" as const,
      label: "手机截图与图片",
      sub: "选择报错截图或设计图，附带发送到当前对话",
      onPress: () => props.onUploadFile("image"),
    },
    {
      icon: "folder" as const,
      label: "工作区文件",
      sub: "浏览电脑项目目录并附加源码片段",
      onPress: props.onBrowseWorkspace,
    },
    ...(props.onBrowseKnowledge
      ? [
          {
            icon: "book-open" as const,
            label: "引用知识库 / 记忆",
            sub: "检索资料库片段或 AI 长期记忆并插入提示词",
            onPress: props.onBrowseKnowledge,
          },
        ]
      : []),
  ];

  return (
    <View style={styles.wrap}>
      {props.permissionSummary ? (
        <Pressable accessibilityRole="button" onPress={props.onOpenPermission} style={({ pressed }) => [styles.pill, pressed && styles.pressed]}>
          <View style={styles.pillDot} />
          <Feather color={palette.warn} name="shield" size={15} />
          <Text numberOfLines={1} style={styles.pillText}>待处理授权：{props.permissionSummary}</Text>
          <Text style={styles.pillLink}>点击查看</Text>
        </Pressable>
      ) : null}

      <View style={[styles.box, planMode && styles.boxPlan, focused && styles.boxFocused]}>
        {planMode ? (
          <View style={styles.planBanner}>
            <Feather color={palette.brandLight} name="compass" size={12} />
            <Text style={styles.planBannerText}>规划模式已开启 · 先输出方案步骤供你确认</Text>
          </View>
        ) : null}

        {attachedFiles.length > 0 || pendingUpload ? (
          <ScrollView contentContainerStyle={styles.tray} horizontal keyboardShouldPersistTaps="handled" showsHorizontalScrollIndicator={false}>
            {attachedFiles.map((file) => {
              const imageUri = isUploadedAssetAttachment(file) ? file.local_uri || file.short_url : undefined;
              return (
                <View key={attachmentKey(file)} style={styles.chip}>
                  {isImageAttachment(file) && imageUri ? (
                    <Image resizeMode="cover" source={{ uri: imageUri }} style={styles.thumb} />
                  ) : (
                    <Feather color={palette.ink2} name="file-text" size={14} />
                  )}
                  <Text numberOfLines={1} style={styles.chipText}>{attachmentTitle(file)}</Text>
                  <Pressable accessibilityLabel="移除附件" hitSlop={6} onPress={() => props.onRemoveAttachedFile(attachmentKey(file))} style={styles.chipRemove}>
                    <Feather color={palette.ink3} name="x" size={14} />
                  </Pressable>
                </View>
              );
            })}
            {pendingUpload ? (
              <View style={styles.chip}>
                <ActivityIndicator color={palette.brand} size="small" />
                <Text style={styles.chipText}>正在上传…</Text>
              </View>
            ) : null}
          </ScrollView>
        ) : null}

        <TextInput
          accessibilityLabel="输入需求"
          multiline
          onBlur={() => setFocused(false)}
          onChangeText={props.onChangeMessage}
          onFocus={() => {
            setFocused(true);
            props.onFocus?.();
          }}
          onKeyPress={(e: any) => {
            if (Platform.OS === "web" && e.nativeEvent.key === "Enter" && !e.nativeEvent.shiftKey) {
              e.preventDefault();
              if (ready) props.onSend();
            }
          }}
          placeholder={placeholder}
          placeholderTextColor={palette.ink4}
          style={styles.input}
          value={messageInput}
        />

        <View style={styles.toolbar}>
          <Pressable accessibilityLabel="添加附件" disabled={pendingUpload} onPress={() => setAttachOpen(true)} style={({ pressed }) => [styles.toolButton, pressed && styles.pressed]}>
            <Feather color={palette.ink2} name="plus" size={20} />
          </Pressable>
          <Pressable
            accessibilityLabel="规划模式"
            accessibilityState={{ selected: planMode, disabled: props.planPending }}
            disabled={props.planPending}
            onPress={props.onTogglePlan}
            style={({ pressed }) => [styles.chipButton, planMode && styles.chipButtonOn, pressed && styles.pressed, props.planPending && styles.dimmed]}
          >
            <Feather color={planMode ? palette.brandLight : palette.ink2} name="list" size={15} />
            <Text style={[styles.chipButtonText, planMode && styles.chipButtonTextOn]}>规划</Text>
          </Pressable>
          <Pressable accessibilityLabel="会话设置" onPress={props.onOpenSession} style={({ pressed }) => [styles.chipButton, styles.modelChip, pressed && styles.pressed]}>
            <Text numberOfLines={1} style={[styles.chipButtonText, styles.modelText]}>{props.modelLabel}</Text>
            <Feather color={palette.ink4} name="chevron-down" size={13} />
          </Pressable>
          <View style={styles.spacer} />
          {showStop ? (
            <Pressable accessibilityLabel="停止" disabled={pendingPause} onPress={props.onPause} style={({ pressed }) => [styles.send, styles.sendStop, pressed && styles.pressed]}>
              {pendingPause ? <ActivityIndicator color={palette.canvas} size="small" /> : <View style={styles.stopSquare} />}
            </Pressable>
          ) : (
            <Pressable accessibilityLabel="发送" disabled={!ready} onPress={props.onSend} style={({ pressed }) => [styles.send, ready && styles.sendReady, pressed && styles.pressed]}>
              <Feather color={ready ? palette.inkInverse : palette.ink4} name="arrow-up" size={19} />
            </Pressable>
          )}
        </View>
      </View>

      <BottomSheet accessibilityLabel="添加附件与上下文" onDismiss={() => setAttachOpen(false)} visible={attachOpen}>
        <Text style={styles.sheetTitle}>添加附件与上下文</Text>
        {attachOptions.map((item) => (
          <Pressable
            accessibilityRole="button"
            key={item.label}
            onPress={() => {
              setAttachOpen(false);
              item.onPress();
            }}
            style={({ pressed }) => [styles.sheetRow, pressed && styles.pressed]}
          >
            <View style={styles.sheetIcon}>
              <Feather color={palette.brandLight} name={item.icon} size={18} />
            </View>
            <View style={styles.sheetTextBlock}>
              <Text style={styles.sheetRowTitle}>{item.label}</Text>
              <Text style={styles.sheetRowSub}>{item.sub}</Text>
            </View>
            <Feather color={palette.ink4} name="chevron-right" size={16} />
          </Pressable>
        ))}
      </BottomSheet>
    </View>
  );
}

const makeStyles = ({ font, palette, shadow }: AppTheme) =>
  StyleSheet.create({
    wrap: { gap: 8, width: "100%" },
    pressed: { opacity: 0.75 },
    dimmed: { opacity: 0.5 },
    pill: {
      alignItems: "center",
      backgroundColor: palette.warnWash,
      borderColor: palette.warn,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 7,
      minHeight: 42,
      paddingHorizontal: 12,
      ...shadow.card,
    },
    pillDot: { backgroundColor: palette.warn, borderRadius: radius.pill, height: 6, width: 6 },
    pillText: { color: palette.ink1, flex: 1, fontSize: font.label, fontWeight: weight.medium },
    pillLink: { color: palette.warn, fontSize: font.label, fontWeight: weight.bold },
    box: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.xxl,
      borderWidth: 1,
      paddingBottom: 8,
      paddingLeft: 14,
      paddingRight: 8,
      paddingTop: 10,
      ...shadow.float,
    },
    boxPlan: {
      borderColor: palette.brandDeep,
    },
    boxFocused: { borderColor: palette.brand },
    planBanner: {
      alignItems: "center",
      flexDirection: "row",
      gap: 5,
      marginBottom: 6,
    },
    planBannerText: {
      color: palette.brandLight,
      fontSize: 11,
      fontWeight: weight.semibold,
    },
    tray: { gap: 6, paddingBottom: 8, paddingRight: 6 },
    chip: {
      alignItems: "center",
      backgroundColor: palette.well,
      borderRadius: radius.sm,
      flexDirection: "row",
      gap: 6,
      height: 32,
      paddingLeft: 10,
      paddingRight: 4,
    },
    thumb: { borderRadius: radius.xxs, height: 22, width: 22 },
    chipText: { color: palette.ink1, fontSize: font.caption, maxWidth: 160 },
    chipRemove: { alignItems: "center", height: 28, justifyContent: "center", width: 28 },
    input: {
      color: palette.ink1,
      fontSize: font.reading,
      includeFontPadding: false,
      lineHeight: Math.round(font.reading * 1.48),
      maxHeight: 120,
      minHeight: 26,
      paddingHorizontal: 0,
      paddingRight: 6,
      paddingVertical: Platform.OS === "ios" ? 2 : 0,
      textAlignVertical: "top",
    },
    toolbar: { alignItems: "center", flexDirection: "row", gap: 6, marginTop: 8 },
    toolButton: { alignItems: "center", borderRadius: radius.pill, height: 36, justifyContent: "center", width: 36 },
    chipButton: {
      alignItems: "center",
      borderColor: palette.hairlineStrong,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 5,
      height: 32,
      paddingHorizontal: 11,
    },
    chipButtonOn: { backgroundColor: palette.brandWash, borderColor: palette.brandDeep },
    chipButtonText: { color: palette.ink2, fontSize: font.caption + 0.5 },
    chipButtonTextOn: { color: palette.brandLight, fontWeight: weight.semibold },
    modelChip: { flexShrink: 1, minWidth: 0 },
    modelText: { flexShrink: 1 },
    spacer: { flex: 1 },
    send: { alignItems: "center", backgroundColor: palette.well, borderRadius: radius.pill, height: 38, justifyContent: "center", width: 38 },
    sendReady: { backgroundColor: palette.brand },
    sendStop: { backgroundColor: palette.ink1 },
    stopSquare: { backgroundColor: palette.canvas, borderRadius: 3, height: 12, width: 12 },
    sheetTitle: { color: palette.ink1, fontSize: font.title, fontWeight: weight.semibold, marginBottom: 8 },
    sheetRow: { alignItems: "center", borderRadius: radius.md, flexDirection: "row", gap: 12, minHeight: 62, paddingHorizontal: 4 },
    sheetIcon: { alignItems: "center", backgroundColor: palette.brandWash, borderRadius: radius.md - 2, height: 42, justifyContent: "center", width: 42 },
    sheetTextBlock: { flex: 1, gap: 2 },
    sheetRowTitle: { color: palette.ink1, fontSize: font.body, fontWeight: weight.medium },
    sheetRowSub: { color: palette.ink3, fontSize: font.caption },
  });
