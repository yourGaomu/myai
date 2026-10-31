import { useMemo, useState } from "react";
import { ActivityIndicator, Platform, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { AgentTurnItem, AgentTurnTimelineStep, ToolCallStep } from "../../utils/chatRenderItems";
import type { SubagentTask } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { parseSharedAsset } from "../../utils/toolAssets";
import { usageBreakdown } from "../../utils/tokenUsage";
import { ChatAttachmentCard } from "./ChatAttachmentCard";
import { MarkdownText } from "./MarkdownText";
import { SharedAssetCard } from "./SharedAssetCard";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

type FileChangeTag = {
  action: "NEW" | "MOD";
  path: string;
};

function isShellLikeTool(name: string): boolean {
  const lower = name.toLowerCase();
  return (
    lower.includes("bash") ||
    lower.includes("shell") ||
    lower.includes("sh") ||
    lower.includes("cmd") ||
    lower.includes("powershell") ||
    lower.includes("command") ||
    lower.includes("terminal") ||
    lower.includes("exec")
  );
}

function extractToolSummary(tool: ToolCallStep): string {
  if (!tool.arguments) return "";
  const raw = String(tool.arguments).trim();
  if (!raw) return "";
  try {
    const parsed = typeof tool.arguments === "string" ? JSON.parse(raw) : tool.arguments;
    if (typeof parsed === "string") {
      return parsed.trim();
    }
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      const candidate =
        parsed.command ??
        parsed.cmd ??
        parsed.CommandLine ??
        parsed.command_line ??
        parsed.script ??
        parsed.path ??
        parsed.file_path ??
        parsed.TargetFile ??
        parsed.AbsolutePath ??
        parsed.query ??
        parsed.Query ??
        parsed.pattern ??
        parsed.Pattern ??
        parsed.url ??
        parsed.Url ??
        parsed.skill_name ??
        parsed.task_id ??
        parsed.title ??
        parsed.name ??
        parsed.toolSummary;
      if (candidate !== undefined && candidate !== null && String(candidate).trim() !== "") {
        return String(candidate).trim();
      }
      const pairs = Object.entries(parsed)
        .filter(([, v]) => v !== undefined && v !== null && typeof v !== "object")
        .slice(0, 2)
        .map(([k, v]) => `${k}=${String(v).trim()}`);
      if (pairs.length > 0) {
        return pairs.join(", ");
      }
      return "";
    }
  } catch {
    return raw.startsWith("{") || raw.startsWith("[") ? "" : raw;
  }
  return "";
}

function formatToolRequestText(tool: ToolCallStep): string {
  if (!tool.arguments) return "";
  const raw = String(tool.arguments).trim();
  if (!raw) return "";
  try {
    const parsed = typeof tool.arguments === "string" ? JSON.parse(raw) : tool.arguments;
    if (typeof parsed === "string") {
      return isShellLikeTool(tool.name) ? `$ ${parsed.trim()}` : parsed.trim();
    }
    if (parsed && typeof parsed === "object") {
      const keys = Object.keys(parsed);
      const cmdValue =
        parsed.command ?? parsed.cmd ?? parsed.CommandLine ?? parsed.command_line ?? parsed.script;
      if (isShellLikeTool(tool.name) && typeof cmdValue === "string" && cmdValue.trim()) {
        if (keys.length === 1) {
          return `$ ${cmdValue.trim()}`;
        }
        const rest = { ...parsed };
        delete rest.command;
        delete rest.cmd;
        delete rest.CommandLine;
        delete rest.command_line;
        delete rest.script;
        return `$ ${cmdValue.trim()}\n\n${JSON.stringify(rest, null, 2)}`;
      }
      return JSON.stringify(parsed, null, 2);
    }
  } catch {
    return isShellLikeTool(tool.name) && !raw.startsWith("{") ? `$ ${raw}` : raw;
  }
  return raw;
}

function formatSingleLineCommand(cmd: string): string {
  return cmd.replace(/\s+/g, " ").trim();
}

