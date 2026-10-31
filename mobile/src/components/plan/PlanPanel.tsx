import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { Plan, PlanStep, SessionSummary } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

type Props = {
  activeSession?: SessionSummary;
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  onExecutePlan: () => void;
  onOpenChat: () => void;
  pendingPlan: boolean;
  sessionID: string;
};

export function PlanPanel({
  activeSession,
  buttonFeedback,
  clientToken,
  onExecutePlan,
  onOpenChat,
  pendingPlan,
  sessionID,
}: Props) {
  const plan = activeSession?.current_plan;
  const progress = planProgress(plan);
  const canExecute = Boolean(
    clientToken &&
      sessionID &&
      plan &&
      (plan.steps?.length || 0) > 0 &&
      ["draft", "approved", "failed", "canceled"].includes(plan.status) &&
      !pendingPlan,
  );

  return (
    <View style={[styles.panel, styles.planPanel]}>
      <View style={styles.header}>
        <View style={styles.flex}>
          <Text style={styles.title}>计划</Text>
          <Text numberOfLines={2} style={styles.meta}>
            {activeSession?.title || sessionID || "没有活动会话"}
          </Text>
        </View>
        <View style={[styles.statusBadge, statusBadgeStyle(plan?.status)]}>
          {pendingPlan || plan?.status === "running" ? <ActivityIndicator color={palette.ink1} size="small" /> : null}
          <Text style={styles.statusText}>{planStatusLabel(plan?.status)}</Text>
        </View>
      </View>

      {!plan ? (
        <View style={styles.emptyBox}>
          <Text style={styles.emptyTitle}>还没有结构化计划</Text>
          <Text style={styles.emptyText}>开发类请求会自动生成并执行计划；需要查看过程时，计划和子智能体状态会在这里持续更新。</Text>
        </View>
      ) : (
        <>
          <View style={styles.goalBox}>
            <Text style={styles.goalLabel}>目标</Text>
            <Text style={styles.goalText}>{plan.goal || plan.steps?.[0]?.title || "未命名计划"}</Text>
          </View>

          <View style={styles.progressBox}>
            <View style={styles.progressHeader}>
              <Text style={styles.progressText}>
                已完成 {progress.done}/{progress.total}
              </Text>
              <Text style={styles.progressMeta}>{progress.running ? "执行中" : progress.failed ? "需要处理" : progress.waiting ? "等待依赖" : "就绪"}</Text>
            </View>
            <View style={styles.progressTrack}>
              <View style={[styles.progressFill, { width: `${progress.percent}%` }]} />
            </View>
          </View>

          <View style={styles.stepList}>
            {(plan.steps || []).map((step) => (
              <PlanStepRow key={step.id || step.order} plan={plan} step={step} />
            ))}
          </View>
        </>
      )}

      <View style={styles.actions}>
        <Pressable
          onPress={onOpenChat}
          style={({ pressed }) => buttonFeedback([styles.secondaryButton], pressed)}
        >
          <Text style={styles.secondaryButtonText}>返回对话</Text>
        </Pressable>
        <Pressable
          disabled={!canExecute}
          onPress={onExecutePlan}
          style={({ pressed }) => buttonFeedback([styles.primaryButton, !canExecute && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingPlan} text={pendingPlan ? "执行中" : plan?.status === "running" ? "执行中" : "执行计划"} />
        </Pressable>
      </View>
    </View>
  );
}

function PlanStepRow({ plan, step }: { plan: Plan; step: PlanStep }) {
  const running = step.status === "running";
  const waitingForDependency = step.status === "pending" && hasUnfinishedDependency(plan, step);
  const displayStatus = waitingForDependency ? "waiting_dependency" : step.status;
  return (
    <View style={[styles.stepRow, stepStatusStyle(displayStatus)]}>
      <View style={styles.stepIndex}>
        {running ? <ActivityIndicator color={palette.ink1} size="small" /> : <Text style={styles.stepIndexText}>{step.order}</Text>}
      </View>
      <View style={styles.flex}>
        <Text style={styles.stepTitle}>{step.title}</Text>
        {step.description ? <Text style={styles.stepDescription}>{step.description}</Text> : null}
        {step.dependencies?.length ? <Text style={styles.stepDependency}>依赖：{step.dependencies.join(", ")}</Text> : null}
        {step.last_error ? <Text style={styles.stepError}>{step.last_error}</Text> : null}
        {step.agent_task_id ? <Text style={styles.stepAgent}>子智能体：{step.agent_task_id}</Text> : null}
      </View>
      <View style={styles.stepMeta}>
        {step.retry_count ? <Text style={styles.stepRetry}>重试 {step.retry_count}{step.max_retries ? `/${step.max_retries}` : ""}</Text> : null}
        <Text style={styles.stepStatus}>{stepStatusLabel(displayStatus)}</Text>
      </View>
    </View>
  );
}

function planProgress(plan?: Plan) {
  const steps = plan?.steps || [];
  const total = steps.length;
  const done = steps.filter((step) => step.status === "done" || step.status === "skipped").length;
  const failed = steps.some((step) => step.status === "failed");
  const running = steps.some((step) => step.status === "running");
  const waiting = steps.some((step) => step.status === "pending" && hasUnfinishedDependency(plan, step));
  const percent = total === 0 ? 0 : Math.round((done / total) * 100);
  return { done, failed, percent, running, total, waiting };
}

function statusBadgeStyle(status?: string) {
  if (status === "done") {
    return styles.statusDone;
  }
  if (status === "running") {
    return styles.statusRunning;
  }
  if (status === "failed") {
    return styles.statusFailed;
  }
  return styles.statusDraft;
}

function planStatusLabel(status?: string) {
  switch (status) {
    case "draft": return "草稿";
    case "approved": return "已确认";
    case "running": return "执行中";
    case "done": return "已完成";
    case "failed": return "失败";
    case "canceled": return "已取消";
    default: return "空";
  }
}

function stepStatusLabel(status?: string) {
  switch (status) {
    case "pending": return "待执行";
    case "running": return "执行中";
    case "done": return "已完成";
    case "skipped": return "已跳过";
    case "waiting_dependency": return "等待依赖";
    case "failed": return "失败";
    case "canceled": return "已取消";
    default: return status || "未知";
  }
}

function stepStatusStyle(status?: string) {
  if (status === "done") {
    return styles.stepDone;
  }
  if (status === "running") {
    return styles.stepRunning;
  }
  if (status === "failed") {
    return styles.stepFailed;
  }
  if (status === "skipped") {
    return styles.stepDone;
  }
  if (status === "waiting_dependency") {
    return styles.stepWaiting;
  }
  return styles.stepPending;
}

function hasUnfinishedDependency(plan: Plan | undefined, step: PlanStep) {
  if (!plan || !step.dependencies?.length) {
    return false;
  }
  return step.dependencies.some((dependency) => {
    const candidate = plan.steps?.find(
      (item) => item.id === dependency || String(item.order) === dependency,
    );
    return Boolean(candidate && candidate.status !== "done" && candidate.status !== "skipped");
  });
}

const styles = StyleSheet.create({
  panel: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.lg,
    borderWidth: 1,
    gap: 12,
    padding: 12,
    ...shadow.card,
  },
  planPanel: {
    minHeight: 420,
  },
  header: {
    alignItems: "center",
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 10,
    justifyContent: "space-between",
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  title: {
    color: palette.ink1,
    fontSize: font.display,
    fontWeight: weight.heavy,
  },
  meta: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.bold,
    marginTop: 3,
  },
  statusBadge: {
    alignItems: "center",
    borderColor: palette.hairline,
    borderRadius: radius.pill,
    borderWidth: 1,
    flexDirection: "row",
    flexShrink: 0,
    gap: 6,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  statusDraft: {
    backgroundColor: palette.brandWash,
  },
  statusRunning: {
    backgroundColor: palette.brandWashStrong,
  },
  statusDone: {
    backgroundColor: palette.okWash,
  },
  statusFailed: {
    backgroundColor: palette.errWash,
  },
  statusText: {
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  emptyBox: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 6,
    padding: 12,
  },
  emptyTitle: {
    color: palette.ink1,
    fontSize: font.title,
    fontWeight: weight.bold,
  },
  emptyText: {
    color: palette.ink2,
    fontSize: font.body,
    fontWeight: weight.bold,
    lineHeight: 19,
  },
  goalBox: {
    backgroundColor: palette.brandWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 4,
    padding: 10,
  },
  goalLabel: {
    color: palette.ink2,
    fontSize: font.caption,
    fontWeight: weight.bold,
    textTransform: "uppercase",
  },
  goalText: {
    color: palette.ink1,
    fontSize: font.bodyLg,
    fontWeight: weight.bold,
    lineHeight: 21,
  },
  progressBox: {
    gap: 8,
  },
  progressHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  progressText: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  progressMeta: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  progressTrack: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.pill,
    borderWidth: 1,
    height: 16,
    overflow: "hidden",
  },
  progressFill: {
    backgroundColor: palette.cyanWash,
    height: "100%",
  },
  stepList: {
    gap: 8,
  },
  stepRow: {
    alignItems: "center",
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  stepPending: {
    backgroundColor: palette.surface1,
  },
  stepRunning: {
    backgroundColor: palette.brandWash,
  },
  stepDone: {
    backgroundColor: palette.okWash,
  },
  stepWaiting: {
    backgroundColor: palette.cyanWash,
  },
  stepFailed: {
    backgroundColor: palette.errWash,
  },
  stepIndex: {
    alignItems: "center",
    backgroundColor: palette.brandWashStrong,
    borderColor: palette.brandDeep,
    borderRadius: radius.pill,
    borderWidth: 1,
    height: 34,
    justifyContent: "center",
    width: 34,
  },
  stepIndexText: {
    color: palette.brand,
    fontSize: font.body,
    fontWeight: weight.heavy,
  },
  stepTitle: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
    lineHeight: 19,
  },
  stepDescription: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.bold,
    marginTop: 3,
  },
  stepDependency: {
    color: palette.ink2,
    fontSize: font.caption,
    lineHeight: 16,
    marginTop: 2,
  },
  stepError: {
    color: palette.err,
    fontSize: font.label,
    marginTop: 4,
  },
  stepAgent: {
    color: palette.cyan,
    fontSize: font.caption,
    marginTop: 4,
  },
  stepMeta: {
    alignItems: "flex-end",
    gap: 4,
  },
  stepRetry: {
    color: palette.brand,
    fontSize: font.caption,
  },
  stepStatus: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  actions: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    justifyContent: "flex-end",
  },
  primaryButton: {
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    minHeight: 42,
    minWidth: 132,
    paddingHorizontal: 12,
    paddingVertical: 9,
  },
  secondaryButton: {
    backgroundColor: palette.brandWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    minHeight: 42,
    paddingHorizontal: 12,
    paddingVertical: 9,
  },
  secondaryButtonText: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  disabledButton: {
    opacity: 0.45,
  },
});
