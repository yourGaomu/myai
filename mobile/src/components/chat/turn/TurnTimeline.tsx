import { useMemo, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import type { SubagentTask } from "../../../protocol";
import type { ButtonFeedback } from "../../../types/ui";
import type { AgentTurnTimelineStep, ToolCallStep } from "../../../utils/chatRenderItems";
import { toolKind } from "../../../utils/toolDisplay";
import { font, radius } from "../../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../../theme/ThemeProvider";
import { MarkdownText } from "../MarkdownText";
import { SubagentParallelGroup, subagentTargetLabel } from "./SubagentParallelGroup";
import { ToolStepRow } from "./ToolStepRow";

type Props = {
  buttonFeedback: ButtonFeedback;
  onInspectSubagent?: (taskID: string) => void;
  steps: AgentTurnTimelineStep[];
  subagentTasks?: SubagentTask[];
};

const collapseAfter = 8;
const collapsedVisible = 6;

type GroupedToolChunk =
  | { id: string; tool: ToolCallStep; type: "single" }
  | {
      id: string;
      items: Array<{ target: string; task?: SubagentTask; tool: ToolCallStep }>;
      type: "subagent_batch";
    };

function findTaskForTool(tool: ToolCallStep, subagentTasks?: SubagentTask[]): SubagentTask | undefined {
  if (!subagentTasks?.length) return undefined;
  const callID = tool.toolCallID || tool.id;
  return subagentTasks.find(
    (t) => t.parent_tool_call_id && (t.parent_tool_call_id === callID || t.parent_tool_call_id === tool.id),
  );
}

// 过程时间线：思考文字和工具调用按发生顺序排在一条竖线上；连续≥2个子代理调用自动聚合为并行卡片。
export function TurnTimeline({ buttonFeedback, onInspectSubagent, steps, subagentTasks }: Props) {
  const styles = useThemedStyles(makeStyles);

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
  const styles = useThemedStyles(makeStyles);
  const [expanded, setExpanded] = useState(false);

  const chunks = useMemo<GroupedToolChunk[]>(() => {
    const out: GroupedToolChunk[] = [];
    let buffer: Array<{ target: string; task?: SubagentTask; tool: ToolCallStep }> = [];

    const flush = () => {
      if (buffer.length === 0) return;
      if (buffer.length >= 2) {
        out.push({
          id: `subbatch-${buffer[0].tool.id}`,
          items: buffer,
          type: "subagent_batch",
        });
      } else {
        out.push({
          id: buffer[0].tool.id,
          tool: buffer[0].tool,
          type: "single",
        });
      }
      buffer = [];
    };

    for (const tool of tools) {
      if (toolKind(tool.name) === "subagent_spawn") {
        buffer.push({
          target: subagentTargetLabel(tool),
          task: findTaskForTool(tool, subagentTasks),
          tool,
        });
      } else {
        flush();
        out.push({ id: tool.id, tool, type: "single" });
      }
    }
    flush();
    return out;
  }, [subagentTasks, tools]);

  const collapsible = chunks.length > collapseAfter && !expanded;
  const visible = collapsible ? chunks.slice(0, collapsedVisible) : chunks;

  return (
    <>
      {visible.map((chunk, index) =>
        chunk.type === "subagent_batch" ? (
          <SubagentParallelGroup
            entries={chunk.items}
            key={chunk.id || `batch-${index}`}
            onInspectSubagent={onInspectSubagent}
          />
        ) : (
          <ToolStepRow
            buttonFeedback={buttonFeedback}
            key={chunk.tool.id || `${chunk.tool.name}-${index}`}
            onInspectSubagent={onInspectSubagent}
            subagentTasks={subagentTasks}
            tool={chunk.tool}
          />
        ),
      )}
      {collapsible ? (
        <Pressable
          accessibilityRole="button"
          onPress={() => setExpanded(true)}
          style={({ pressed }) => [styles.more, pressed && styles.morePressed]}
        >
          <Text style={styles.moreText}>还有 {chunks.length - collapsedVisible} 项操作</Text>
        </Pressable>
      ) : null}
    </>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
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
