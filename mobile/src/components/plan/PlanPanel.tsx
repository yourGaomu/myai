import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { Plan, PlanStep, SessionSummary } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";

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
  const canExecute = Boolean(clientToken && sessionID && plan && plan.status !== "done" && !pendingPlan);

  return (
    <View style={[styles.panel, styles.planPanel]}>
      <View style={styles.header}>
        <View style={styles.flex}>
          <Text style={styles.title}>Plan</Text>
          <Text numberOfLines={2} style={styles.meta}>
            {activeSession?.title || sessionID || "No active session"}
          </Text>
        </View>
        <View style={[styles.statusBadge, statusBadgeStyle(plan?.status)]}>
          {pendingPlan || plan?.status === "running" ? <ActivityIndicator color="#12100e" size="small" /> : null}
          <Text style={styles.statusText}>{plan?.status || "empty"}</Text>
        </View>
      </View>

      {!plan ? (
        <View style={styles.emptyBox}>
          <Text style={styles.emptyTitle}>No structured plan yet</Text>
          <Text style={styles.emptyText}>Switch this session to Plan mode, ask for a plan, then come back here to review and execute it.</Text>
        </View>
      ) : (
        <>
          <View style={styles.goalBox}>
            <Text style={styles.goalLabel}>Goal</Text>
            <Text style={styles.goalText}>{plan.goal || plan.steps?.[0]?.title || "Untitled plan"}</Text>
          </View>

          <View style={styles.progressBox}>
            <View style={styles.progressHeader}>
              <Text style={styles.progressText}>
                {progress.done}/{progress.total} done
              </Text>
              <Text style={styles.progressMeta}>{progress.running ? "running" : progress.failed ? "needs attention" : "ready"}</Text>
            </View>
            <View style={styles.progressTrack}>
              <View style={[styles.progressFill, { width: `${progress.percent}%` }]} />
            </View>
          </View>

          <View style={styles.stepList}>
            {(plan.steps || []).map((step) => (
              <PlanStepRow key={step.id || step.order} step={step} />
            ))}
          </View>
        </>
      )}

      <View style={styles.actions}>
        <Pressable
          onPress={onOpenChat}
          style={({ pressed }) => buttonFeedback([styles.secondaryButton], pressed)}
        >
          <Text style={styles.secondaryButtonText}>Back to chat</Text>
        </Pressable>
        <Pressable
          disabled={!canExecute}
          onPress={onExecutePlan}
          style={({ pressed }) => buttonFeedback([styles.primaryButton, !canExecute && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingPlan} text={pendingPlan ? "Executing" : plan?.status === "running" ? "Executing" : "Execute plan"} />
        </Pressable>
      </View>
    </View>
  );
}

function PlanStepRow({ step }: { step: PlanStep }) {
  const running = step.status === "running";
  return (
    <View style={[styles.stepRow, stepStatusStyle(step.status)]}>
      <View style={styles.stepIndex}>
        {running ? <ActivityIndicator color="#12100e" size="small" /> : <Text style={styles.stepIndexText}>{step.order}</Text>}
      </View>
      <View style={styles.flex}>
        <Text style={styles.stepTitle}>{step.title}</Text>
        {step.description ? <Text style={styles.stepDescription}>{step.description}</Text> : null}
      </View>
      <Text style={styles.stepStatus}>{step.status}</Text>
    </View>
  );
}

function planProgress(plan?: Plan) {
  const steps = plan?.steps || [];
  const total = steps.length;
  const done = steps.filter((step) => step.status === "done").length;
  const failed = steps.some((step) => step.status === "failed");
  const running = steps.some((step) => step.status === "running");
  const percent = total === 0 ? 0 : Math.round((done / total) * 100);
  return { done, failed, percent, running, total };
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
  return styles.stepPending;
}

const styles = StyleSheet.create({
  panel: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    elevation: 2,
    gap: 12,
    padding: 12,
    shadowColor: "#12100e",
    shadowOffset: { width: 4, height: 4 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
  },
  planPanel: {
    minHeight: 420,
  },
  header: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
    justifyContent: "space-between",
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  title: {
    color: "#12100e",
    fontSize: 28,
    fontWeight: "900",
  },
  meta: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "800",
    marginTop: 3,
  },
  statusBadge: {
    alignItems: "center",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 3,
    flexDirection: "row",
    gap: 6,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  statusDraft: {
    backgroundColor: "#f5eefc",
  },
  statusRunning: {
    backgroundColor: "#ffd84f",
  },
  statusDone: {
    backgroundColor: "#b9e9b0",
  },
  statusFailed: {
    backgroundColor: "#ff7f68",
  },
  statusText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  emptyBox: {
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 6,
    padding: 12,
  },
  emptyTitle: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  emptyText: {
    color: "#6c665f",
    fontSize: 13,
    fontWeight: "700",
    lineHeight: 19,
  },
  goalBox: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 4,
    padding: 10,
  },
  goalLabel: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "900",
    textTransform: "uppercase",
  },
  goalText: {
    color: "#12100e",
    fontSize: 15,
    fontWeight: "900",
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
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  progressMeta: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "800",
  },
  progressTrack: {
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 3,
    height: 16,
    overflow: "hidden",
  },
  progressFill: {
    backgroundColor: "#4fd7ee",
    height: "100%",
  },
  stepList: {
    gap: 8,
  },
  stepRow: {
    alignItems: "center",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  stepPending: {
    backgroundColor: "#fffaf0",
  },
  stepRunning: {
    backgroundColor: "#fff4cc",
  },
  stepDone: {
    backgroundColor: "#edf9e8",
  },
  stepFailed: {
    backgroundColor: "#ffe1db",
  },
  stepIndex: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 2,
    height: 34,
    justifyContent: "center",
    width: 34,
  },
  stepIndexText: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  stepTitle: {
    color: "#12100e",
    fontSize: 14,
    fontWeight: "900",
    lineHeight: 19,
  },
  stepDescription: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "700",
    marginTop: 3,
  },
  stepStatus: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  actions: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    justifyContent: "flex-end",
  },
  primaryButton: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    minHeight: 42,
    minWidth: 132,
    paddingHorizontal: 12,
    paddingVertical: 9,
  },
  secondaryButton: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    minHeight: 42,
    paddingHorizontal: 12,
    paddingVertical: 9,
  },
  secondaryButtonText: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  disabledButton: {
    opacity: 0.45,
  },
});
