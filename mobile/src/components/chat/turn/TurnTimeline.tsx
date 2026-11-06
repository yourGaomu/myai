import { Fragment, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import type { SubagentTask } from "../../../protocol";
import type { ButtonFeedback } from "../../../types/ui";
import type { AgentTurnTimelineStep, ToolCallStep } from "../../../utils/chatRenderItems";
import { font, palette, radius } from "../../../constants/theme";
import { MarkdownText } from "../MarkdownText";
import { ToolStepRow } from "./ToolStepRow";

type Props = {
  buttonFeedback: ButtonFeedback;
  onInspectSubagent?: (taskID: string) => void;
  steps: AgentTurnTimelineStep[];
  subagentTasks?: SubagentTask[];
};

const collapseAfter = 8;
const collapsedVisible = 6;

// 过程时间线：思考文字和工具调用按发生顺序排在一条竖线上。
export function TurnTimeline({ buttonFeedback, onInspectSubagent, steps, subagentTasks }: Props) {
  return (
    <View style={styles.steps}>
      <View pointerEvents="none" style={styles.rail} />
      {steps.map((step) =>
        step.type === "thought" ? (
          <View key={step.id} style={styles.thought}>
            <View style={styles.thoughtDot} />
            <MarkdownText text={step.text} tone="muted" />
          </View>
        ) : (
          <ToolGroup
            buttonFeedback={buttonFeedback}
            key={step.id}
            onInspectSubagent={onInspectSubagent}
            subagentTasks={subagentTasks}
            tools={step.tools}
          />
        ),
      )}
    </View>
  );
}

function ToolGroup({
  buttonFeedback,
  onInspectSubagent,
  subagentTasks,
  tools,
}: {
  buttonFeedback: ButtonFeedback;
  onInspectSubagent?: (taskID: string) => void;
  subagentTasks?: SubagentTask[];
  tools: ToolCallStep[];
}) {
  const [expanded, setExpanded] = useState(false);
  const collapsible = tools.length > collapseAfter && !expanded;
  const visible = collapsible ? tools.slice(0, collapsedVisible) : tools;

  return (
    <>
      {visible.map((tool, index) => (
        <Fragment key={tool.id || `${tool.name}-${index}`}>
          <ToolStepRow
            buttonFeedback={buttonFeedback}
            onInspectSubagent={onInspectSubagent}
            subagentTasks={subagentTasks}
            tool={tool}
          />
        </Fragment>
      ))}
      {collapsible ? (
        <Pressable
          accessibilityRole="button"
          onPress={() => setExpanded(true)}
          style={({ pressed }) => [styles.more, pressed && styles.morePressed]}
        >
          <Text style={styles.moreText}>还有 {tools.length - collapsedVisible} 个操作</Text>
        </Pressable>
      ) : null}
    </>
  );
}

const styles = StyleSheet.create({
  steps: {
    gap: 2,
    paddingBottom: 4,
    paddingTop: 2,
    position: "relative",
  },
  rail: {
    backgroundColor: palette.hairlineStrong,
    bottom: 16,
    left: 13,
    position: "absolute",
    top: 16,
    width: 1,
  },
  thought: {
    paddingBottom: 6,
    paddingLeft: 35,
    paddingTop: 4,
    position: "relative",
  },
  thoughtDot: {
    backgroundColor: palette.canvas,
    borderColor: palette.ink4,
    borderRadius: radius.pill,
    borderWidth: 1.5,
    height: 7,
    left: 10,
    position: "absolute",
    top: 12,
    width: 7,
  },
  more: {
    borderRadius: radius.xs + 2,
    justifyContent: "center",
    marginLeft: 35,
    minHeight: 34,
  },
  morePressed: {
    backgroundColor: palette.pressed,
  },
  moreText: {
    color: palette.brandLight,
    fontSize: font.label,
  },
});
