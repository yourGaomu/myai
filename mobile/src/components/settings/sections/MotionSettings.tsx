import { useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Switch, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, radius, weight } from "../../../constants/theme";
import { useMotion } from "../../../hooks/useMotionConfig";
import { useAppTheme, useThemedStyles } from "../../../theme/ThemeProvider";
import {
  motionSlotSpecs,
  type MotionCropPosition,
  type MotionSlot,
} from "../../../utils/motionRegistry";
import { CharacterAvatar } from "../../common/CharacterAvatar";
import { ConfirmModal } from "../../common/ConfirmModal";

const previewStageButtons: Array<{ label: string; slot: MotionSlot }> = [
  { slot: "idle", label: "待机" },
  { slot: "thinking", label: "思考中" },
  { slot: "working", label: "执行中" },
  { slot: "subagent", label: "子代理" },
  { slot: "attention", label: "待确认" },
  { slot: "done", label: "完成" },
];

const cropChoices: Array<{ key: MotionCropPosition; label: string }> = [
  { key: "top", label: "偏上防切头" },
  { key: "center", label: "居中" },
  { key: "bottom", label: "偏下" },
];

function formatSizeKB(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

export function MotionSettings() {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const {
    busySlot,
    pickAndSaveSlot,
    resetAllSlots,
    resetSlot,
    settings,
    systemReduceMotion,
    updateMotionFlag,
    updateSlotAdjustments,
  } = useMotion();

  const [previewSlot, setPreviewSlot] = useState<MotionSlot>("thinking");
  const [expandedEditSlot, setExpandedEditSlot] = useState<MotionSlot | null>(null);
  const [feedback, setFeedback] = useState<{ error?: boolean; text: string } | null>(null);
  const [confirmResetAll, setConfirmResetAll] = useState(false);

  const customCount = Object.keys(settings.customSlots).length;

  const handleUpload = async (slot: MotionSlot, label: string) => {
    setFeedback(null);
    const result = await pickAndSaveSlot(slot);
    if (result.ok) {
      setPreviewSlot(slot);
      setExpandedEditSlot(slot);
      setFeedback({ text: `已保存「${label}」自定义动效到本地，即时生效。` });
    } else if (result.error) {
      setFeedback({ error: true, text: result.error });
    }
  };

  return (
    <View style={styles.root}>
      {/* 实时场景预览 Stage */}
      <View style={styles.previewStage}>
        <View style={styles.previewTopBar}>
          <View style={styles.previewBadge}>
            <View style={[styles.dot, { backgroundColor: palette.brandLight }]} />
            <Text style={styles.previewBadgeText}>
              当前预览：{previewStageButtons.find((item) => item.slot === previewSlot)?.label}
            </Text>
          </View>
          {settings.customSlots[previewSlot] ? (
            <Text style={styles.customTag}>已使用自定义 GIF</Text>
          ) : (
            <Text style={styles.defaultTag}>内置角色动图</Text>
          )}
        </View>

        <View style={styles.previewCanvas}>
          <View style={styles.previewIsland}>
            <CharacterAvatar height={84} size={84} slot={previewSlot} variant="full" />
            <View style={styles.previewStatusPill}>
              <View style={[styles.dot, { backgroundColor: palette.ok }]} />
              <Text style={styles.previewStatusText}>
                {previewStageButtons.find((item) => item.slot === previewSlot)?.label}
              </Text>
            </View>
          </View>
        </View>

        <View style={styles.stageTabs}>
          {previewStageButtons.map((item) => {
            const active = previewSlot === item.slot;
            return (
              <Pressable
                key={item.slot}
                onPress={() => setPreviewSlot(item.slot)}
                style={({ pressed }) => [styles.stageTab, active && styles.stageTabActive, pressed && styles.pressed]}
              >
                <Text style={[styles.stageTabText, active && styles.stageTabTextActive]}>{item.label}</Text>
              </Pressable>
            );
          })}
        </View>
      </View>

      {/* 总开关 */}
      <View style={styles.card}>
        <View style={styles.switchRow}>
          <View style={styles.switchCol}>
            <Text style={styles.cardTitle}>启用角色动态 GIF 演绎</Text>
            <Text style={styles.cardSub}>开启后在启动、思考、工具执行与悬浮顶栏实时播放角色动效</Text>
          </View>
          <Switch
            onValueChange={(value) => updateMotionFlag({ motionEnabled: value })}
            thumbColor={settings.motionEnabled ? palette.brand : palette.ink3}
            trackColor={{ false: palette.surface3, true: palette.brandWashStrong }}
            value={settings.motionEnabled}
          />
        </View>

        <View style={styles.divider} />

        <View style={styles.switchRow}>
          <View style={styles.switchCol}>
            <Text style={styles.switchTitle}>减弱动态效果</Text>
            <Text style={styles.cardSub}>
              {systemReduceMotion
                ? "检测到系统已开启减弱动态，当前自动优先使用静态形象"
                : "仅展示静态角色插画，节省电量与减少视觉干扰"}
            </Text>
          </View>
          <Switch
            onValueChange={(value) => updateMotionFlag({ reduceMotion: value })}
            thumbColor={settings.reduceMotion ? palette.brand : palette.ink3}
            trackColor={{ false: palette.surface3, true: palette.brandWashStrong }}
            value={settings.reduceMotion}
          />
        </View>
      </View>

      {feedback ? (
        <View style={[styles.feedbackBanner, feedback.error && styles.feedbackBannerError]}>
          <Feather color={feedback.error ? palette.err : palette.ok} name={feedback.error ? "alert-circle" : "check-circle"} size={15} />
          <Text style={[styles.feedbackText, { color: feedback.error ? palette.err : palette.ok }]}>{feedback.text}</Text>
        </View>
      ) : null}

      {/* 7 大场景槽位管理 */}
      <View style={styles.sectionHead}>
        <Text style={styles.sectionLabel}>按场景自定义上传 GIF（{customCount}/7 已定制）</Text>
        <Text style={styles.sectionHint}>支持你本地生成的透明 GIF / Animated WebP；未上传槽位将智能继承兜底</Text>
      </View>

      {motionSlotSpecs.map((spec) => {
        const custom = settings.customSlots[spec.key];
        const uploading = busySlot === spec.key;
        const editing = expandedEditSlot === spec.key && Boolean(custom);

        return (
          <View key={spec.key} style={[styles.slotCard, custom && styles.slotCardCustom]}>
            <View style={styles.slotTopRow}>
              <Pressable onPress={() => setPreviewSlot(spec.key)} style={styles.slotThumb}>
                <CharacterAvatar height={54} size={54} slot={spec.key} variant="full" />
              </Pressable>

              <View style={styles.slotMeta}>
                <View style={styles.slotTitleRow}>
                  <Text numberOfLines={1} style={styles.slotTitle}>{spec.label}</Text>
                  <View style={[styles.statusChip, custom ? styles.statusChipCustom : null]}>
                    <Text style={[styles.statusChipText, custom ? styles.statusChipTextCustom : null]}>
                      {custom ? "自定义" : "默认"}
                    </Text>
                  </View>
                </View>
                <Text style={styles.slotDesc}>{spec.description}</Text>
                <Text numberOfLines={1} style={styles.slotFileInfo}>
                  {custom ? `${custom.fileName} · ${formatSizeKB(custom.size)}` : spec.recommendedSize}
                </Text>
              </View>
            </View>

            <View style={styles.slotActions}>
              <Pressable
                accessibilityRole="button"
                disabled={Boolean(busySlot)}
                onPress={() => void handleUpload(spec.key, spec.label)}
                style={({ pressed }) => [styles.actionBtn, styles.uploadBtn, pressed && styles.pressed]}
              >
                {uploading ? (
                  <ActivityIndicator color={palette.inkInverse} size="small" />
                ) : (
                  <Feather color={palette.inkInverse} name="upload" size={14} />
                )}
                <Text style={styles.uploadBtnText}>{custom ? "替换 GIF" : "选择 GIF"}</Text>
              </Pressable>

              {custom ? (
                <>
                  <Pressable
                    onPress={() => setExpandedEditSlot(editing ? null : spec.key)}
                    style={({ pressed }) => [styles.actionBtn, styles.secondaryBtn, pressed && styles.pressed]}
                  >
                    <Feather color={palette.ink1} name="sliders" size={14} />
                    <Text style={styles.secondaryBtnText}>{editing ? "收起校准" : "微调位置"}</Text>
                  </Pressable>
                  <Pressable
                    accessibilityLabel={`恢复${spec.label}默认动效`}
                    onPress={() => {
                      resetSlot(spec.key);
                      setFeedback({ text: `已恢复「${spec.label}」为系统默认动效。` });
                    }}
                    style={({ pressed }) => [styles.iconActionBtn, pressed && styles.pressed]}
                  >
                    <Feather color={palette.err} name="trash-2" size={15} />
                  </Pressable>
                </>
              ) : null}
            </View>

            {editing && custom ? (
              <View style={styles.adjustPanel}>
                <View style={styles.adjustRow}>
                  <Text style={styles.adjustLabel}>缩放：{custom.scale.toFixed(2)}x</Text>
                  <View style={styles.stepBtns}>
                    <Pressable
                      onPress={() => updateSlotAdjustments(spec.key, { scale: custom.scale - 0.08 })}
                      style={({ pressed }) => [styles.stepBtn, pressed && styles.pressed]}
                    >
                      <Feather color={palette.ink1} name="minus" size={14} />
                    </Pressable>
                    <Pressable
                      onPress={() => updateSlotAdjustments(spec.key, { scale: 1 })}
                      style={({ pressed }) => [styles.stepResetBtn, pressed && styles.pressed]}
                    >
                      <Text style={styles.stepResetText}>1.0x</Text>
                    </Pressable>
                    <Pressable
                      onPress={() => updateSlotAdjustments(spec.key, { scale: custom.scale + 0.08 })}
                      style={({ pressed }) => [styles.stepBtn, pressed && styles.pressed]}
                    >
                      <Feather color={palette.ink1} name="plus" size={14} />
                    </Pressable>
                  </View>
                </View>

                <View style={styles.adjustRow}>
                  <Text style={styles.adjustLabel}>垂直偏移：{custom.offsetY} px</Text>
                  <View style={styles.stepBtns}>
                    <Pressable
                      onPress={() => updateSlotAdjustments(spec.key, { offsetY: custom.offsetY - 2 })}
                      style={({ pressed }) => [styles.stepBtn, pressed && styles.pressed]}
                    >
                      <Feather color={palette.ink1} name="arrow-up" size={14} />
                    </Pressable>
                    <Pressable
                      onPress={() => updateSlotAdjustments(spec.key, { offsetY: 0 })}
                      style={({ pressed }) => [styles.stepResetBtn, pressed && styles.pressed]}
                    >
                      <Text style={styles.stepResetText}>0</Text>
                    </Pressable>
                    <Pressable
                      onPress={() => updateSlotAdjustments(spec.key, { offsetY: custom.offsetY + 2 })}
                      style={({ pressed }) => [styles.stepBtn, pressed && styles.pressed]}
                    >
                      <Feather color={palette.ink1} name="arrow-down" size={14} />
                    </Pressable>
                  </View>
                </View>

                <View style={styles.cropRow}>
                  {cropChoices.map((choice) => {
                    const active = custom.cropPosition === choice.key;
                    return (
                      <Pressable
                        key={choice.key}
                        onPress={() => updateSlotAdjustments(spec.key, { cropPosition: choice.key })}
                        style={({ pressed }) => [styles.cropChip, active && styles.cropChipActive, pressed && styles.pressed]}
                      >
                        <Text style={[styles.cropChipText, active && styles.cropChipTextActive]}>{choice.label}</Text>
                      </Pressable>
                    );
                  })}
                </View>
              </View>
            ) : null}
          </View>
        );
      })}

      {customCount > 0 ? (
        <Pressable
          onPress={() => setConfirmResetAll(true)}
          style={({ pressed }) => [styles.resetAllBtn, pressed && styles.pressed]}
        >
          <Feather color={palette.err} name="rotate-ccw" size={15} />
          <Text style={styles.resetAllText}>清空并恢复全部默认角色动效</Text>
        </Pressable>
      ) : null}

      <ConfirmModal
        confirmLabel="恢复默认"
        danger
        icon="trash-2"
        message={`确定要清除已上传的 ${customCount} 个本地自定义 GIF，并全部恢复为系统内置角色动效吗？`}
        onCancel={() => setConfirmResetAll(false)}
        onConfirm={() => {
          resetAllSlots();
          setConfirmResetAll(false);
          setExpandedEditSlot(null);
          setFeedback({ text: "已清除全部自定义动效并恢复系统默认。" });
        }}
        title="恢复默认角色动效"
        visible={confirmResetAll}
      />
    </View>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    root: {
      gap: 12,
      paddingBottom: 24,
    },
    previewStage: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.lg,
      borderWidth: 1,
      gap: 12,
      padding: 14,
    },
    previewTopBar: {
      alignItems: "center",
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 6,
      justifyContent: "space-between",
    },
    previewBadge: {
      alignItems: "center",
      flexDirection: "row",
      flexShrink: 1,
      gap: 6,
      minWidth: 0,
    },
    previewBadgeText: {
      color: palette.ink1,
      flexShrink: 1,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    customTag: {
      color: palette.brandLight,
      fontSize: 11,
      fontWeight: weight.semibold,
    },
    defaultTag: {
      color: palette.ink3,
      fontSize: 11,
    },
    previewCanvas: {
      alignItems: "center",
      backgroundColor: palette.canvas,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      justifyContent: "center",
      minHeight: 132,
      paddingVertical: 14,
    },
    previewIsland: {
      alignItems: "center",
    },
    previewStatusPill: {
      alignItems: "center",
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 5,
      marginTop: -6,
      paddingHorizontal: 10,
      paddingVertical: 3,
    },
    previewStatusText: {
      color: palette.ink1,
      fontSize: 11,
      fontWeight: weight.semibold,
    },
    stageTabs: {
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 6,
    },
    stageTab: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      paddingHorizontal: 11,
      paddingVertical: 5,
    },
    stageTabActive: {
      backgroundColor: palette.brandWashStrong,
      borderColor: palette.brand,
    },
    stageTabText: {
      color: palette.ink2,
      fontSize: 12,
      fontWeight: weight.medium,
    },
    stageTabTextActive: {
      color: palette.brandLight,
      fontWeight: weight.semibold,
    },
    dot: {
      borderRadius: radius.pill,
      height: 7,
      width: 7,
    },
    card: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.lg,
      borderWidth: 1,
      gap: 12,
      padding: 14,
    },
    switchRow: {
      alignItems: "center",
      flexDirection: "row",
      gap: 12,
    },
    switchCol: {
      flex: 1,
      gap: 2,
    },
    cardTitle: {
      color: palette.ink1,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    switchTitle: {
      color: palette.ink1,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    cardSub: {
      color: palette.ink2,
      fontSize: font.caption,
      lineHeight: 17,
    },
    divider: {
      backgroundColor: palette.hairline,
      height: 1,
    },
    feedbackBanner: {
      alignItems: "center",
      backgroundColor: palette.okWash,
      borderColor: palette.okBorder,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 8,
      paddingHorizontal: 12,
      paddingVertical: 10,
    },
    feedbackBannerError: {
      backgroundColor: palette.errWash,
      borderColor: palette.errBorder,
    },
    feedbackText: {
      flex: 1,
      fontSize: font.caption,
      fontWeight: weight.medium,
    },
    sectionHead: {
      gap: 2,
      paddingHorizontal: 4,
      paddingTop: 4,
    },
    sectionLabel: {
      color: palette.ink1,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    sectionHint: {
      color: palette.ink3,
      fontSize: font.caption,
    },
    slotCard: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.lg,
      borderWidth: 1,
      gap: 10,
      padding: 13,
    },
    slotCardCustom: {
      borderColor: palette.brandDeep,
    },
    slotTopRow: {
      alignItems: "center",
      flexDirection: "row",
      gap: 12,
    },
    slotThumb: {
      alignItems: "center",
      backgroundColor: palette.canvas,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      height: 60,
      justifyContent: "center",
      overflow: "hidden",
      width: 60,
    },
    slotMeta: {
      flex: 1,
      gap: 3,
      minWidth: 0,
    },
    slotTitleRow: {
      alignItems: "center",
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 6,
    },
    slotTitle: {
      color: palette.ink1,
      flexShrink: 1,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    statusChip: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      paddingHorizontal: 7,
      paddingVertical: 1,
    },
    statusChipCustom: {
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
    },
    statusChipText: {
      color: palette.ink3,
      fontSize: 10.5,
      fontWeight: weight.semibold,
    },
    statusChipTextCustom: {
      color: palette.brandLight,
    },
    slotDesc: {
      color: palette.ink2,
      fontSize: font.caption,
      lineHeight: 17,
    },
    slotFileInfo: {
      color: palette.ink3,
      fontSize: 11,
    },
    slotActions: {
      alignItems: "center",
      flexDirection: "row",
      gap: 8,
    },
    actionBtn: {
      alignItems: "center",
      borderRadius: radius.md,
      flexDirection: "row",
      gap: 6,
      justifyContent: "center",
      minHeight: 36,
      paddingHorizontal: 12,
    },
    uploadBtn: {
      backgroundColor: palette.brand,
      flex: 1,
    },
    uploadBtnText: {
      color: palette.inkInverse,
      fontSize: font.label,
      fontWeight: weight.bold,
    },
    secondaryBtn: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderWidth: 1,
    },
    secondaryBtnText: {
      color: palette.ink1,
      fontSize: font.label,
      fontWeight: weight.medium,
    },
    iconActionBtn: {
      alignItems: "center",
      backgroundColor: palette.errWash,
      borderColor: palette.errBorder,
      borderRadius: radius.md,
      borderWidth: 1,
      height: 36,
      justifyContent: "center",
      width: 38,
    },
    adjustPanel: {
      backgroundColor: palette.canvas,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      gap: 10,
      padding: 10,
    },
    adjustRow: {
      alignItems: "center",
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 8,
      justifyContent: "space-between",
    },
    adjustLabel: {
      color: palette.ink2,
      flexShrink: 1,
      fontSize: font.caption,
      fontWeight: weight.semibold,
    },
    stepBtns: {
      alignItems: "center",
      flexDirection: "row",
      gap: 6,
    },
    stepBtn: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.sm,
      borderWidth: 1,
      height: 30,
      justifyContent: "center",
      width: 32,
    },
    stepResetBtn: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.sm,
      borderWidth: 1,
      height: 30,
      justifyContent: "center",
      paddingHorizontal: 9,
    },
    stepResetText: {
      color: palette.ink2,
      fontSize: 11.5,
      fontWeight: weight.semibold,
    },
    cropRow: {
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 6,
    },
    cropChip: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.sm,
      borderWidth: 1,
      flexGrow: 1,
      minWidth: 84,
      paddingHorizontal: 8,
      paddingVertical: 6,
    },
    cropChipActive: {
      backgroundColor: palette.brandWash,
      borderColor: palette.brand,
    },
    cropChipText: {
      color: palette.ink2,
      fontSize: 11.5,
      fontWeight: weight.medium,
    },
    cropChipTextActive: {
      color: palette.brandLight,
      fontWeight: weight.semibold,
    },
    resetAllBtn: {
      alignItems: "center",
      backgroundColor: palette.errWash,
      borderColor: palette.errBorder,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 8,
      justifyContent: "center",
      minHeight: 44,
    },
    resetAllText: {
      color: palette.err,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    pressed: {
      opacity: 0.78,
    },
  });
