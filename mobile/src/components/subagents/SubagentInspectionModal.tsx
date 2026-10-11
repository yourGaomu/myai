import { useMemo, useState } from "react";
import {
  ActivityIndicator,
  Modal,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from "react-native";

import type {
  AssetSummary,
  AgentRunSnapshot,
  SessionHistoryMessage,
  SubagentRunSummary,
  SubagentTask,
  SubagentTaskResultPayload,
} from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { assetSummaryToSharedAsset } from "../../utils/toolAssets";
import { formatDateTime } from "../../utils/format";
import { MarkdownText } from "../chat/MarkdownText";
import { SharedAssetCard } from "../chat/SharedAssetCard";

type Props = {
  buttonFeedback: ButtonFeedback;
  visible: boolean;
  onClose: () => void;
  onRefresh: () => void;
  task: SubagentTask | null;
  runs: SubagentRunSummary[];
  agentRuns: AgentRunSnapshot[];
  messages: SessionHistoryMessage[];
  assets: AssetSummary[];
  taskEvents: SubagentTaskResultPayload[];
  sessionReady: boolean;
  readOnly: boolean;
  loadingDetail: boolean;
  loadingEvents: boolean;
  loadingMoreMessages: boolean;
  hasMoreMessages: boolean;
  onLoadMoreMessages: () => void;
  selectedRunID: string | null;
  onSelectRun: (runID: string) => void;
};

type TabType = "process" | "messages" | "assets";

type AggregatedToolItem = {
  toolCallID: string;
  name: string;
  arguments: string;
  status: "running" | "success" | "failed" | "denied";
  result?: string;
  errorCode?: string;
  errorMessage?: string;
  truncated?: boolean;
};

export function SubagentInspectionModal({
  buttonFeedback,
  visible,
  onClose,
  onRefresh,
  task,
  runs,
  messages,
  assets,
  taskEvents,
  sessionReady,
  readOnly,
  loadingDetail,
  loadingEvents,
  loadingMoreMessages,
  hasMoreMessages,
  onLoadMoreMessages,
  selectedRunID,
  onSelectRun,
}: Props) {
  const [activeTab, setActiveTab] = useState<TabType>("process");
  const [thoughtExpanded, setThoughtExpanded] = useState(true);

  // 1.0 过滤当前选中的 Run 对应的事件（若指定 selectedRunID）
  const filteredEvents = useMemo(() => {
    if (!selectedRunID) return taskEvents;
    return taskEvents.filter((ev) => !ev.run_id || ev.run_id === selectedRunID);
  }, [selectedRunID, taskEvents]);

  // 2.0 聚合实时过程：提取思考、回答正文、工具调用配对、权限拒绝和创建的资产
  const processViewModel = useMemo(() => {
    let reasoningText = "";
    let answerText = "";
    const toolsMap = new Map<string, AggregatedToolItem>();
    const toolOrder: string[] = [];
    const eventAssets: AssetSummary[] = [];
    const permissionDenials: { toolCallID: string; toolName: string; reason: string }[] = [];
    let isWaiting = false;
    let waitingStatus = "";

    for (const ev of filteredEvents) {
      if (ev.kind === "task.reasoning" && ev.content) {
        reasoningText = ev.delta ? reasoningText + ev.content : ev.content;
      } else if (ev.kind === "task.answer" && ev.content) {
        answerText = ev.delta ? answerText + ev.content : ev.content;
      } else if (ev.kind === "task.tool_call") {
        const callID = ev.tool_call_id || `tool-${toolsMap.size}`;
        if (!toolsMap.has(callID)) {
          toolOrder.push(callID);
        }
        toolsMap.set(callID, {
          toolCallID: callID,
          name: ev.tool_name || "tool",
          arguments: ev.arguments || "",
          status: "running",
        });
      } else if (ev.kind === "task.tool_result") {
        const callID = ev.tool_call_id || toolOrder[toolOrder.length - 1] || "tool-0";
        const existing = toolsMap.get(callID) || {
          toolCallID: callID,
          name: ev.tool_name || "tool",
          arguments: ev.arguments || "",
          status: "running",
        };
        const failed =
          Boolean(ev.error_message || ev.error_code) ||
          Boolean(ev.status && ev.status !== "success" && ev.status !== "succeeded");
        existing.status = failed ? "failed" : "success";
        existing.result = ev.content;
        existing.errorCode = ev.error_code;
        existing.errorMessage = ev.error_message;
        existing.truncated = ev.truncated;
        if (!toolsMap.has(callID)) {
          toolOrder.push(callID);
        }
        toolsMap.set(callID, existing);
      } else if (ev.kind === "task.permission") {
        // 2.1 后台权限拒绝事件：记录原因，不显示交互审批按钮
        permissionDenials.push({
          toolCallID: ev.tool_call_id || "",
          toolName: ev.tool_name || "tool",
          reason: ev.error_message || "后台子代理暂不支持交互审批，本次调用已拒绝。",
        });
      } else if (ev.kind === "task.asset_created" && ev.assets) {
        // 2.2 资产产生事件：收集并支持即时渲染
        for (const item of ev.assets) {
          if (!eventAssets.some((a) => a.id === item.id)) {
            eventAssets.push(item);
          }
        }
      } else if (ev.kind === "task.waiting") {
        isWaiting = true;
        waitingStatus = ev.status || "等待子任务";
      } else if (ev.kind === "task.updated" || ev.kind === "task.completed") {
        isWaiting = false;
      }
    }

    // 2.3 终态结果兜底：若流式回答未到达或为空，使用 task.result 作为最终回答快照，绝不重复拼接
    const finalAnswer = answerText.trim() ? answerText : task?.result || "";
    const tools = toolOrder.map((id) => toolsMap.get(id)!).filter(Boolean);

    return {
      reasoningText: reasoningText.trim() || task?.reasoning || "",
      answerText: finalAnswer,
      tools,
      eventAssets,
      permissionDenials,
      isWaiting,
      waitingStatus,
    };
  }, [filteredEvents, task]);

  // 合并全部已发现资产（事件中的即时资产 + 落库资源）
  const allAssets = useMemo(() => {
    const map = new Map<string, AssetSummary>();
    for (const a of assets) {
      map.set(a.id, a);
    }
    for (const a of processViewModel.eventAssets) {
      map.set(a.id, a);
    }
    return Array.from(map.values());
  }, [assets, processViewModel.eventAssets]);

  if (!task) return null;

  const isRunning = task.status === "running";
  const statusLabel = formatStatusLabel(task.status);
  const statusBadgeStyle = getStatusBadgeStyle(task.status);

  return (
    <Modal
      animationType="slide"
      onRequestClose={onClose}
      transparent
      visible={visible}
    >
      <View style={styles.backdrop}>
        <View style={styles.container}>
          {/* 3.0 顶部导航与操作栏 */}
          <View style={styles.header}>
            <View style={styles.headerLeft}>
              <View style={styles.avatarIcon}>
                <Text style={styles.avatarEmoji}>🤖</Text>
              </View>
              <View style={styles.headerTitleWrap}>
                <View style={styles.headerTitleRow}>
                  <Text numberOfLines={1} style={styles.headerTitle}>
                    {task.agent_nickname || task.title || "子代理"}
                  </Text>
                  <View style={[styles.statusBadge, statusBadgeStyle]}>
                    <Text style={styles.statusBadgeText}>{statusLabel}</Text>
                  </View>
                </View>
                <View style={styles.headerMetaRow}>
                  <Text style={styles.readOnlyBadge}>🔒 只读详情 (主会话不变)</Text>
                  {task.definition_id ? (
                    <Text numberOfLines={1} style={styles.headerSub}>
                      · {task.definition_id}
                    </Text>
                  ) : null}
                </View>
              </View>
            </View>

            <View style={styles.headerActions}>
              <Pressable
                onPress={onRefresh}
                style={({ pressed }) => buttonFeedback(styles.iconButton, pressed)}
              >
                {loadingDetail || loadingEvents ? (
                  <ActivityIndicator color="#12100e" size="small" />
                ) : (
                  <Text style={styles.iconButtonText}>↻</Text>
                )}
              </Pressable>
              <Pressable
                onPress={onClose}
                style={({ pressed }) => buttonFeedback(styles.closeButton, pressed)}
              >
                <Text style={styles.closeButtonText}>✕</Text>
              </Pressable>
            </View>
          </View>

          {/* 4.0 任务元信息与排队状态提示 */}
          <View style={styles.metaCard}>
            {task.instruction ? (
              <View style={styles.metaRow}>
                <Text style={styles.metaLabel}>指令:</Text>
                <Text numberOfLines={2} style={styles.metaValue}>
                  {task.instruction}
                </Text>
              </View>
            ) : null}
            <View style={styles.idRow}>
              <Text style={styles.idText}>
                Task: <Text style={styles.idMono}>{task.id}</Text>
              </Text>
              {task.child_session_id ? (
                <Text style={styles.idText}>
                  Child: <Text style={styles.idMono}>{task.child_session_id}</Text>
                </Text>
              ) : null}
            </View>

            {/* 4.1 排队或启动失败提醒（session_ready=false 时仍可查看） */}
            {!sessionReady ? (
              <View style={styles.warningBanner}>
                <Text style={styles.warningBannerText}>
                  ⚠️ 子会话尚未创建或处于排队中，当前展示任务流转与运行事件。
                </Text>
              </View>
            ) : null}
          </View>

          {/* 5.0 Follow-up 多 Run 切换器 */}
          {runs.length > 1 ? (
            <View style={styles.runBar}>
              <Text style={styles.runBarLabel}>运行轮次:</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                {runs.map((r, i) => {
                  const isSelected = (selectedRunID || task.current_run_id) === r.id;
                  return (
                    <Pressable
                      key={r.id}
                      onPress={() => onSelectRun(r.id)}
                      style={({ pressed }) =>
                        buttonFeedback(
                          [styles.runPill, isSelected && styles.runPillSelected],
                          pressed,
                        )
                      }
                    >
                      <Text
                        style={[
                          styles.runPillText,
                          isSelected && styles.runPillTextSelected,
                        ]}
                      >
                        Run #{i + 1} ({formatStatusLabel(r.status)})
                      </Text>
                    </Pressable>
                  );
                })}
              </ScrollView>
            </View>
          ) : null}

          {/* 6.0 视图选项卡 (实时过程 ↔ 历史消息 ↔ 产物) */}
          <View style={styles.tabBar}>
            <Pressable
              onPress={() => setActiveTab("process")}
              style={({ pressed }) =>
                buttonFeedback(
                  [styles.tabButton, activeTab === "process" && styles.tabButtonActive],
                  pressed,
                )
              }
            >
              <Text
                style={[
                  styles.tabButtonText,
                  activeTab === "process" && styles.tabButtonTextActive,
                ]}
              >
                ⚡ 实时过程
              </Text>
            </Pressable>

            <Pressable
              onPress={() => setActiveTab("messages")}
              style={({ pressed }) =>
                buttonFeedback(
                  [styles.tabButton, activeTab === "messages" && styles.tabButtonActive],
                  pressed,
                )
              }
            >
              <Text
                style={[
                  styles.tabButtonText,
                  activeTab === "messages" && styles.tabButtonTextActive,
                ]}
              >
                💬 会话历史 ({messages.length})
              </Text>
            </Pressable>

            <Pressable
              onPress={() => setActiveTab("assets")}
              style={({ pressed }) =>
                buttonFeedback(
                  [styles.tabButton, activeTab === "assets" && styles.tabButtonActive],
                  pressed,
                )
              }
            >
              <Text
                style={[
                  styles.tabButtonText,
                  activeTab === "assets" && styles.tabButtonTextActive,
                ]}
              >
                📦 产物与文件 ({allAssets.length})
              </Text>
            </Pressable>
          </View>

          {/* 7.0 选项卡内容区 */}
          <ScrollView
            contentContainerStyle={styles.contentBody}
            nestedScrollEnabled
            showsVerticalScrollIndicator
          >
            {activeTab === "process" && (
              <View style={styles.processWrap}>
                {/* 7.1 等待指示器 */}
                {processViewModel.isWaiting ? (
                  <View style={styles.waitingNotice}>
                    <ActivityIndicator color="#0f766e" size="small" />
                    <Text style={styles.waitingNoticeText}>
                      正在{processViewModel.waitingStatus}...
                    </Text>
                  </View>
                ) : null}

                {/* 7.2 权限拒绝说明（后台审批不可用时明确显示） */}
                {processViewModel.permissionDenials.map((denial, idx) => (
                  <View key={`denial-${idx}`} style={styles.permissionAlert}>
                    <Text style={styles.permissionAlertTitle}>
                      🛡️ 工具权限拒绝 · {denial.toolName}
                    </Text>
                    <Text style={styles.permissionAlertText}>{denial.reason}</Text>
                  </View>
                ))}

                {/* 7.3 推理过程思考块 */}
                {processViewModel.reasoningText ? (
                  <View style={styles.thoughtCard}>
                    <Pressable
                      onPress={() => setThoughtExpanded((p) => !p)}
                      style={styles.thoughtHeader}
                    >
                      <Text style={styles.thoughtHeaderTitle}>💭 推理思考</Text>
                      <Text style={styles.thoughtChevron}>
                        {thoughtExpanded ? "⌄" : "›"}
                      </Text>
                    </Pressable>
                    {thoughtExpanded ? (
                      <View style={styles.thoughtBody}>
                        <MarkdownText text={processViewModel.reasoningText} />
                      </View>
                    ) : null}
                  </View>
                ) : null}

                {/* 7.4 工具调用过程 */}
                {processViewModel.tools.length > 0 ? (
                  <View style={styles.toolsSection}>
                    <Text style={styles.sectionHeaderTitle}>🛠️ 工具调用序列</Text>
                    {processViewModel.tools.map((tool) => (
                      <View key={tool.toolCallID} style={styles.toolCard}>
                        <View style={styles.toolHeader}>
                          <View style={styles.toolHeaderLeft}>
                            <View
                              style={[
                                styles.toolStatusTag,
                                tool.status === "success" && styles.toolStatusSuccess,
                                tool.status === "failed" && styles.toolStatusFailed,
                                tool.status === "running" && styles.toolStatusRunning,
                              ]}
                            >
                              <Text style={styles.toolStatusTagText}>
                                {tool.status === "success"
                                  ? "成功"
                                  : tool.status === "failed"
                                    ? "失败"
                                    : "执行中"}
                              </Text>
                            </View>
                            <Text numberOfLines={1} style={styles.toolName}>
                              {tool.name}
                            </Text>
                          </View>
                          <Text style={styles.toolCallIDMono}>
                            {tool.toolCallID.slice(0, 8)}
                          </Text>
                        </View>

                        {tool.arguments ? (
                          <View style={styles.toolArgBox}>
                            <Text style={styles.toolArgLabel}>入参:</Text>
                            <Text selectable style={styles.toolMonoText}>
                              {tool.arguments}
                            </Text>
                          </View>
                        ) : null}

                        {tool.result || tool.errorMessage ? (
                          <View style={styles.toolResultBox}>
                            <Text style={styles.toolArgLabel}>
                              {tool.errorMessage ? "错误:" : "输出:"}
                            </Text>
                            <Text
                              selectable
                              style={[
                                styles.toolMonoText,
                                tool.errorMessage && styles.toolErrorText,
                              ]}
                            >
                              {tool.errorMessage || tool.result}
                            </Text>
                          </View>
                        ) : null}
                      </View>
                    ))}
                  </View>
                ) : null}

                {/* 7.5 产生的资产预览 (复用 SharedAssetCard) */}
                {processViewModel.eventAssets.length > 0 ? (
                  <View style={styles.assetsSection}>
                    <Text style={styles.sectionHeaderTitle}>🖼️ 产物与资源</Text>
                    {processViewModel.eventAssets.map((asset) => (
                      <SharedAssetCard
                        asset={assetSummaryToSharedAsset(asset)}
                        buttonFeedback={buttonFeedback}
                        key={asset.id}
                      />
                    ))}
                  </View>
                ) : null}

                {/* 7.6 最终回答正文 */}
                {processViewModel.answerText ? (
                  <View style={styles.answerCard}>
                    <Text style={styles.answerHeaderTitle}>💡 子代理回答</Text>
                    <MarkdownText text={processViewModel.answerText} />
                  </View>
                ) : isRunning ? (
                  <View style={styles.emptyNotice}>
                    <ActivityIndicator color="#0284c7" size="small" />
                    <Text style={styles.emptyNoticeText}>子代理正在执行中，等待输出...</Text>
                  </View>
                ) : (
                  <View style={styles.emptyNotice}>
                    <Text style={styles.emptyNoticeText}>暂无详细输出</Text>
                  </View>
                )}
              </View>
            )}

            {/* 8.0 会话历史消息 Tab */}
            {activeTab === "messages" && (
              <View style={styles.messagesWrap}>
                {hasMoreMessages ? (
                  <Pressable
                    disabled={loadingMoreMessages}
                    onPress={onLoadMoreMessages}
                    style={({ pressed }) =>
                      buttonFeedback(styles.loadMoreButton, pressed)
                    }
                  >
                    {loadingMoreMessages ? (
                      <ActivityIndicator color="#12100e" size="small" />
                    ) : (
                      <Text style={styles.loadMoreButtonText}>加载更早的历史记录 ❯</Text>
                    )}
                  </Pressable>
                ) : null}

                {messages.length === 0 ? (
                  <View style={styles.emptyNotice}>
                    <Text style={styles.emptyNoticeText}>
                      {!sessionReady
                        ? "子会话排队或未落库，消息为空"
                        : "暂无子会话持久化消息"}
                    </Text>
                  </View>
                ) : (
                  messages.map((m) => (
                    <View
                      key={m.id}
                      style={[
                        styles.messageCard,
                        m.role === "user" && styles.messageCardUser,
                        m.role === "assistant" && styles.messageCardAssistant,
                        m.role === "tool" && styles.messageCardTool,
                      ]}
                    >
                      <View style={styles.messageHeaderRow}>
                        <Text style={styles.messageRoleText}>
                          {m.role === "user"
                            ? "用户指令"
                            : m.role === "assistant"
                              ? "智能体"
                              : `工具: ${m.tool_name || "tool"}`}
                        </Text>
                        {m.created_at ? (
                          <Text style={styles.messageTimeText}>
                            {formatDateTime(m.created_at)}
                          </Text>
                        ) : null}
                      </View>
                      {m.content ? <MarkdownText text={m.content} /> : null}
                      {m.tool_error ? (
                        <Text style={styles.toolErrorText}>{m.tool_error}</Text>
                      ) : null}
                    </View>
                  ))
                )}
              </View>
            )}

            {/* 9.0 产物与文件 Tab */}
            {activeTab === "assets" && (
              <View style={styles.assetsWrap}>
                {allAssets.length === 0 ? (
                  <View style={styles.emptyNotice}>
                    <Text style={styles.emptyNoticeText}>暂未产生文件或图片产物</Text>
                  </View>
                ) : (
                  allAssets.map((asset) => (
                    <SharedAssetCard
                      asset={assetSummaryToSharedAsset(asset)}
                      buttonFeedback={buttonFeedback}
                      key={asset.id}
                    />
                  ))
                )}
              </View>
            )}
          </ScrollView>
        </View>
      </View>
    </Modal>
  );
}

function formatStatusLabel(status?: string): string {
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
      return status || "未知";
  }
}