function formatToolActionRowTitle(tool: ToolCallStep): string {
  const summary = formatSingleLineCommand(extractToolSummary(tool));
  const lower = tool.name.toLowerCase();

  if (tool.name === "wait_agent") {
    if (tool.status === "running") {
      return summary ? `等待子代理结果 · ${summary}` : "等待子代理结果";
    }
    if (tool.status === "error" || tool.error) {
      return summary ? `等待子代理失败 · ${summary}` : "等待子代理失败";
    }
    return summary ? `已获取子代理结果 · ${summary}` : "子代理结果已就绪";
  }

  if (tool.name === "knowledge_search") {
    if (tool.status === "running") {
      return summary ? `正在执行 知识库检索 · ${summary}` : "正在执行 知识库检索";
    }
    if (tool.status === "error" || tool.error) {
      return summary ? `知识库检索失败 · ${summary}` : "知识库检索失败";
    }
    return summary ? `已检索知识库 · ${summary}` : "知识库检索";
  }

  if (tool.status === "running") {
    return summary ? `正在运行 ${tool.name} · ${summary}` : `正在运行 ${tool.name}`;
  }
  if (tool.status === "error" || tool.error) {
    return summary ? `运行失败 ${tool.name} · ${summary}` : `运行失败 ${tool.name}`;
  }

  if (isShellLikeTool(tool.name)) {
    return summary ? `已运行 ${summary}` : `已运行 ${tool.name}`;
  }
  if (lower.includes("read") || lower.includes("view")) {
    return summary ? `已读取 ${summary}` : `已调用 ${tool.name}`;
  }
  if (lower.includes("write") || lower.includes("edit") || lower.includes("replace") || lower.includes("patch")) {
    return summary ? `已修改 ${summary}` : `已调用 ${tool.name}`;
  }
  if (lower.includes("search") || lower.includes("grep") || lower.includes("find") || lower.includes("glob")) {
    return summary ? `已搜索 ${summary}` : `已调用 ${tool.name}`;
  }
  return summary ? `已调用 ${tool.name} · ${summary}` : `已调用 ${tool.name}`;
}

function extractFileChanges(tools: ToolCallStep[] | undefined): FileChangeTag[] {
  if (!tools) return [];
  const files: FileChangeTag[] = [];
  const seen = new Set<string>();
  for (const t of tools) {
    const lower = t.name.toLowerCase();
    if (
      t.name === "write_to_file" ||
      t.name === "replace_file_content" ||
      lower.includes("write") ||
      lower.includes("edit") ||
      lower.includes("patch")
    ) {
      try {
        const parsed = typeof t.arguments === "string" ? JSON.parse(t.arguments) : t.arguments;
        const target = parsed?.TargetFile || parsed?.file_path || parsed?.path || "";
        if (target && !seen.has(target)) {
          seen.add(target);
          files.push({
            action: lower.includes("write") ? "NEW" : "MOD",
            path: String(target),
          });
        }
      } catch {
        // ignore
      }
    }
  }
  return files;
}

type Props = {
  buttonFeedback: ButtonFeedback;
  onRegenerate?: () => void;
  turn: AgentTurnItem;
  subagentTasks?: SubagentTask[];
  onInspectSubagent?: (taskID: string) => void;
};

