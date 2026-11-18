import { ActivityIndicator, Pressable, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { Plan, PlanStep, SessionSummary } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { useAppTheme } from "../../theme/ThemeProvider";
import { ShimmerProgressBar } from "../common/MotionEffects";
import { Spinner } from "../common/Spinner";
import { usePlanStyles, type PlanStyles } from "./planStyles";

type Props = {
  activeSession?: SessionSummary;
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  onExecutePlan: () => void;
  onOpenChat: () => void;
  pendingPlan: boolean;
  sessionID: string;
};

// 计划页：目标 + 进度 + 步骤时间线（自适应深浅主题）。
export function PlanPanel({ activeSession, clientToken, onExecutePlan, onOpenChat, pendingPlan, sessionID }: Props) {
  const { palette } = useAppTheme();
  const styles = usePlanStyles();
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
  const running = pendingPlan || plan?.status === "running";

  return (
    <View style={styles.page}>
      <View style={styles.head}>
        <Text numberOfLines={2} style={styles.session}>{activeSession?.title || sessionID || "没有活动会话"}</Text>
        <View style={[styles.chip, chipStyle(plan?.status, styles)]}>
          {running ? <Spinner size={12} /> : null}
          <Text style={[styles.chipText, chipTextStyle(plan?.status, styles)]}>{planStatusLabel(plan?.status)}</Text>
        </View>
      </View>

      {!plan ? (
        <View style={styles.empty}>
          <Text style={styles.emptyTitle}>还没有结构化计划</Text>
          <Text style={styles.emptyText}>开发类请求会自动生成并执行计划。执行过程和子代理状态会在这里持续更新。</Text>
        </View>
      ) : (
        <>
          <View style={styles.goal}>
            <Text style={styles.goalLabel}>目标</Text>
            <Text style={styles.goalText}>{plan.goal || plan.steps?.[0]?.title || "未命名计划"}</Text>
          </View>

          <View style={styles.progress}>
            <Text style={styles.progressText}>
              已完成 {progress.done}/{progress.total}
              {progress.running ? " · 执行中" : progress.failed ? " · 需要处理" : progress.waiting ? " · 等待依赖" : ""}
            </Text>
            <ShimmerProgressBar
              active={running}
              height={6}
              progress={progress.percent}
              tone={progress.done === progress.total && progress.total > 0 ? "ok" : "brand"}
            />
          </View>

          <View style={styles.steps}>
            {(plan.steps?.length || 0) > 1 ? <View pointerEvents="none" style={styles.rail} /> : null}
            {(plan.steps || []).map((step) => (
              <StepRow key={step.id || step.order} plan={plan} step={step} styles={styles} />
            ))}
          </View>
        </>
      )}

      <View style={styles.actions}>
        <Pressable accessibilityRole="button" onPress={onOpenChat} style={({ pressed }) => [styles.button, styles.buttonSecondary, pressed && styles.disabled]}>
          <Text style={styles.buttonSecondaryText}>返回对话</Text>
        </Pressable>
        <Pressable
          accessibilityRole="button"
          disabled={!canExecute}
          onPress={onExecutePlan}
          style={({ pressed }) => [styles.button, styles.buttonPrimary, (!canExecute || pressed) && styles.disabled]}
        >
          {pendingPlan ? <ActivityIndicator color={palette.inkInverse} size="small" /> : null}
          <Text style={styles.buttonPrimaryText}>{running ? "执行中" : "执行计划"}</Text>
        </Pressable>
      </View>
    </View>
  );
}

function StepRow({ plan, step, styles }: { plan: Plan; step: PlanStep; styles: PlanStyles }) {
  const waiting = step.status === "pending" && hasUnfinishedDependency(plan, step);
  const status = waiting ? "waiting_dependency" : step.status;
  const finished = status === "done" || status === "skipped";
  return (
    <View style={styles.step}>
      <View style={styles.stepIcon}>
        <StepIcon status={status} styles={styles} />
      </View>
      <View style={styles.stepBody}>
        <Text style={[styles.stepTitle, finished && styles.stepTitleDone]}>{step.title}</Text>
        {step.description ? <Text style={styles.stepSub}>{step.description}</Text> : null}
        {step.dependencies?.length ? <Text style={styles.stepSub}>依赖：{step.dependencies.join("、")}</Text> : null}
        {step.last_error ? <Text style={styles.stepError}>{step.last_error}</Text> : null}
        {step.agent_task_id ? <Text style={styles.stepSub}>子代理 {step.agent_task_id}</Text> : null}
      </View>
      <Text style={[styles.stepStatus, status === "running" && styles.stepStatusRunning, status === "failed" && styles.stepStatusFailed]}>
        {step.retry_count ? `重试 ${step.retry_count}${step.max_retries ? `/${step.max_retries}` : ""} · ` : ""}
        {stepStatusLabel(status)}
      </Text>
    </View>
  );
}

function StepIcon({ status, styles }: { status?: string; styles: PlanStyles }) {
  const { palette } = useAppTheme();
  if (status === "running") return <Spinner size={15} />;
  if (status === "done") return <Feather color={palette.ok} name="check-circle" size={18} />;
  if (status === "skipped") return <Feather color={palette.ink3} name="skip-forward" size={16} />;
  if (status === "failed") return <Feather color={palette.err} name="x-circle" size={18} />;
  if (status === "canceled") return <Feather color={palette.ink3} name="slash" size={16} />;
  if (status === "waiting_dependency") return <Feather color={palette.warn} name="clock" size={16} />;
  return <View style={styles.pendingDot} />;
}

function chipStyle(status: string | undefined, styles: PlanStyles) {
  if (status === "running") return styles.chipRunning;
  if (status === "done") return styles.chipDone;
  if (status === "failed") return styles.chipFailed;
  return null;
}

function chipTextStyle(status: string | undefined, styles: PlanStyles) {
  if (status === "running") return styles.chipTextRunning;
  if (status === "done") return styles.chipTextDone;
  if (status === "failed") return styles.chipTextFailed;
  return null;
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

function hasUnfinishedDependency(plan: Plan | undefined, step: PlanStep) {
  if (!plan || !step.dependencies?.length) {
    return false;
  }
  return step.dependencies.some((dependency) => {
    const candidate = plan.steps?.find((item) => item.id === dependency || String(item.order) === dependency);
    return Boolean(candidate && candidate.status !== "done" && candidate.status !== "skipped");
  });
}
