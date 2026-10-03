import { useMemo, useState } from "react";
import { Platform, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import type { AgentTurnItem, AgentTurnTimelineStep, ToolCallStep } from "../../utils/chatRenderItems";
import type { ButtonFeedback } from "../../types/ui";
import { parseSharedAsset } from "../../utils/toolAssets";
import { usageBreakdown } from "../../utils/tokenUsage";
import { ChatAttachmentCard } from "./ChatAttachmentCard";
import { MarkdownText } from "./MarkdownText";
import { SharedAssetCard } from "./SharedAssetCard";

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
};

export function AgentTurnCard({ buttonFeedback, onRegenerate, turn }: Props) {
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
      return turn.elapsed ? `正在思考与执行 · ${turn.elapsed}` : "正在思考与执行...";
    }
    if (turn.elapsed) {
      return `用时 ${turn.elapsed}`;
    }
    if (toolCount > 0) {
      return `已执行 ${toolCount} 个命令与操作`;
    }
    return "思考推导过程";
  }, [isRunning, toolCount, turn.elapsed]);

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
            <Text style={styles.avatarEmoji}>🤖</Text>
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
            <Text style={styles.statusPausedText}>⏸ 已暂停{turn.elapsed ? ` · ${turn.elapsed}` : ""}</Text>
          ) : (
            <Text style={styles.statusCompletedText}>✓ 完成{turn.elapsed ? ` · ${turn.elapsed}` : ""}</Text>
          )}
        </View>
      </View>

      {/* 复合气泡大卡片 */}
      <View style={styles.cardBox}>
        {/* 顶部耗时折叠头 + 交错式过程时间流 (思考段落 ↔ 运行了命令 / Shell 面板) */}
        {hasProcess ? (
          <View style={styles.processContainer}>
            <Pressable
              onPress={() => setTrayOpen((prev) => !prev)}
              style={({ pressed }) => buttonFeedback(styles.durationHeader, pressed)}
            >
              <Text style={styles.durationHeaderText}>{durationHeaderLabel}</Text>
              <Text style={styles.durationChevron}>{trayOpen ? "⌄" : "›"}</Text>
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
                      tools={step.tools}
                    />
                  );
                })}

                {/* 关联文件变更审查卡片 */}
                {fileChanges.length > 0 ? (
                  <View style={styles.reviewSection}>
                    <View style={styles.reviewHeaderRow}>
                      <Text style={styles.reviewTitle}>📝 关联文件变更 ({fileChanges.length} 个文件)</Text>
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
                            <Text style={styles.reviewDiffHint}>查看 ❯</Text>
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
              <Text style={styles.actionBtnText}>{copied ? "✓ 已复制" : "📋 复制"}</Text>
            </Pressable>
            {turn.canRegenerate && onRegenerate ? (
              <Pressable
                onPress={onRegenerate}
                style={({ pressed }) => buttonFeedback([styles.actionBtn, styles.regenBtn], pressed)}
              >
                <Text style={styles.regenBtnText}>🔄 重新生成</Text>
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
}: {
  buttonFeedback: ButtonFeedback;
  tools: ToolCallStep[];
}) {
  const [groupOpen, setGroupOpen] = useState(true);

  if (tools.length === 1) {
    return <InterleavedToolRow buttonFeedback={buttonFeedback} tool={tools[0]} />;
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
        <Text style={styles.toolInlineChevron}>{groupOpen ? "⌄" : "›"}</Text>
      </Pressable>

      {groupOpen ? (
        <View style={styles.toolGroupChildren}>
          {tools.map((tool, index) => (
            <InterleavedToolRow
              buttonFeedback={buttonFeedback}
              key={tool.id || `${tool.name}-${index}`}
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
}: {
  buttonFeedback: ButtonFeedback;
  tool: ToolCallStep;
}) {
  const isFailed = Boolean(tool.error || tool.status === "error");
  const isRunning = tool.status === "running";
  const [expanded, setExpanded] = useState(isFailed);
  const sharedAsset = parseSharedAsset(tool.name, tool.result || "");
  const requestText = useMemo(() => formatToolRequestText(tool), [tool]);
  const rowTitle = useMemo(() => formatToolActionRowTitle(tool), [tool]);
  const outputText = tool.error || tool.result || "";

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
        <Text style={styles.toolInlineChevron}>{expanded ? "⌄" : "›"}</Text>
      </Pressable>

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
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 5,
    borderWidth: 1.5,
    height: 22,
    justifyContent: "center",
    width: 22,
  },
  avatarEmoji: {
    fontSize: 12,
  },
  agentName: {
    color: "#12100e",
    fontSize: 12.5,
    fontWeight: "900",
  },
  modelPill: {
    backgroundColor: "#ede7da",
    borderColor: "#d4ccbd",
    borderRadius: 4,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  modelPillText: {
    color: "#555047",
    fontSize: 9.5,
    fontWeight: "800",
  },
  statusBadge: {
    alignItems: "center",
    borderRadius: 10,
    borderWidth: 1,
    flexDirection: "row",
    gap: 4,
    paddingHorizontal: 7,
    paddingVertical: 2,
  },
  statusCompleted: {
    backgroundColor: "#b9e9b0",
    borderColor: "#0e4e16",
  },
  statusCompletedText: {
    color: "#0e4e16",
    fontSize: 10,
    fontWeight: "800",
  },
  statusRunning: {
    backgroundColor: "#ffd84f",
    borderColor: "#b58c00",
  },
  statusRunningText: {
    color: "#684d00",
    fontSize: 10,
    fontWeight: "800",
  },
  statusPaused: {
    backgroundColor: "#ede7da",
    borderColor: "#7a7267",
  },
  statusPausedText: {
    color: "#4a453e",
    fontSize: 10,
    fontWeight: "800",
  },
  statusError: {
    backgroundColor: "#ff7f68",
    borderColor: "#d56c5b",
  },
  statusErrorText: {
    color: "#7a1f1a",
    fontSize: 10,
    fontWeight: "800",
  },
  pulseDot: {
    backgroundColor: "#b58c00",
    borderRadius: 3,
    height: 6,
    width: 6,
  },

  /* 复合卡片外框 */
  cardBox: {
    backgroundColor: "#ffffff",
    borderColor: "#12100e",
    borderRadius: 14,
    borderBottomLeftRadius: 4,
    borderWidth: 1.5,
    elevation: 2,
    overflow: "hidden",
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },

  /* 顶部耗时与交错过程流 (Codex 风格) */
  processContainer: {
    backgroundColor: "#ffffff",
    paddingHorizontal: 12,
    paddingTop: 10,
    paddingBottom: 6,
  },
  durationHeader: {
    alignItems: "center",
    borderBottomColor: "#eceae4",
    borderBottomWidth: 1,
    flexDirection: "row",
    gap: 6,
    paddingBottom: 8,
  },
  durationHeaderText: {
    color: "#5e5a53",
    fontSize: 13,
    fontWeight: "700",
  },
  durationChevron: {
    color: "#7a756c",
    fontSize: 13,
    fontWeight: "700",
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
    backgroundColor: "#f7f6f2",
    borderColor: "#8c877e",
    borderRadius: 4,
    borderWidth: 1.2,
    height: 18,
    justifyContent: "center",
    minWidth: 20,
    paddingHorizontal: 3,
  },
  termIconBadgeError: {
    backgroundColor: "#fff0ee",
    borderColor: "#c94a3f",
  },
  termIconBadgeRunning: {
    backgroundColor: "#fff8db",
    borderColor: "#b58c00",
  },
  termIconText: {
    color: "#5e5a53",
    fontFamily: monoFont,
    fontSize: 9.5,
    fontWeight: "900",
    lineHeight: 11,
  },
  termIconTextError: {
    color: "#c94a3f",
  },
  termIconTextRunning: {
    color: "#8a6800",
  },
  toolInlineGroupTitle: {
    color: "#6b665e",
    flex: 1,
    fontSize: 13,
    fontWeight: "600",
  },
  toolInlineText: {
    color: "#6b665e",
    flex: 1,
    fontSize: 13,
    fontWeight: "500",
  },
  toolInlineTextError: {
    color: "#b83b30",
  },
  toolInlineDuration: {
    color: "#9a948a",
    fontFamily: monoFont,
    fontSize: 11,
  },
  toolInlineChevron: {
    color: "#7a756c",
    fontSize: 13,
    fontWeight: "700",
    paddingHorizontal: 2,
  },

  /* 工具详情上下分栏面板 (方案 A：调用请求 + 执行返回) */
  shellBox: {
    backgroundColor: "#f4f3ef",
    borderColor: "#dbd6ca",
    borderRadius: 10,
    borderWidth: 1,
    marginTop: 2,
    overflow: "hidden",
  },
  toolSectionBlock: {
    paddingBottom: 2,
  },
  toolSectionDivider: {
    backgroundColor: "#e2dfd7",
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
    backgroundColor: "#e6e1d6",
    borderColor: "#b8b0a2",
    borderRadius: 4,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  toolReqBadgeText: {
    color: "#4a453e",
    fontSize: 10,
    fontWeight: "800",
  },
  toolResBadge: {
    borderRadius: 4,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  toolResBadgeSuccess: {
    backgroundColor: "#dcf5d6",
    borderColor: "#6ab85e",
  },
  toolResBadgeRunning: {
    backgroundColor: "#fff3bf",
    borderColor: "#d4a91e",
  },
  toolResBadgeError: {
    backgroundColor: "#ffe3de",
    borderColor: "#d96b5c",
  },
  toolResBadgeText: {
    fontSize: 10,
    fontWeight: "800",
  },
  toolResBadgeTextSuccess: {
    color: "#1b5e20",
  },
  toolResBadgeTextRunning: {
    color: "#7a5900",
  },
  toolResBadgeTextError: {
    color: "#9c2418",
  },
  toolErrorCodeText: {
    color: "#b83b30",
    fontFamily: monoFont,
    fontSize: 10.5,
    fontWeight: "700",
  },
  toolTruncatedTag: {
    color: "#8c6d1f",
    fontSize: 10,
    fontWeight: "700",
  },
  shellHeaderTitle: {
    color: "#4a463f",
    flex: 1,
    fontFamily: monoFont,
    fontSize: 11.5,
    fontWeight: "700",
  },
  shellHeaderMeta: {
    color: "#8c867c",
    fontFamily: monoFont,
    fontSize: 11,
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
    color: "#2c2a26",
    fontFamily: monoFont,
    fontSize: 11.5,
    fontWeight: "600",
    lineHeight: 17,
  },
  shellOutputText: {
    color: "#57534c",
    fontFamily: monoFont,
    fontSize: 11.5,
    lineHeight: 17,
  },
  shellOutputTextError: {
    color: "#b83b30",
  },
  shellEmptyText: {
    color: "#8c867c",
    fontFamily: monoFont,
    fontSize: 11.5,
    fontStyle: "italic",
  },
  shellAssetWrap: {
    marginTop: 4,
  },

  /* 关联文件变更审查 */
  reviewSection: {
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 1.5,
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
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  reviewBadge: {
    backgroundColor: "#b9e9b0",
    borderColor: "#12100e",
    borderRadius: 4,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  reviewBadgeText: {
    color: "#0e4e16",
    fontSize: 9.5,
    fontWeight: "800",
  },
  reviewList: {
    gap: 4,
  },
  reviewItem: {
    alignItems: "center",
    backgroundColor: "#f8f5ee",
    borderColor: "#ded5c6",
    borderRadius: 5,
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
    borderRadius: 3,
    borderWidth: 1,
    paddingHorizontal: 4,
    paddingVertical: 0.5,
  },
  reviewActionTagMod: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
  },
  reviewActionTagNew: {
    backgroundColor: "#b9e9b0",
    borderColor: "#12100e",
  },
  reviewActionTagText: {
    color: "#12100e",
    fontSize: 9,
    fontWeight: "900",
  },
  reviewFilePath: {
    color: "#12100e",
    flex: 1,
    fontFamily: monoFont,
    fontSize: 10.5,
    fontWeight: "700",
  },
  reviewDiffHint: {
    color: "#6c665f",
    fontSize: 10,
    fontWeight: "800",
  },

  /* 最终回答主体 */
  cardBody: {
    gap: 6,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  cardBodyWithProcess: {
    borderTopColor: "#eceae4",
    borderTopWidth: 1,
  },
  attachments: {
    gap: 6,
    marginTop: 6,
  },

  /* 底部元数据栏 */
  cardFooter: {
    alignItems: "center",
    borderTopColor: "#f0eae1",
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
    backgroundColor: "#f2ecdf",
    borderColor: "#d7cfc2",
    borderRadius: 5,
    borderWidth: 1,
    justifyContent: "center",
    paddingHorizontal: 7,
    paddingVertical: 3,
  },
  actionBtnText: {
    color: "#49443c",
    fontSize: 10.5,
    fontWeight: "800",
  },
  regenBtn: {
    backgroundColor: "#4fd7ee",
    borderColor: "#12100e",
  },
  regenBtnText: {
    color: "#12100e",
    fontSize: 10.5,
    fontWeight: "900",
  },
  tokenStat: {
    alignItems: "center",
    flexDirection: "row",
    gap: 5,
  },
  tokenBadge: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 4,
    borderWidth: 1,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  tokenBadgeText: {
    color: "#12100e",
    fontSize: 10.5,
    fontWeight: "900",
  },
  tokenBreakdownText: {
    color: "#7d7568",
    fontSize: 10,
    fontWeight: "700",
  },
});
