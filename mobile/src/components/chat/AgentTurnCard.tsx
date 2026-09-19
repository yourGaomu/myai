import { useMemo, useState } from "react";
import { Platform, Pressable, StyleSheet, Text, View } from "react-native";

import type { AgentTurnItem, ToolCallStep } from "../../utils/chatRenderItems";
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

function extractToolParamSummary(tool: ToolCallStep): string {
  if (!tool.arguments) return "";
  try {
    const parsed = typeof tool.arguments === "string" ? JSON.parse(tool.arguments) : tool.arguments;
    if (parsed.CommandLine) return String(parsed.CommandLine);
    if (parsed.TargetFile) {
      const parts = String(parsed.TargetFile).split(/[/\\]/);
      return parts.pop() || "";
    }
    if (parsed.AbsolutePath) {
      const parts = String(parsed.AbsolutePath).split(/[/\\]/);
      return parts.pop() || "";
    }
    if (parsed.Query) return `"${parsed.Query}"`;
    if (parsed.Pattern) return `"${parsed.Pattern}"`;
    if (parsed.Url) return String(parsed.Url);
    if (parsed.toolSummary) return String(parsed.toolSummary);
  } catch {
    return String(tool.arguments).replace(/[\r\n\t]/g, " ").slice(0, 40);
  }
  return "";
}

function toolKindIcon(name: string): string {
  if (name.includes("command") || name.includes("terminal")) return "⚡";
  if (name.includes("write") || name.includes("replace")) return "📝";
  if (name.includes("file") || name.includes("read") || name.includes("view")) return "📄";
  if (name.includes("search") || name.includes("grep") || name.includes("find")) return "🔍";
  if (name.includes("image")) return "🎨";
  if (name.includes("subagent")) return "🤖";
  return "🛠️";
}