function getStatusBadgeStyle(status?: string) {
  switch (status) {
    case "running":
      return styles.statusRunning;
    case "succeeded":
      return styles.statusSucceeded;
    case "failed":
      return styles.statusFailed;
    case "waiting_subagents":
    case "waiting_permission":
      return styles.statusWaiting;
    default:
      return styles.statusDefault;
  }
}

const monoFont = Platform.OS === "ios" ? "Menlo" : "monospace";

const styles = StyleSheet.create({
  backdrop: {
    backgroundColor: "rgba(18, 16, 14, 0.65)",
    flex: 1,
    justifyContent: "flex-end",
  },
  container: {
    backgroundColor: "#fffdf9",
    borderTopColor: "#12100e",
    borderTopLeftRadius: 18,
    borderTopRightRadius: 18,
    borderTopWidth: 3,
    display: "flex",
    height: "88%",
    overflow: "hidden",
    width: "100%",
  },
  header: {
    alignItems: "center",
    backgroundColor: "#f4eee2",
    borderBottomColor: "#12100e",
    borderBottomWidth: 2,
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 14,
    paddingVertical: 12,
  },
  headerLeft: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 10,
  },
  avatarIcon: {
    alignItems: "center",
    backgroundColor: "#e8ded1",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 1.5,
    height: 38,
    justifyContent: "center",
    width: 38,
  },
  avatarEmoji: {
    fontSize: 20,
  },
  headerTitleWrap: {
    flex: 1,
    gap: 2,
  },
  headerTitleRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  headerTitle: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  headerMetaRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 4,
  },
  readOnlyBadge: {
    color: "#78716c",
    fontSize: 11,
    fontWeight: "700",
  },
  headerSub: {
    color: "#a8a29e",
    fontSize: 11,
  },
  headerActions: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  iconButton: {
    alignItems: "center",
    backgroundColor: "#fbf8f2",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 1.5,
    height: 32,
    justifyContent: "center",
    width: 32,
  },
  iconButtonText: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  closeButton: {
    alignItems: "center",
    backgroundColor: "#fee2e2",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 1.5,
    height: 32,
    justifyContent: "center",
    width: 32,
  },
  closeButtonText: {
    color: "#991b1b",
    fontSize: 14,
    fontWeight: "900",
  },
  statusBadge: {
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 1,
    paddingHorizontal: 7,
    paddingVertical: 1,
  },
  statusBadgeText: {
    fontSize: 11,
    fontWeight: "800",
  },
  statusRunning: {
    backgroundColor: "#cffafe",
  },
  statusSucceeded: {
    backgroundColor: "#dcfce7",
  },
  statusFailed: {
    backgroundColor: "#fee2e2",
  },
  statusWaiting: {
    backgroundColor: "#fef3c7",
  },
  statusDefault: {
    backgroundColor: "#e7e5e4",
  },
  metaCard: {
    backgroundColor: "#fcfbf7",
    borderBottomColor: "#e7dfd5",
    borderBottomWidth: 1,
    gap: 4,
    paddingHorizontal: 14,
    paddingVertical: 8,
  },
  metaRow: {
    flexDirection: "row",
    gap: 6,
  },
  metaLabel: {
    color: "#78716c",
    fontSize: 12,
    fontWeight: "700",
  },
  metaValue: {
    color: "#1c1917",
    flex: 1,
    fontSize: 12,
  },
  idRow: {
    flexDirection: "row",
    gap: 12,
  },
  idText: {
    color: "#a8a29e",
    fontSize: 11,
  },
  idMono: {
    color: "#44403c",
    fontFamily: monoFont,
    fontSize: 11,
  },
  warningBanner: {
    backgroundColor: "#fffbeb",
    borderColor: "#fde68a",
    borderRadius: 6,
    borderWidth: 1,
    marginTop: 4,
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
  warningBannerText: {
    color: "#b45309",
    fontSize: 11,
    fontWeight: "600",
  },
  runBar: {
    alignItems: "center",
    backgroundColor: "#f5eee4",
    borderBottomColor: "#12100e",
    borderBottomWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 14,
    paddingVertical: 6,
  },
  runBarLabel: {
    color: "#57534e",
    fontSize: 12,
    fontWeight: "700",
  },
  runPill: {
    backgroundColor: "#fff",
    borderColor: "#78716c",
    borderRadius: 4,
    borderWidth: 1,
    marginRight: 6,
    paddingHorizontal: 8,
    paddingVertical: 2,
  },
  runPillSelected: {
    backgroundColor: "#12100e",
    borderColor: "#12100e",
  },
  runPillText: {
    color: "#44403c",
    fontSize: 11,
    fontWeight: "700",
  },
  runPillTextSelected: {
    color: "#fff",
  },
  tabBar: {
    backgroundColor: "#ede4d4",
    borderBottomColor: "#12100e",
    borderBottomWidth: 2,
    flexDirection: "row",
  },
  tabButton: {
    alignItems: "center",
    borderRightColor: "#12100e",
    borderRightWidth: 1,
    flex: 1,
    paddingVertical: 9,
  },
  tabButtonActive: {
    backgroundColor: "#fffdf9",
    borderBottomColor: "#fffdf9",
    borderBottomWidth: 2,
    marginBottom: -2,
  },
  tabButtonText: {
    color: "#78716c",
    fontSize: 13,
    fontWeight: "800",
  },
  tabButtonTextActive: {
    color: "#12100e",
  },
  contentBody: {
    gap: 12,
    padding: 14,
    paddingBottom: 40,
  },
  processWrap: {
    gap: 12,
  },
  waitingNotice: {
    alignItems: "center",
    backgroundColor: "#f0fdfa",
    borderColor: "#0f766e",
    borderRadius: 8,
    borderWidth: 1.5,
    flexDirection: "row",
    gap: 8,
    padding: 10,
  },
  waitingNoticeText: {
    color: "#0f766e",
    fontSize: 13,
    fontWeight: "700",
  },
  permissionAlert: {
    backgroundColor: "#fef2f2",
    borderColor: "#ef4444",
    borderRadius: 8,
    borderWidth: 1.5,
    gap: 4,
    padding: 10,
  },
  permissionAlertTitle: {
    color: "#991b1b",
    fontSize: 13,
    fontWeight: "800",
  },
  permissionAlertText: {
    color: "#b91c1c",
    fontSize: 12,
  },
  thoughtCard: {
    backgroundColor: "#f8f6f0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 1.5,
    overflow: "hidden",
  },
  thoughtHeader: {
    alignItems: "center",
    backgroundColor: "#ede7da",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  thoughtHeaderTitle: {
    color: "#44403c",
    fontSize: 12,
    fontWeight: "800",
  },
  thoughtChevron: {
    color: "#78716c",
    fontSize: 14,
    fontWeight: "900",
  },
  thoughtBody: {
    padding: 10,
  },
  toolsSection: {
    gap: 8,
  },
  sectionHeaderTitle: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  toolCard: {
    backgroundColor: "#fff",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 1.5,
    gap: 6,
    padding: 10,
  },
  toolHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  toolHeaderLeft: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  toolStatusTag: {
    borderRadius: 4,
    paddingHorizontal: 6,
    paddingVertical: 1,
  },
  toolStatusTagText: {
    fontSize: 10,
    fontWeight: "800",
  },
  toolStatusRunning: {
    backgroundColor: "#cffafe",
  },
  toolStatusSuccess: {
    backgroundColor: "#dcfce7",
  },
  toolStatusFailed: {
    backgroundColor: "#fee2e2",
  },
  toolName: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "800",
  },
  toolCallIDMono: {
    color: "#a8a29e",
    fontFamily: monoFont,
    fontSize: 10,
  },
  toolArgBox: {
    backgroundColor: "#f5f5f4",
    borderRadius: 4,
    padding: 6,
  },
  toolResultBox: {
    backgroundColor: "#f5f5f4",
    borderRadius: 4,
    padding: 6,
  },
  toolArgLabel: {
    color: "#78716c",
    fontSize: 10,
    fontWeight: "700",
    marginBottom: 2,
  },
  toolMonoText: {
    color: "#1c1917",
    fontFamily: monoFont,
    fontSize: 11,
  },
  toolErrorText: {
    color: "#dc2626",
  },
  assetsSection: {
    gap: 8,
  },
  answerCard: {
    backgroundColor: "#fff",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    gap: 8,
    padding: 12,
  },
  answerHeaderTitle: {
    color: "#12100e",
    fontSize: 14,
    fontWeight: "900",
  },
  emptyNotice: {
    alignItems: "center",
    borderColor: "#e7dfd5",
    borderRadius: 8,
    borderStyle: "dashed",
    borderWidth: 1.5,
    flexDirection: "row",
    gap: 8,
    justifyContent: "center",
    paddingVertical: 24,
  },
  emptyNoticeText: {
    color: "#a8a29e",
    fontSize: 13,
    fontWeight: "600",
  },
  messagesWrap: {
    gap: 8,
  },
  loadMoreButton: {
    alignItems: "center",
    backgroundColor: "#ede7da",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 1,
    paddingVertical: 8,
  },
  loadMoreButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "800",
  },
  messageCard: {
    backgroundColor: "#fff",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 1.5,
    gap: 4,
    padding: 10,
  },
  messageCardUser: {
    backgroundColor: "#eff6ff",
    borderColor: "#3b82f6",
  },
  messageCardAssistant: {
    backgroundColor: "#fdf8ee",
  },
  messageCardTool: {
    backgroundColor: "#fbfbfa",
  },
  messageHeaderRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  messageRoleText: {
    color: "#78716c",
    fontSize: 11,
    fontWeight: "800",
  },
  messageTimeText: {
    color: "#a8a29e",
    fontSize: 10,
  },
  assetsWrap: {
    gap: 10,
  },
});
