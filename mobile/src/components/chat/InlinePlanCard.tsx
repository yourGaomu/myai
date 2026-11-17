import { useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { Plan } from "../../protocol";
import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { Spinner } from "../common/Spinner";

type Props = {
  onExecutePlan: () => void;
  onOpenDetail: () => void;
  pendingExecute?: boolean;
  plan: Plan;
};

function statusMeta(status: string): { label: string; tone: "ok" | "busy" | "err" | "idle" } {
  const s = (status || "").toLowerCase();
  if (s === "completed" || s === "succeeded" || s === "done") return { label: "已完成", tone: "ok" };
  if (s === "running" || s === "executing" || s === "in_progress") return { label: "执行中", tone: "busy" };
  if (s === "failed" || s === "error") return { label: "执行受阻", tone: "err" };
  if (s === "canceled" || s === "cancelled") return { label: "已取消", tone: "err" };
  return { label: "待执行", tone: "idle" };
}

// 对话流内的结构化计划卡片：实时跟踪步骤进度，并支持一键批准执行或打开全屏详情。
export function InlinePlanCard({ onExecutePlan, onOpenDetail, pendingExecute = false, plan }: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const steps = plan.steps || [];
  const meta = statusMeta(plan.status);
  const isRunning = meta.tone === "busy";
  const isCompleted = meta.tone === "ok";
  const doneCount = steps.filter((step) => {
    const s = (step.status || "").toLowerCase();
    return s === "completed" || s === "succeeded" || s === "done";
  }).length;
  const pct = steps.length > 0 ? Math.round((doneCount / steps.length) * 100) : isCompleted ? 100 : 0;
  const [expanded, setExpanded] = useState(true);

  const badgeBg =
    meta.tone === "ok"
      ? palette.okWash
      : meta.tone === "busy"
        ? palette.brandWashStrong
        : meta.tone === "err"
          ? palette.errWash
          : palette.surface2;
  const badgeColor =
    meta.tone === "ok"
      ? palette.ok
      : meta.tone === "busy"
        ? palette.brandLight
        : meta.tone === "err"
          ? palette.err
          : palette.ink2;

  return (
    <View style={[styles.card, isRunning && styles.cardActive]}>
      <Pressable onPress={() => setExpanded((value) => !value)} style={({ pressed }) => [styles.head, pressed && styles.pressed]}>
        <View style={styles.iconBox}>
          <Feather color={palette.brandLight} name="clipboard" size={16} />
        </View>
        <View style={styles.headText}>
          <View style={styles.titleRow}>
            <Text style={styles.title}>执行计划</Text>
            {plan.revision ? <Text style={styles.revision}>v{plan.revision}</Text> : null}
          </View>
          <Text numberOfLines={1} style={styles.sub}>
            {steps.length > 0 ? `${doneCount}/${steps.length} 个步骤完成` : plan.goal || "点击查看详细内容"}
          </Text>
        </View>
        <View style={[styles.statusBadge, { backgroundColor: badgeBg }]}>
          <Text style={[styles.statusText, { color: badgeColor }]}>{meta.label}</Text>
        </View>
        <Feather color={palette.ink4} name={expanded ? "chevron-down" : "chevron-right"} size={15} />
      </Pressable>

      {plan.goal ? <Text style={styles.goal}>{plan.goal}</Text> : null}

      {steps.length > 0 ? (
        <View style={styles.progressTrack}>
          <View style={[styles.progressFill, { width: `${pct}%`, backgroundColor: isCompleted ? palette.ok : palette.brand }]} />
        </View>
      ) : null}

      {expanded && steps.length > 0 ? (
        <View style={styles.steps}>
          {steps.slice(0, 6).map((step, index) => {
            const st = (step.status || "").toLowerCase();
            const stepDone = st === "completed" || st === "succeeded" || st === "done";
            const stepRunning = st === "running" || st === "in_progress" || st === "executing";
            const stepFailed = st === "failed" || st === "error";
            return (
              <View key={step.id || `${index}`} style={styles.stepRow}>
                <View style={styles.stepIconWrap}>
                  {stepRunning ? (
                    <Spinner color={palette.brandLight} size={13} />
                  ) : stepDone ? (
                    <Feather color={palette.ok} name="check-circle" size={14} />
                  ) : stepFailed ? (
                    <Feather color={palette.err} name="x-circle" size={14} />
                  ) : (
                    <View style={styles.stepPendingDot} />
                  )}
                </View>
                <View style={styles.stepBody}>
                  <Text
                    numberOfLines={2}
                    style={[
                      styles.stepTitle,
                      stepDone && styles.stepTitleDone,
                      stepFailed && { color: palette.err },
                    ]}
                  >
                    {step.order ? `${step.order}. ` : `${index + 1}. `}
                    {step.title}
                  </Text>
                  {step.last_error ? (
                    <Text numberOfLines={2} style={styles.stepErr}>{step.last_error}</Text>
                  ) : null}
                </View>
              </View>
            );
          })}
          {steps.length > 6 ? (
            <Pressable onPress={onOpenDetail} style={styles.moreLink}>
              <Text style={styles.moreLinkText}>还有 {steps.length - 6} 个步骤，点此查看完整计划 ›</Text>
            </Pressable>
          ) : null}
        </View>
      ) : null}

      <View style={styles.actions}>
        <Pressable
          accessibilityRole="button"
          onPress={onOpenDetail}
          style={({ pressed }) => [styles.btn, styles.secondaryBtn, pressed && styles.pressed]}
        >
          <Feather color={palette.ink1} name="maximize-2" size={14} />
          <Text style={styles.secondaryText}>查看完整详情</Text>
        </Pressable>

        {!isCompleted && !isRunning ? (
          <Pressable
            accessibilityRole="button"
            disabled={pendingExecute}
            onPress={onExecutePlan}
            style={({ pressed }) => [styles.btn, styles.primaryBtn, pressed && styles.pressed, pendingExecute && styles.disabled]}
          >
            {pendingExecute ? (
              <ActivityIndicator color={palette.inkInverse} size="small" />
            ) : (
              <Feather color={palette.inkInverse} name="play" size={14} />
            )}
            <Text style={styles.primaryText}>{pendingExecute ? "启动中..." : "批准并执行"}</Text>
          </Pressable>
        ) : null}
      </View>
    </View>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    card: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.lg,
      borderWidth: 1,
      gap: 10,
      padding: 13,
    },
    cardActive: {
      borderColor: palette.brandDeep,
    },
    head: {
      alignItems: "center",
      flexDirection: "row",
      gap: 10,
    },
    iconBox: {
      alignItems: "center",
      backgroundColor: palette.brandWash,
      borderRadius: radius.sm,
      height: 32,
      justifyContent: "center",
      width: 32,
    },
    headText: {
      flex: 1,
      minWidth: 0,
    },
    titleRow: {
      alignItems: "center",
      flexDirection: "row",
      gap: 6,
    },
    title: {
      color: palette.ink1,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    revision: {
      color: palette.ink3,
      fontSize: 11,
      fontWeight: weight.medium,
    },
    sub: {
      color: palette.ink3,
      fontSize: font.caption - 0.5,
      marginTop: 1,
    },
    statusBadge: {
      borderRadius: radius.pill,
      paddingHorizontal: 9,
      paddingVertical: 3,
    },
    statusText: {
      fontSize: 11,
      fontWeight: weight.semibold,
    },
    goal: {
      color: palette.ink2,
      fontSize: font.label,
      lineHeight: 20,
    },
    progressTrack: {
      backgroundColor: palette.surface2,
      borderRadius: radius.pill,
      height: 4,
      overflow: "hidden",
    },
    progressFill: {
      borderRadius: radius.pill,
      height: "100%",
    },
    steps: {
      gap: 7,
      paddingTop: 2,
    },
    stepRow: {
      alignItems: "flex-start",
      flexDirection: "row",
      gap: 8,
    },
    stepIconWrap: {
      alignItems: "center",
      height: 18,
      justifyContent: "center",
      marginTop: 1,
      width: 18,
    },
    stepPendingDot: {
      borderColor: palette.ink4,
      borderRadius: radius.pill,
      borderWidth: 1.5,
      height: 8,
      width: 8,
    },
    stepBody: {
      flex: 1,
      gap: 2,
    },
    stepTitle: {
      color: palette.ink1,
      fontSize: font.label,
      lineHeight: 19,
    },
    stepTitleDone: {
      color: palette.ink2,
    },
    stepErr: {
      color: palette.err,
      fontSize: font.caption,
      lineHeight: 17,
    },
    moreLink: {
      paddingLeft: 26,
      paddingVertical: 2,
    },
    moreLinkText: {
      color: palette.brandLight,
      fontSize: font.caption,
    },
    actions: {
      flexDirection: "row",
      gap: 8,
      marginTop: 2,
    },
    btn: {
      alignItems: "center",
      borderRadius: radius.md,
      flex: 1,
      flexDirection: "row",
      gap: 6,
      justifyContent: "center",
      minHeight: 38,
      paddingHorizontal: 12,
    },
    secondaryBtn: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderWidth: 1,
    },
    primaryBtn: {
      backgroundColor: palette.brand,
    },
    secondaryText: {
      color: palette.ink1,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    primaryText: {
      color: palette.inkInverse,
      fontSize: font.label,
      fontWeight: weight.bold,
    },
    pressed: {
      opacity: 0.8,
    },
    disabled: {
      opacity: 0.55,
    },
  });