export function AgentTurnCard({
  buttonFeedback,
  onRegenerate,
  turn,
  subagentTasks,
  onInspectSubagent,
}: Props) {
  const isRunning = turn.status === "running";
  const timeline = useMemo<AgentTurnTimelineStep[]>(() => {
    if (turn.timeline && turn.timeline.length > 0) {
      return turn.timeline;
    }
    const fallback: AgentTurnTimelineStep[] = [];
    if (turn.reasoning?.trim()) {
      fallback.push({
        type: "thought",
        id: `${turn.id}-fallback-thought`,
        text: turn.reasoning.trim(),
      });
    }
    if (turn.tools && turn.tools.length > 0) {
      fallback.push({
        type: "tool_group",
        id: `${turn.id}-fallback-tools`,
        tools: turn.tools,
      });
    }
    return fallback;
  }, [turn.id, turn.reasoning, turn.timeline, turn.tools]);

  const hasProcess = timeline.length > 0;
  const toolCount = turn.tools ? turn.tools.length : 0;
  const [trayOpen, setTrayOpen] = useState(true);
  const [copied, setCopied] = useState(false);

  const tokensInfo = useMemo(() => usageBreakdown(turn.usage), [turn.usage]);
  const fileChanges = useMemo(() => extractFileChanges(turn.tools), [turn.tools]);

  const durationHeaderLabel = useMemo(() => {
    if (isRunning) {
      const isWaitingSubagent = turn.tools?.some(
        (t) => t.name === "wait_agent" && t.status === "running",
      );
      if (isWaitingSubagent) {
        return turn.elapsed ? `等待子代理结果 · ${turn.elapsed}` : "等待子代理结果...";
      }
      return turn.elapsed ? `正在思考与执行 · ${turn.elapsed}` : "正在思考与执行...";
    }
    if (turn.elapsed) {
      return `用时 ${turn.elapsed}`;
    }
    if (toolCount > 0) {
      return `已执行 ${toolCount} 个命令与操作`;
    }
    return "思考推导过程";
  }, [isRunning, toolCount, turn.elapsed, turn.tools]);

  const handleCopy = () => {
    const copyContent = (turn.text || turn.reasoning || "").trim();
    if (
      copyContent &&
      typeof navigator !== "undefined" &&
      navigator.clipboard &&
      typeof navigator.clipboard.writeText === "function"
    ) {
      void navigator.clipboard.writeText(copyContent).catch(() => {});
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <View style={styles.wrapper}>
      {/* 头部：Agent 身份与状态徽章 */}
      <View style={styles.header}>
        <View style={styles.identity}>
          <View style={styles.avatarIcon}>
            <Feather color={palette.inkInverse} name="cpu" size={14} />
          </View>
          <Text style={styles.agentName}>{turn.model || "MyAI Agent"}</Text>
          <View style={styles.modelPill}>
            <Text style={styles.modelPillText}>v2.4 Auto</Text>
          </View>
        </View>
        <View
          style={[
            styles.statusBadge,
            isRunning
              ? styles.statusRunning
              : turn.status === "error"
                ? styles.statusError
                : turn.status === "paused"
                  ? styles.statusPaused
                  : styles.statusCompleted,
          ]}
        >
          {isRunning ? (
            <>
              <View style={styles.pulseDot} />
              <Text style={styles.statusRunningText}>正在执行{turn.elapsed ? ` · ${turn.elapsed}` : "..."}</Text>
            </>
          ) : turn.status === "error" ? (
            <Text style={styles.statusErrorText}>执行异常</Text>
          ) : turn.status === "paused" ? (
            <>
              <Feather color={palette.ink1} name="pause" size={12} />
              <Text style={styles.statusPausedText}>已暂停{turn.elapsed ? ` · ${turn.elapsed}` : ""}</Text>
            </>
          ) : (
            <>
              <Feather color={palette.ok} name="check" size={12} />
              <Text style={styles.statusCompletedText}>完成{turn.elapsed ? ` · ${turn.elapsed}` : ""}</Text>
            </>
          )}
        </View>
      </View>

      {/* 复合气泡大卡片 */}
      <View style={styles.cardBox}>
        {/* 顶部耗时折叠头 + 交错式过程时间流 */}
        {hasProcess ? (
          <View style={styles.processContainer}>
            <Pressable
              onPress={() => setTrayOpen((prev) => !prev)}
              style={({ pressed }) => buttonFeedback(styles.durationHeader, pressed)}
            >
              <Text style={styles.durationHeaderText}>{durationHeaderLabel}</Text>
              <Feather color={palette.ink3} name={trayOpen ? "chevron-down" : "chevron-right"} size={14} />
            </Pressable>

            {trayOpen ? (
              <View style={styles.interleavedBody}>
                {timeline.map((step) => {
                  if (step.type === "thought") {
                    return (
                      <View key={step.id} style={styles.thoughtBlock}>
                        <MarkdownText text={step.text} />
                      </View>
                    );
                  }
                  return (
                    <InterleavedToolGroup
                      buttonFeedback={buttonFeedback}
                      key={step.id}
                      onInspectSubagent={onInspectSubagent}
                      subagentTasks={subagentTasks}
                      tools={step.tools}
                    />
                  );
                })}

                {/* 关联文件变更审查卡片 */}
                {fileChanges.length > 0 ? (
                  <View style={styles.reviewSection}>
                    <View style={styles.reviewHeaderRow}>
                      <View style={styles.iconRow}>
                        <Feather color={palette.brand} name="edit-3" size={14} />
                        <Text style={styles.reviewTitle}>关联文件变更 ({fileChanges.length} 个文件)</Text>
                      </View>
                      <View style={styles.reviewBadge}>
                        <Text style={styles.reviewBadgeText}>改动已生效</Text>
                      </View>
                    </View>
                    <View style={styles.reviewList}>
                      {fileChanges.map((file, i) => {
                        const basename = file.path.split(/[/\\]/).pop() || file.path;
                        return (
                          <View key={`${file.path}-${i}`} style={styles.reviewItem}>
                            <View style={styles.reviewItemLeft}>
                              <View
                                style={[
                                  styles.reviewActionTag,
                                  file.action === "NEW" ? styles.reviewActionTagNew : styles.reviewActionTagMod,
                                ]}
                              >
                                <Text style={styles.reviewActionTagText}>{file.action}</Text>
                              </View>
                              <Text numberOfLines={1} style={styles.reviewFilePath}>
                                {basename}
                              </Text>
                            </View>
                            <View style={styles.iconRow}>
                              <Text style={styles.reviewDiffHint}>查看</Text>
                              <Feather color={palette.ink3} name="chevron-right" size={13} />
                            </View>
                          </View>
                        );
                      })}
                    </View>
                  </View>
                ) : null}
              </View>
            ) : null}
          </View>
        ) : null}

        {/* 最终回答正文 */}
        {turn.text || (turn.attachments && turn.attachments.length > 0) ? (
          <View style={[styles.cardBody, hasProcess && styles.cardBodyWithProcess]}>
            {turn.text ? <MarkdownText text={turn.text} /> : null}

            {/* 附件列表 */}
            {turn.attachments?.length ? (
              <View style={styles.attachments}>
                {turn.attachments.map((attachment, index) => (
                  <ChatAttachmentCard
                    attachment={attachment}
                    buttonFeedback={buttonFeedback}
                    key={`${turn.id}-att-${index}`}
                  />
                ))}
              </View>
            ) : null}
          </View>
        ) : null}

        {/* 底部元信息与操作栏 */}
        <View style={styles.cardFooter}>
          <View style={styles.footerActions}>
            <Pressable onPress={handleCopy} style={({ pressed }) => buttonFeedback(styles.actionBtn, pressed)}>
              <Feather color={palette.ink2} name={copied ? "check" : "clipboard"} size={13} />
              <Text style={styles.actionBtnText}>{copied ? "已复制" : "复制"}</Text>
            </Pressable>
            {turn.canRegenerate && onRegenerate ? (
              <Pressable
                onPress={onRegenerate}
                style={({ pressed }) => buttonFeedback([styles.actionBtn, styles.regenBtn], pressed)}
              >
                <Feather color={palette.ink1} name="refresh-cw" size={13} />
                <Text style={styles.regenBtnText}>重新生成</Text>
              </Pressable>
            ) : null}
          </View>

          {/* Token 明细拆解 */}
          {tokensInfo ? (
            <View style={styles.tokenStat}>
              <View style={styles.tokenBadge}>
                <Text style={styles.tokenBadgeText}>{tokensInfo.total} tokens</Text>
              </View>
              {tokensInfo.hasBreakdown ? (
                <Text style={styles.tokenBreakdownText}>
                  (入 {tokensInfo.input} · 出 {tokensInfo.output})
                </Text>
              ) : null}
            </View>
          ) : null}
        </View>
      </View>
    </View>
  );
}

function InterleavedToolGroup({
  buttonFeedback,
  tools,
  subagentTasks,
  onInspectSubagent,
}: {
  buttonFeedback: ButtonFeedback;
  tools: ToolCallStep[];
  subagentTasks?: SubagentTask[];
  onInspectSubagent?: (taskID: string) => void;
}) {
  const [groupOpen, setGroupOpen] = useState(true);

  if (tools.length === 1) {
    return (
      <InterleavedToolRow
        buttonFeedback={buttonFeedback}
        onInspectSubagent={onInspectSubagent}
        subagentTasks={subagentTasks}
        tool={tools[0]}
      />
    );
  }

  const runningCount = tools.filter((t) => t.status === "running").length;
  const failedCount = tools.filter((t) => Boolean(t.error || t.status === "error")).length;
  const groupTitle =
    runningCount > 0
      ? `正在调用 ${tools.length} 个工具 (${runningCount} 个执行中)`
      : failedCount > 0
        ? `已调用 ${tools.length} 个工具 (${failedCount} 个失败)`
        : `已调用 ${tools.length} 个工具`;

  return (
    <View style={styles.toolGroupBlock}>
      <Pressable
        onPress={() => setGroupOpen((prev) => !prev)}
        style={({ pressed }) => buttonFeedback(styles.toolInlineRow, pressed)}
      >
        <View style={[styles.termIconBadge, failedCount > 0 && styles.termIconBadgeError]}>
          <Text style={[styles.termIconText, failedCount > 0 && styles.termIconTextError]}>{">_"}</Text>
        </View>
        <Text numberOfLines={1} style={styles.toolInlineGroupTitle}>
          {groupTitle}
        </Text>
        <Feather color={palette.ink3} name={groupOpen ? "chevron-down" : "chevron-right"} size={14} />
      </Pressable>

      {groupOpen ? (
        <View style={styles.toolGroupChildren}>
          {tools.map((tool, index) => (
            <InterleavedToolRow
              buttonFeedback={buttonFeedback}
              key={tool.id || `${tool.name}-${index}`}
              onInspectSubagent={onInspectSubagent}
              subagentTasks={subagentTasks}
              tool={tool}
            />
          ))}
        </View>
      ) : null}
    </View>
  );
}

function InterleavedToolRow({
  buttonFeedback,
  tool,
  subagentTasks,
  onInspectSubagent,
}: {
  buttonFeedback: ButtonFeedback;
  tool: ToolCallStep;
  subagentTasks?: SubagentTask[];
  onInspectSubagent?: (taskID: string) => void;
}) {
  const isFailed = Boolean(tool.error || tool.status === "error");
  const isRunning = tool.status === "running";
  const [expanded, setExpanded] = useState(isFailed);
  const sharedAsset = parseSharedAsset(tool.name, tool.result || "");
  const requestText = useMemo(() => formatToolRequestText(tool), [tool]);
  const rowTitle = useMemo(() => formatToolActionRowTitle(tool), [tool]);
  const outputText = tool.error || tool.result || "";

  // 1.0 按 parent_tool_call_id 精确配对子代理任务（并发任务不按顺序或工具名配对）
  const matchingSubagentTask = useMemo(() => {
    if (!subagentTasks || subagentTasks.length === 0) return undefined;
    const currentToolCallID = tool.toolCallID || tool.id;
    return subagentTasks.find(
      (t) =>
        t.parent_tool_call_id &&
        (t.parent_tool_call_id === currentToolCallID || t.parent_tool_call_id === tool.id),
    );
  }, [subagentTasks, tool.id, tool.toolCallID]);

  return (
    <View style={styles.toolStepContainer}>
      <Pressable
        onPress={() => setExpanded((prev) => !prev)}
        style={({ pressed }) => buttonFeedback(styles.toolInlineRow, pressed)}
      >
        <View
          style={[
            styles.termIconBadge,
            isFailed && styles.termIconBadgeError,
            isRunning && styles.termIconBadgeRunning,
          ]}
        >
          <Text
            style={[
              styles.termIconText,
              isFailed && styles.termIconTextError,
              isRunning && styles.termIconTextRunning,
            ]}
          >
            {">_"}
          </Text>
        </View>
        <Text
          numberOfLines={1}
          style={[styles.toolInlineText, isFailed && styles.toolInlineTextError]}
        >
          {rowTitle}
        </Text>
        {tool.duration ? <Text style={styles.toolInlineDuration}>{tool.duration}</Text> : null}
        <Feather color={palette.ink3} name={expanded ? "chevron-down" : "chevron-right"} size={14} />
      </Pressable>

      {/* 2.0 主聊天子代理摘要入口：按 parent_tool_call_id 匹配，显示状态与最近输出 */}
      {matchingSubagentTask ? (
        <View style={styles.subagentSummaryCard}>
          <View style={styles.subagentSummaryHeader}>
            <View style={styles.subagentSummaryTitleRow}>
              <Feather color={palette.brand} name="cpu" size={14} />
              <Text numberOfLines={1} style={styles.subagentSummaryName}>
                {matchingSubagentTask.agent_nickname || matchingSubagentTask.title || "子代理"}
              </Text>
              <View
                style={[
                  styles.subagentStatusPill,
                  matchingSubagentTask.status === "running" && styles.subagentStatusRunning,
                  matchingSubagentTask.status === "succeeded" && styles.subagentStatusSucceeded,
                  matchingSubagentTask.status === "failed" && styles.subagentStatusFailed,
                  (matchingSubagentTask.status === "waiting_subagents" ||
                    matchingSubagentTask.status === "waiting_permission") &&
                    styles.subagentStatusWaiting,
                ]}
              >
                <Text style={styles.subagentStatusPillText}>
                  {formatSubagentStatus(matchingSubagentTask.status)}
                </Text>
              </View>
            </View>
            {onInspectSubagent ? (
              <Pressable
                onPress={() => onInspectSubagent(matchingSubagentTask.id)}
                style={({ pressed }) => buttonFeedback(styles.inspectSubagentBtn, pressed)}
              >
                <Feather color={palette.ink1} name="search" size={12} />
                <Text style={styles.inspectSubagentBtnText}>实时详情</Text>
                <Feather color={palette.ink3} name="chevron-right" size={13} />
              </Pressable>
            ) : null}
          </View>
          {matchingSubagentTask.result || matchingSubagentTask.reasoning || matchingSubagentTask.instruction ? (
            <Text numberOfLines={2} style={styles.subagentSummarySnippet}>
              {matchingSubagentTask.result || matchingSubagentTask.reasoning || matchingSubagentTask.instruction}
            </Text>
          ) : null}
        </View>
      ) : (tool.name === "spawn_agent" || tool.name === "start_async_task") && isRunning ? (
        <View style={styles.subagentSpawningCard}>
          <ActivityIndicator color={palette.info} size="small" />
          <Text style={styles.subagentSpawningText}>子代理启动中...</Text>
        </View>
      ) : null}

      {expanded ? (
        <View style={styles.shellBox}>
          {/* 上半栏：工具调用请求 (Arguments) */}
          <View style={styles.toolSectionBlock}>
            <View style={styles.shellHeaderBar}>
              <View style={styles.toolSectionHeaderLeft}>
                <View style={styles.toolReqBadge}>
                  <Text style={styles.toolReqBadgeText}>调用请求</Text>
                </View>
                <Text numberOfLines={1} style={styles.shellHeaderTitle}>
                  {tool.name === "knowledge_search" ? "知识库检索" : tool.name}
                </Text>
              </View>
            </View>
            <ScrollView
              nestedScrollEnabled
              showsVerticalScrollIndicator
              style={styles.toolRequestScroll}
            >
              {requestText ? (
                <Text selectable style={styles.shellCommandText}>
                  {requestText}
                </Text>
              ) : (
                <Text style={styles.shellEmptyText}>(无请求参数)</Text>
              )}
            </ScrollView>
          </View>

          <View style={styles.toolSectionDivider} />

          {/* 下半栏：工具执行返回 (Result / Error) */}
          <View style={styles.toolSectionBlock}>
            <View style={styles.shellHeaderBar}>
              <View style={styles.toolSectionHeaderLeft}>
                <View
                  style={[
                    styles.toolResBadge,
                    isFailed
                      ? styles.toolResBadgeError
                      : isRunning
                        ? styles.toolResBadgeRunning
                        : styles.toolResBadgeSuccess,
                  ]}
                >
                  <Text
                    style={[
                      styles.toolResBadgeText,
                      isFailed
                        ? styles.toolResBadgeTextError
                        : isRunning
                          ? styles.toolResBadgeTextRunning
                          : styles.toolResBadgeTextSuccess,
                    ]}
                  >
                    {isFailed ? "执行异常" : isRunning ? "执行中" : "执行返回"}
                  </Text>
                </View>
                {tool.errorCode ? (
                  <Text style={styles.toolErrorCodeText}>{tool.errorCode}</Text>
                ) : null}
                {tool.truncated ? (
                  <Text style={styles.toolTruncatedTag}>已截断</Text>
                ) : null}
              </View>
              {tool.duration ? <Text style={styles.shellHeaderMeta}>{tool.duration}</Text> : null}
            </View>

            <ScrollView
              nestedScrollEnabled
              showsVerticalScrollIndicator
              style={styles.shellScroll}
            >
              {sharedAsset && !isFailed ? (
                <View style={styles.shellAssetWrap}>
                  <SharedAssetCard asset={sharedAsset} buttonFeedback={buttonFeedback} />
                </View>
              ) : outputText ? (
                <Text
                  selectable
                  style={[styles.shellOutputText, isFailed && styles.shellOutputTextError]}
                >
                  {outputText}
                </Text>
              ) : (
                <Text style={styles.shellEmptyText}>
                  {isRunning ? "正在等待工具执行返回..." : "(无返回内容)"}
                </Text>
              )}
            </ScrollView>
          </View>
        </View>
      ) : null}
    </View>
  );
}

const monoFont = Platform.OS === "ios" ? "Menlo" : "monospace";

const styles = StyleSheet.create({
  wrapper: {
    alignSelf: "stretch",
    gap: 4,
    width: "100%",
  },
  header: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 2,
  },
  identity: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  avatarIcon: {
    alignItems: "center",
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.xs,
    borderWidth: 1,
    height: 22,
    justifyContent: "center",
    width: 22,
  },
  avatarEmoji: {
    fontSize: font.label,
  },
  agentName: {
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  modelPill: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.xs,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  modelPillText: {
    color: palette.ink1,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  statusBadge: {
    alignItems: "center",
    borderRadius: radius.md,
    borderWidth: 1,
    flexDirection: "row",
    gap: 4,
    paddingHorizontal: 7,
    paddingVertical: 2,
  },
  iconRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 5,
  },
  statusCompleted: {
    backgroundColor: palette.okWash,
    borderColor: palette.okBorder,
  },
  statusCompletedText: {
    color: palette.ok,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  statusRunning: {
    backgroundColor: palette.brandWashStrong,
    borderColor: palette.brandDeep,
  },
  statusRunningText: {
    color: palette.brand,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  statusPaused: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairlineStrong,
  },
  statusPausedText: {
    color: palette.ink1,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  statusError: {
    backgroundColor: palette.errWash,
    borderColor: palette.errBorder,
  },
  statusErrorText: {
    color: palette.err,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  pulseDot: {
    backgroundColor: palette.brand,
    borderRadius: radius.xxs,
    height: 6,
    width: 6,
  },

  /* 复合卡片外框 */
  cardBox: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.md,
    borderBottomLeftRadius: radius.xs,
    borderWidth: 1,
    overflow: "hidden",
    ...shadow.float,
  },

  /* 顶部耗时与交错过程流 (Codex 风格) */
  processContainer: {
    backgroundColor: palette.surface2,
    paddingHorizontal: 12,
    paddingTop: 10,
    paddingBottom: 6,
  },
  durationHeader: {
    alignItems: "center",
    borderBottomColor: palette.hairline,
    borderBottomWidth: 1,
    flexDirection: "row",
    gap: 6,
    paddingBottom: 8,
  },
  durationHeaderText: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  durationChevron: {
    color: palette.ink2,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  interleavedBody: {
    gap: 10,
    paddingTop: 10,
    paddingBottom: 4,
  },
  thoughtBlock: {
    paddingVertical: 1,
  },

  /* 工具调用单行与折叠组 */
  toolGroupBlock: {
    gap: 4,
  },
  toolGroupChildren: {
    gap: 4,
  },
  toolStepContainer: {
    gap: 6,
  },
  toolInlineRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    paddingVertical: 3,
  },
  termIconBadge: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.xs,
    borderWidth: 1,
    height: 18,
    justifyContent: "center",
    minWidth: 20,
    paddingHorizontal: 3,
  },
  termIconBadgeError: {
    backgroundColor: palette.errWash,
    borderColor: palette.errBorder,
  },
  termIconBadgeRunning: {
    backgroundColor: palette.brandWashStrong,
    borderColor: palette.brandDeep,
  },
  termIconText: {
    color: palette.ink1,
    fontFamily: monoFont,
    fontSize: font.micro,
    fontWeight: weight.bold,
    lineHeight: 11,
  },
  termIconTextError: {
    color: palette.err,
  },
  termIconTextRunning: {
    color: palette.brand,
  },
  toolInlineGroupTitle: {
    color: palette.ink1,
    flex: 1,
    fontSize: font.body,
    fontWeight: weight.semibold,
  },
  toolInlineText: {
    color: palette.ink1,
    flex: 1,
    fontSize: font.body,
    fontWeight: weight.medium,
  },
  toolInlineTextError: {
    color: palette.err,
  },
  toolInlineDuration: {
    color: palette.ink2,
    fontFamily: monoFont,
    fontSize: font.caption,
  },
  toolInlineChevron: {
    color: palette.ink2,
    fontSize: font.body,
    fontWeight: weight.bold,
    paddingHorizontal: 2,
  },

  /* 工具详情上下分栏面板 (方案 A：调用请求 + 执行返回) */
  shellBox: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.md,
    borderWidth: 1,
    marginTop: 2,
    overflow: "hidden",
  },
  toolSectionBlock: {
    paddingBottom: 2,
  },
  toolSectionDivider: {
    backgroundColor: palette.surface3,
    height: 1,
  },
  shellHeaderBar: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 10,
    paddingTop: 7,
    paddingBottom: 4,
  },
  toolSectionHeaderLeft: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 6,
    marginRight: 8,
  },
  toolReqBadge: {
    backgroundColor: palette.surface3,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.xs,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  toolReqBadgeText: {
    color: palette.ink1,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  toolResBadge: {
    borderRadius: radius.xs,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  toolResBadgeSuccess: {
    backgroundColor: palette.okWash,
    borderColor: palette.okBorder,
  },
  toolResBadgeRunning: {
    backgroundColor: palette.brandWash,
    borderColor: palette.brandDeep,
  },
  toolResBadgeError: {
    backgroundColor: palette.errWash,
    borderColor: palette.errBorder,
  },
  toolResBadgeText: {
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  toolResBadgeTextSuccess: {
    color: palette.ok,
  },
  toolResBadgeTextRunning: {
    color: palette.brand,
  },
  toolResBadgeTextError: {
    color: palette.err,
  },
  toolErrorCodeText: {
    color: palette.err,
    fontFamily: monoFont,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  toolTruncatedTag: {
    color: palette.brand,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  shellHeaderTitle: {
    color: palette.ink1,
    flex: 1,
    fontFamily: monoFont,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  shellHeaderMeta: {
    color: palette.ink2,
    fontFamily: monoFont,
    fontSize: font.caption,
  },
  toolRequestScroll: {
    maxHeight: 150,
    paddingHorizontal: 10,
    paddingBottom: 8,
  },
  shellScroll: {
    maxHeight: 240,
    paddingHorizontal: 10,
    paddingBottom: 10,
  },
  shellCommandText: {
    color: palette.ink1,
    fontFamily: monoFont,
    fontSize: font.caption,
    fontWeight: weight.semibold,
    lineHeight: 17,
  },
  shellOutputText: {
    color: palette.ink1,
    fontFamily: monoFont,
    fontSize: font.caption,
    lineHeight: 17,
  },
  shellOutputTextError: {
    color: palette.err,
  },
  shellEmptyText: {
    color: palette.ink2,
    fontFamily: monoFont,
    fontSize: font.caption,
    fontStyle: "italic",
  },
  shellAssetWrap: {
    marginTop: 4,
  },

  /* 关联文件变更审查 */
  reviewSection: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 6,
    marginTop: 4,
    padding: 8,
  },
  reviewHeaderRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  reviewTitle: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  reviewBadge: {
    backgroundColor: palette.okWash,
    borderColor: palette.hairline,
    borderRadius: radius.xs,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  reviewBadgeText: {
    color: palette.ok,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  reviewList: {
    gap: 4,
  },
  reviewItem: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.xs,
    borderWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 7,
    paddingVertical: 4,
  },
  reviewItemLeft: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 6,
    marginRight: 8,
  },
  reviewActionTag: {
    borderRadius: radius.xxs,
    borderWidth: 1,
    paddingHorizontal: 4,
    paddingVertical: 0.5,
  },
  reviewActionTagMod: {
    backgroundColor: palette.brandWashStrong,
    borderColor: palette.brandDeep,
  },
  reviewActionTagNew: {
    backgroundColor: palette.okWash,
    borderColor: palette.okBorder,
  },
  reviewActionTagText: {
    color: palette.ink1,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  reviewFilePath: {
    color: palette.ink1,
    flex: 1,
    fontFamily: monoFont,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  reviewDiffHint: {
    color: palette.ink2,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },

  /* 最终回答主体 */
  cardBody: {
    gap: 6,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  cardBodyWithProcess: {
    borderTopColor: palette.hairline,
    borderTopWidth: 1,
  },
  attachments: {
    gap: 6,
    marginTop: 6,
  },

  /* 底部元数据栏 */
  cardFooter: {
    alignItems: "center",
    borderTopColor: palette.hairline,
    borderTopWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  footerActions: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  actionBtn: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.xs,
    borderWidth: 1,
    flexDirection: "row",
    gap: 4,
    justifyContent: "center",
    paddingHorizontal: 7,
    paddingVertical: 3,
  },
  actionBtnText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  regenBtn: {
    backgroundColor: palette.cyanWash,
    borderColor: palette.hairline,
  },
  regenBtnText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  tokenStat: {
    alignItems: "center",
    flexDirection: "row",
    gap: 5,
  },
  tokenBadge: {
    backgroundColor: palette.brandWashStrong,
    borderColor: palette.brandDeep,
    borderRadius: radius.xs,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  tokenBadgeText: {
    color: palette.brand,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  tokenBreakdownText: {
    color: palette.ink2,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  subagentSummaryCard: {
    backgroundColor: palette.okWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 6,
    marginTop: 4,
    padding: 10,
  },
  subagentSummaryHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  subagentSummaryTitleRow: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 6,
  },
  subagentIcon: {
    fontSize: font.body,
  },
  subagentSummaryName: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  subagentStatusPill: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.pill,
    borderWidth: 1,
    paddingHorizontal: 6,
    paddingVertical: 1,
  },
  subagentStatusPillText: {
    color: palette.ink1,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  subagentStatusRunning: {
    backgroundColor: palette.cyanWash,
  },
  subagentStatusSucceeded: {
    backgroundColor: palette.okWash,
  },
  subagentStatusFailed: {
    backgroundColor: palette.errWash,
  },
  subagentStatusWaiting: {
    backgroundColor: palette.brandWash,
  },
  inspectSubagentBtn: {
    alignItems: "center",
    backgroundColor: palette.surface3,
    borderRadius: radius.sm,
    flexDirection: "row",
    gap: 4,
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
  inspectSubagentBtnText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  subagentSummarySnippet: {
    color: palette.ink2,
    fontSize: font.caption,
    lineHeight: 16,
  },
  subagentSpawningCard: {
    alignItems: "center",
    backgroundColor: palette.cyanWash,
    borderColor: palette.okBorder,
    borderRadius: radius.sm,
    borderStyle: "dashed",
    borderWidth: 1,
    flexDirection: "row",
    gap: 8,
    marginTop: 4,
    padding: 8,
  },
  subagentSpawningText: {
    color: palette.info,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
});

function formatSubagentStatus(status?: string): string {
  switch (status) {
    case "queued":
      return "排队中";
    case "running":
      return "执行中";
    case "waiting_subagents":
      return "等待子任务";
    case "waiting_permission":
      return "等待授权";
    case "succeeded":
      return "已完成";
    case "failed":
      return "失败";
    case "canceled":
      return "已取消";
    default:
      return status || "运行中";
  }
}
