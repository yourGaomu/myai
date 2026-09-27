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

function extractToolCommand(tool: ToolCallStep): string {
  if (!tool.arguments) return "";
  const raw = String(tool.arguments).trim();
  if (!raw) return "";
  try {
    const parsed = typeof tool.arguments === "string" ? JSON.parse(raw) : tool.arguments;
    if (typeof parsed === "string") {
      return parsed.trim();
    }
    if (parsed && typeof parsed === "object") {
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
        parsed.toolSummary;
      if (candidate !== undefined && candidate !== null && String(candidate).trim() !== "") {
        return String(candidate).trim();
      }
    }
  } catch {
    return raw;
  }
  return raw;
}

function formatSingleLineCommand(cmd: string): string {
  return cmd.replace(/\s+/g, " ").trim();
}

function formatToolActionRowTitle(tool: ToolCallStep): string {
  const cmd = formatSingleLineCommand(extractToolCommand(tool));
  const lower = tool.name.toLowerCase();

  if (tool.status === "running") {
    return cmd ? `正在运行 ${cmd}` : `正在运行 ${tool.name}`;
  }
  if (tool.status === "error" || tool.error) {
    return cmd ? `运行失败 ${cmd}` : `运行失败 ${tool.name}`;
  }

  if (isShellLikeTool(tool.name)) {
    return cmd ? `已运行 ${cmd}` : "运行了命令";
  }
  if (lower.includes("read") || lower.includes("view")) {
    return cmd ? `已读取 ${cmd}` : `已调用 ${tool.name}`;
  }
  if (lower.includes("write") || lower.includes("edit") || lower.includes("replace") || lower.includes("patch")) {
    return cmd ? `已修改 ${cmd}` : `已调用 ${tool.name}`;
  }
  if (lower.includes("search") || lower.includes("grep") || lower.includes("find") || lower.includes("glob")) {
    return cmd ? `已搜索 ${cmd}` : `已调用 ${tool.name}`;
  }
  return cmd ? `已运行 ${ cmd }` : `运行了 ${tool.name}`;
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

  return (
    <View style={styles.toolGroupBlock}>
      <Pressable
        onPress={() => setGroupOpen((prev) => !prev)}
        style={({ pressed }) => buttonFeedback(styles.toolInlineRow, pressed)}
      >
        <View style={styles.termIconBadge}>
          <Text style={styles.termIconText}>{">_"}</Text>
        </View>
        <Text numberOfLines={1} style={styles.toolInlineGroupTitle}>
          运行了命令
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
  const [expanded, setExpanded] = useState(isFailed);
  const sharedAsset = parseSharedAsset(tool.name, tool.result || "");
  const commandText = extractToolCommand(tool);
  const rowTitle = formatToolActionRowTitle(tool);
  const shellHeaderLabel = isShellLikeTool(tool.name) ? "Shell" : tool.name;
  const outputText = tool.error || tool.result || "";

  return (
    <View style={styles.toolStepContainer}>
      <Pressable
        onPress={() => setExpanded((prev) => !prev)}
        style={({ pressed }) => buttonFeedback(styles.toolInlineRow, pressed)}
      >
        <View style={[styles.termIconBadge, isFailed && styles.termIconBadgeError]}>
          <Text style={[styles.termIconText, isFailed && styles.termIconTextError]}>{">_"}</Text>
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
          <View style={styles.shellHeaderBar}>
            <Text style={styles.shellHeaderTitle}>{shellHeaderLabel}</Text>
            {tool.duration ? <Text style={styles.shellHeaderMeta}>{tool.duration}</Text> : null}
          </View>

          <ScrollView
            nestedScrollEnabled
            showsVerticalScrollIndicator
            style={styles.shellScroll}
          >
            {commandText ? (
              <Text selectable style={styles.shellCommandText}>
                {`$ ${commandText}`}
              </Text>
            ) : null}

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
                {tool.status === "running" ? "正在执行命令..." : "(无输出内容)"}
              </Text>
            )}
          </ScrollView>
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
  toolInlineGroupTitle: {
    color: "#6b665e",
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

  /* Shell 灰底展开面板 */
  shellBox: {
    backgroundColor: "#f4f3ef",
    borderColor: "#e2dfd7",
    borderRadius: 10,
    borderWidth: 1,
    marginTop: 2,
    overflow: "hidden",
  },
  shellHeaderBar: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 12,
    paddingTop: 8,
    paddingBottom: 4,
  },
  shellHeaderTitle: {
    color: "#6e6a63",
    fontSize: 12,
    fontWeight: "600",
  },
  shellHeaderMeta: {
    color: "#8c867c",
    fontFamily: monoFont,
    fontSize: 11,
  },
  shellScroll: {
    maxHeight: 240,
    paddingHorizontal: 12,
    paddingBottom: 10,
  },
  shellCommandText: {
    color: "#2c2a26",
    fontFamily: monoFont,
    fontSize: 12,
    fontWeight: "600",
    lineHeight: 18,
    marginBottom: 6,
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