function extractFileChanges(tools: ToolCallStep[] | undefined): FileChangeTag[] {
  if (!tools) return [];
  const files: FileChangeTag[] = [];
  const seen = new Set<string>();
  for (const t of tools) {
    if (t.name === "write_to_file" || t.name === "replace_file_content") {
      try {
        const parsed = typeof t.arguments === "string" ? JSON.parse(t.arguments) : t.arguments;
        const target = parsed.TargetFile || parsed.file_path || "";
        if (target && !seen.has(target)) {
          seen.add(target);
          files.push({
            action: t.name === "write_to_file" ? "NEW" : "MOD",
            path: target,
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
  const hasProcess = Boolean(turn.reasoning || (turn.tools && turn.tools.length > 0));
  const toolCount = turn.tools ? turn.tools.length : 0;
  const [trayOpen, setTrayOpen] = useState(isRunning);
  const [copied, setCopied] = useState(false);

  const tokensInfo = useMemo(() => usageBreakdown(turn.usage), [turn.usage]);
  const fileChanges = useMemo(() => extractFileChanges(turn.tools), [turn.tools]);

  const processSummary = useMemo(() => {
    const elapsedSuffix = turn.elapsed ? ` · 耗时 ${turn.elapsed}` : "";
    if (toolCount > 0) {
      return `已执行 ${toolCount} 个操作${elapsedSuffix}`;
    }
    if (turn.reasoning) {
      return `思考推导完成${elapsedSuffix}`;
    }
    return "执行活动";
  }, [toolCount, turn.elapsed, turn.reasoning]);

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
        {/* 顶部过程折叠托盘 (Process Tray) */}
        {hasProcess ? (
          <View style={styles.processTray}>
            <Pressable
              onPress={() => setTrayOpen((prev) => !prev)}
              style={({ pressed }) => buttonFeedback(styles.trayHeader, pressed)}
            >
              <View style={styles.traySummaryLeft}>
                <Text style={styles.trayBoltIcon}>⚡</Text>
                <Text numberOfLines={1} style={styles.traySummaryText}>
                  {processSummary}
                </Text>
              </View>
              <View style={styles.trayToggleBtn}>
                <Text style={styles.trayToggleBtnText}>{trayOpen ? "收起 ▲" : "展开详情 ▼"}</Text>
              </View>
            </Pressable>

            {trayOpen ? (
              <View style={styles.trayBody}>
                {/* 1. 思考推导过程 (Reasoning) */}
                {turn.reasoning ? (
                  <View style={styles.traySection}>
                    <Text style={styles.traySectionTitle}>🧠 思考推导过程 (REASONING)</Text>
                    <View style={styles.reasoningBox}>
                      <Text selectable style={styles.reasoningText}>
                        {turn.reasoning}
                      </Text>
                    </View>
                  </View>
                ) : null}

                {/* 2. 工具调用轨迹 (Tool Markers Timeline) */}
                {toolCount > 0 ? (
                  <View style={styles.traySection}>
                    <View style={styles.traySectionHeaderRow}>
                      <Text style={styles.traySectionTitle}>🛠️ 工具调用轨迹 ({toolCount} 步)</Text>
                      {turn.elapsed ? <Text style={styles.traySectionMeta}>耗时 {turn.elapsed}</Text> : null}
                    </View>
                    <View style={styles.toolList}>
                      {turn.tools.map((tool, idx) => (
                        <ToolStepItem
                          buttonFeedback={buttonFeedback}
                          index={idx + 1}
                          key={tool.id || `${tool.name}-${idx}`}
                          tool={tool}
                        />
                      ))}
                    </View>
                  </View>
                ) : null}

                {/* 3. 关联文件变更审查卡片 (File Changes Review Card) */}
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
                              <View style={[styles.reviewActionTag, file.action === "NEW" ? styles.reviewActionTagNew : styles.reviewActionTagMod]}>
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
        <View style={styles.cardBody}>
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

function ToolStepItem({
  buttonFeedback,
  index,
  tool,
}: {
  buttonFeedback: ButtonFeedback;
  index: number;
  tool: ToolCallStep;
}) {
  const [expanded, setExpanded] = useState(false);
  const sharedAsset = parseSharedAsset(tool.name, tool.result || "");
  const isFailed = Boolean(tool.error || tool.status === "error");
  const summary = extractToolParamSummary(tool);
  const icon = toolKindIcon(tool.name);

  return (
    <View style={styles.toolItem}>
      <Pressable
        onPress={() => setExpanded((prev) => !prev)}
        style={({ pressed }) => buttonFeedback(styles.toolHeader, pressed)}
      >
        <View style={[styles.toolStatusDot, isFailed && styles.toolStatusDotError]}>
          <Text style={styles.toolStatusDotText}>{isFailed ? "✕" : "✓"}</Text>
        </View>
        <Text style={styles.toolKindIcon}>{icon}</Text>
        <Text numberOfLines={1} style={styles.toolName}>
          {tool.name}
        </Text>
        {summary ? (
          <Text numberOfLines={1} style={styles.toolSummarySnippet}>
            {summary}
          </Text>
        ) : null}
        <Text style={styles.toolDuration}>{tool.duration || `#${index}`}</Text>
        <Text style={styles.toolArrow}>{expanded ? "▲" : "▼"}</Text>
      </Pressable>

      {expanded ? (
        <View style={styles.toolDetail}>
          {tool.arguments ? (
            <View style={styles.toolSection}>
              <Text style={styles.toolDetailLabel}>输入参数 (ARGUMENTS):</Text>
              <Text selectable style={styles.toolCode}>
                {tool.arguments}
              </Text>
            </View>
          ) : null}
          {tool.result ? (
            <View style={styles.toolSection}>
              <Text style={styles.toolDetailLabel}>{isFailed ? "错误信息 (ERROR):" : "控制台输出 (OUTPUT):"}</Text>
              {sharedAsset && !isFailed ? (
                <SharedAssetCard asset={sharedAsset} buttonFeedback={buttonFeedback} />
              ) : (
                <View style={styles.terminalBox}>
                  <Text selectable style={[styles.terminalText, isFailed && styles.terminalTextError]}>
                    {tool.result}
                  </Text>
                </View>
              )}
            </View>
          ) : null}
        </View>
      ) : null}
    </View>
  );
}

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

  /* 复合卡片外框 (Neo-Brutalism) */
  cardBox: {
    backgroundColor: "#fffdf7",
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

  /* 过程抽屉 */
  processTray: {
    backgroundColor: "#f8f4ec",
    borderBottomColor: "#e5dfd2",
    borderBottomWidth: 1,
  },
  trayHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  traySummaryLeft: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 6,
    marginRight: 8,
  },
  trayBoltIcon: {
    color: "#ffd84f",
    fontSize: 13,
  },
  traySummaryText: {
    color: "#49443c",
    fontSize: 11.5,
    fontWeight: "800",
  },
  trayToggleBtn: {
    backgroundColor: "#ece5d8",
    borderColor: "#dcd4c5",
    borderRadius: 4,
    borderWidth: 1,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  trayToggleBtnText: {
    color: "#6c665f",
    fontSize: 10,
    fontWeight: "800",
  },
  trayBody: {
    backgroundColor: "#fffdfa",
    borderTopColor: "#e8e2d7",
    borderTopWidth: 1,
    gap: 8,
    padding: 10,
  },
  traySection: {
    gap: 4,
  },
  traySectionTitle: {
    color: "#7a7367",
    fontSize: 10,
    fontWeight: "900",
    letterSpacing: 0.3,
  },
  reasoningBox: {
    backgroundColor: "#f5f0e6",
    borderLeftColor: "#ffd84f",
    borderLeftWidth: 3,
    borderRadius: 4,
    padding: 8,
  },
  reasoningText: {
    color: "#4c463d",
    fontFamily: Platform.OS === "ios" ? "Menlo" : "monospace",
    fontSize: 11,
    lineHeight: 16,
  },
  traySectionHeaderRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  traySectionMeta: {
    color: "#8c857b",
    fontSize: 10,
    fontWeight: "700",
  },
  toolList: {
    gap: 4,
  },
  toolItem: {
    backgroundColor: "#f7f3eb",
    borderColor: "#ded5c6",
    borderRadius: 6,
    borderWidth: 1,
    overflow: "hidden",
  },
  toolHeader: {
    alignItems: "center",
    backgroundColor: "#f0e9dc",
    flexDirection: "row",
    gap: 5,
    paddingHorizontal: 8,
    paddingVertical: 6,
  },
  toolStatusDot: {
    alignItems: "center",
    backgroundColor: "#b9e9b0",
    borderColor: "#25231f",
    borderRadius: 3,
    borderWidth: 1,
    height: 14,
    justifyContent: "center",
    width: 14,
  },
  toolStatusDotError: {
    backgroundColor: "#ff7f68",
  },
  toolStatusDotText: {
    color: "#12100e",
    fontSize: 9,
    fontWeight: "900",
    lineHeight: 11,
  },
  toolKindIcon: {
    fontSize: 11,
  },
  toolName: {
    color: "#12100e",
    fontFamily: Platform.OS === "ios" ? "Menlo" : "monospace",
    fontSize: 11,
    fontWeight: "900",
  },
  toolSummarySnippet: {
    color: "#6c665f",
    flex: 1,
    fontFamily: Platform.OS === "ios" ? "Menlo" : "monospace",
    fontSize: 10,
    marginHorizontal: 3,
  },
  toolDuration: {
    color: "#8c857b",
    fontFamily: Platform.OS === "ios" ? "Menlo" : "monospace",
    fontSize: 9.5,
    fontWeight: "700",
  },
  toolArrow: {
    color: "#7d7568",
    fontSize: 9.5,
  },
  toolDetail: {
    backgroundColor: "#ffffff",
    borderTopColor: "#ded5c6",
    borderTopWidth: 1,
    gap: 6,
    padding: 8,
  },
  toolSection: {
    gap: 3,
  },
  toolDetailLabel: {
    color: "#6c665f",
    fontSize: 9.5,
    fontWeight: "800",
    letterSpacing: 0.2,
  },
  toolCode: {
    backgroundColor: "#f5f2eb",
    borderColor: "#ded5c6",
    borderRadius: 4,
    borderWidth: 1,
    color: "#12100e",
    fontFamily: Platform.OS === "ios" ? "Menlo" : "monospace",
    fontSize: 10.5,
    lineHeight: 15,
    padding: 6,
  },
  terminalBox: {
    backgroundColor: "#12100e",
    borderColor: "#25231f",
    borderRadius: 6,
    borderWidth: 1.5,
    padding: 8,
  },
  terminalText: {
    color: "#50fa7b",
    fontFamily: Platform.OS === "ios" ? "Menlo" : "monospace",
    fontSize: 10.5,
    lineHeight: 15,
  },
  terminalTextError: {
    color: "#ff7f68",
  },
  reviewSection: {
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 1.5,
    gap: 6,
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
    fontFamily: Platform.OS === "ios" ? "Menlo" : "monospace",
    fontSize: 10.5,
    fontWeight: "700",
  },
  reviewDiffHint: {
    color: "#6c665f",
    fontSize: 10,
    fontWeight: "800",
  },
  toolErrorCode: {
    backgroundColor: "#fff0ee",
    color: "#9b4037",
  },

  /* 回答主体 */
  cardBody: {
    gap: 6,
    paddingHorizontal: 12,
    paddingVertical: 10,
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
