import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import {
  ActivityIndicator,
  Keyboard,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { Feather } from "@expo/vector-icons";

import type { AgentRunSnapshot, AssetSummary, ChangeEntry, Plan, SubagentTask } from "../../protocol";
import type { UserQuestionState } from "../../types/app";
import type { ChatItem } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { latestRegeneratableTurnID } from "../../hooks/useChatActions";
import { AssetPreviewVisibleContext } from "../../hooks/useAssetImagePreview";
import { buildChatTurns, type ChatRenderItem } from "../../utils/chatRenderItems";
import type { MotionSlot } from "../../utils/motionRegistry";
import { radius, space, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { UserQuestionPrompt } from "../permissions/UserQuestionPrompt";
import { AgentRunTimeline } from "./AgentRunTimeline";
import { AgentTurnCard } from "./AgentTurnCard";
import { AssistantLoadingBubble } from "./AssistantLoadingBubble";
import { ChatEmptyState } from "./ChatEmptyState";
import { InlinePlanCard } from "./InlinePlanCard";
import { MessageBubble } from "./MessageBubble";
import { ToolActivityGroup } from "./ToolActivityGroup";

const ignoreRegenerate = () => undefined;

type Props = {
  activeAssistantID: string;
  activePlan?: Plan;
  activitySlot?: MotionSlot;
  assets: AssetSummary[];
  buttonFeedback: ButtonFeedback;
  changesCount?: number;
  chatScrollRef: RefObject<ScrollView | null>;
  connected: boolean;
  deviceID: string;
  greetingName: string;
  loadingHistory: boolean;
  messages: ChatItem[];
  onAnswerUserQuestion?: (question: UserQuestionState, answers: Record<string, string>, cancelled?: boolean) => boolean;
  onChangeUserQuestionDraft?: (question: UserQuestionState, answers: Record<string, string>) => void;
  onExecutePlan?: () => void;
  onInspectSubagent?: (taskID: string) => void;
  onNearHeaderChange?: (near: boolean) => void;
  onOpenChangePath?: (path: string) => void;
  onOpenChanges?: () => void;
  onOpenPlan?: () => void;
  onPickSuggestion: (text: string) => void;
  onRegenerate: (turnID: string) => void;
  onScrolledChange?: (scrolled: boolean) => void;
  pendingPlan?: boolean;
  pendingQuestions?: UserQuestionState[];
  pendingRequestID: string;
  runs: AgentRunSnapshot[];
  sessionID: string;
  subagentTasks?: SubagentTask[];
  topClearance?: number;
  workspaceChanges?: ChangeEntry[];
};

export function ChatPanel({
  activeAssistantID,
  activePlan,
  activitySlot = "thinking",
  assets,
  buttonFeedback,
  changesCount = 0,
  chatScrollRef,
  connected,
  deviceID,
  greetingName,
  loadingHistory,
  messages,
  onAnswerUserQuestion,
  onChangeUserQuestionDraft,
  onExecutePlan,
  onInspectSubagent,
  onNearHeaderChange,
  onOpenChangePath,
  onOpenChanges,
  onOpenPlan,
  onPickSuggestion,
  onRegenerate,
  onScrolledChange,
  pendingPlan = false,
  pendingQuestions = [],
  pendingRequestID,
  runs,
  sessionID,
  subagentTasks,
  topClearance = 82,
  workspaceChanges,
}: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);

  const renderItems = useMemo(() => buildChatTurns(messages, runs, assets), [assets, messages, runs]);
  const [viewport, setViewport] = useState({ offset: 0, height: 0 });
  const followTailRef = useRef(true);
  const lastViewportHeightRef = useRef<number>(0);
  const scrolledRef = useRef(false);
  const nearHeaderRef = useRef(false);
  const [showBackToBottom, setShowBackToBottom] = useState(false);

  const isEmpty = messages.length === 0 && runs.length === 0 && !activePlan && pendingQuestions.length === 0;
  const assistantLoadingLabel = useMemo(
    () => loadingLabel(pendingRequestID, activeAssistantID, messages, runs),
    [activeAssistantID, messages, pendingRequestID, runs],
  );
  const hasRunningStream =
    Boolean(pendingRequestID) ||
    runs.some(({ run }) => run.status === "running");
  const regeneratableTurnID = connected && sessionID && !loadingHistory && !pendingPlan &&
    !hasRunningStream && pendingQuestions.length === 0 &&
    !messages.some((message) => message.role === "user" && message.status === "queued")
    ? latestRegeneratableTurnID(renderItems)
    : undefined;

  const scrollToTail = useCallback(
    (animated = true, force = false) => {
      requestAnimationFrame(() => {
        if (force || followTailRef.current) {
          chatScrollRef.current?.scrollToEnd({ animated });
        }
      });
    },
    [chatScrollRef],
  );

  useEffect(() => {
    followTailRef.current = true;
    setShowBackToBottom(false);
    nearHeaderRef.current = false;
    onNearHeaderChange?.(false);
  }, [onNearHeaderChange, sessionID]);

  useEffect(() => {
    if (isEmpty && nearHeaderRef.current) {
      nearHeaderRef.current = false;
      onNearHeaderChange?.(false);
    }
  }, [isEmpty, onNearHeaderChange]);

  // 只在原本跟随最新消息时随键盘调整，回看历史时保留阅读位置。
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const showEvent = Platform.OS === "ios" ? "keyboardWillShow" : "keyboardDidShow";
    const sub = Keyboard.addListener(showEvent, () => {
      clearTimeout(timer);
      if (!followTailRef.current) return;
      scrollToTail(true);
      timer = setTimeout(() => scrollToTail(false), 90);
    });
    return () => {
      sub.remove();
      clearTimeout(timer);
    };
  }, [scrollToTail]);

  return (
    <View style={styles.chatContainer}>
      <ScrollView
        contentContainerStyle={[
          styles.messages,
          { paddingTop: isEmpty ? Math.max(40, Math.round(topClearance * 0.72)) : topClearance },
          isEmpty && styles.messagesEmpty,
        ]}
        keyboardShouldPersistTaps="handled"
        onContentSizeChange={() => {
          if (followTailRef.current) {
            scrollToTail(true);
          }
        }}
        onLayout={(event) => {
          const currentHeight = event.nativeEvent.layout.height;
          setViewport((current) => current.height === currentHeight ? current : { ...current, height: currentHeight });
          if (lastViewportHeightRef.current > 0 && currentHeight < lastViewportHeightRef.current) {
            if (followTailRef.current) {
              scrollToTail(true);
            }
          }
          lastViewportHeightRef.current = currentHeight;
        }}
        onScroll={(event) => {
          const { contentOffset, contentSize, layoutMeasurement } = event.nativeEvent;
          const offset = contentOffset.y;
          const height = layoutMeasurement.height;
          setViewport((current) => current.offset === offset && current.height === height ? current : { offset, height });
          const distanceFromBottom = contentSize.height - (contentOffset.y + layoutMeasurement.height);
          followTailRef.current = distanceFromBottom <= 44;

          const awayFromBottom = !isEmpty && distanceFromBottom > 120;
          setShowBackToBottom(awayFromBottom);

          const scrolled = contentOffset.y > 4;
          if (scrolled !== scrolledRef.current) {
            scrolledRef.current = scrolled;
            onScrolledChange?.(scrolled);
          }

          // 内容进入顶栏角色岛下方邻近带时平滑半透明化，避免遮挡对话内容
          const near = !isEmpty && contentOffset.y > 16;
          if (near !== nearHeaderRef.current) {
            nearHeaderRef.current = near;
            onNearHeaderChange?.(near);
          }
        }}
        ref={chatScrollRef}
        scrollEventThrottle={48}
        showsVerticalScrollIndicator={false}
        style={styles.messagesScroll}
      >
        {isEmpty ? (
          loadingHistory ? (
            <View style={styles.inlineLoading}>
              <ActivityIndicator color={palette.ink2} size="small" />
              <Text style={styles.inlineLoadingText}>正在加载会话历史...</Text>
            </View>
          ) : (
            <ChatEmptyState
              changesCount={changesCount}
              connected={connected}
              deviceID={deviceID}
              name={greetingName}
              onOpenChanges={onOpenChanges}
              onPickSuggestion={onPickSuggestion}
            />
          )
        ) : (
          <>
            {renderItems.map((item) => {
              if (item.type === "agent_turn") {
                return (
                  <PreviewRow key={JSON.stringify([sessionID, item.id])} viewport={viewport}>
                    <AgentTurnCard
                      buttonFeedback={buttonFeedback}
                      catalog={assets}
                      onInspectSubagent={onInspectSubagent}
                      onOpenChangePath={onOpenChangePath}
                      onOpenChanges={onOpenChanges}
                      onRegenerate={item.id === regeneratableTurnID ? () => onRegenerate(item.id) : undefined}
                      sessionID={sessionID}
                      subagentTasks={subagentTasks}
                      turn={item}
                      workspaceChanges={workspaceChanges}
                    />
                  </PreviewRow>
                );
              }
              if (item.type === "tool_group") {
                return (
                  <View key={item.id}>
                    <ToolActivityGroup buttonFeedback={buttonFeedback} group={item.group} />
                  </View>
                );
              }
              return (
                <PreviewRow key={JSON.stringify([sessionID, item.id])} viewport={viewport}>
                  <MessageBubble
                    buttonFeedback={buttonFeedback}
                    catalog={assets}
                    hideReasoning={item.message.role === "assistant" && isCoveredByRun(item.message, runs)}
                    message={item.message}
                    onRegenerate={ignoreRegenerate}
                    sessionID={sessionID}
                  />
                </PreviewRow>
              );
            })}

            {activePlan && onOpenPlan && onExecutePlan ? (
              <InlinePlanCard
                onExecutePlan={onExecutePlan}
                onOpenDetail={onOpenPlan}
                pendingExecute={pendingPlan}
                plan={activePlan}
              />
            ) : null}

            {pendingQuestions.map((question) => (
              <UserQuestionPrompt
                buttonFeedback={buttonFeedback}
                key={JSON.stringify([question.sessionID, question.requestID, question.questionID])}
                onAnswer={(answers, cancelled) => onAnswerUserQuestion?.(question, answers, cancelled) ?? false}
                onChangeDraft={onChangeUserQuestionDraft ? (answers) => onChangeUserQuestionDraft(question, answers) : undefined}
                question={question}
              />
            ))}

            {hasRunningStream && !renderItems.some((it) => it.type === "agent_turn" && it.status === "running") ? (
              <AssistantLoadingBubble label={assistantLoadingLabel} slot={activitySlot} />
            ) : null}
          </>
        )}
      </ScrollView>

      {showBackToBottom ? (
        <Pressable
          accessibilityLabel="回到底部"
          accessibilityRole="button"
          onPress={() => {
            followTailRef.current = true;
            setShowBackToBottom(false);
            scrollToTail(true, true);
          }}
          style={({ pressed }) => [styles.scrollBottomBtn, pressed && styles.scrollBottomPressed]}
        >
          {hasRunningStream ? <View style={styles.scrollBottomPulse} /> : null}
          <Feather color={palette.ink1} name="arrow-down" size={15} />
          <Text style={styles.scrollBottomText}>
            {pendingQuestions.length > 0 ? "查看待确认" : "回到底部"}
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
}

function PreviewRow({ children, viewport }: { children: ReactNode; viewport: { offset: number; height: number } }) {
  const [layout, setLayout] = useState<{ y: number; height: number } | null>(null);
  const overscan = 160;
  const visible = Boolean(layout && viewport.height > 0 &&
    layout.y + layout.height >= viewport.offset - overscan &&
    layout.y <= viewport.offset + viewport.height + overscan);
  return (
    <View onLayout={({ nativeEvent: { layout: next } }) => {
      setLayout((current) => current?.y === next.y && current.height === next.height ? current : next);
    }}>
      <AssetPreviewVisibleContext.Provider value={visible}>{children}</AssetPreviewVisibleContext.Provider>
    </View>
  );
}

type PanelRenderItem = ChatRenderItem | { type: "run"; id: string; snapshot: AgentRunSnapshot };

export function injectAgentRuns(items: ChatRenderItem[], messages: ChatItem[], runs: AgentRunSnapshot[]): PanelRenderItem[] {
  if (runs.length === 0) {
    return items;
  }
  const result: PanelRenderItem[] = [];
  const inserted = new Set<string>();
  const users = messages.filter((message) => message.role === "user");

  items.forEach((item) => {
    result.push(item);
    const anchor = item.type === "user_turn" ? item.message : item.type === "message" ? item.message : undefined;
    if (!anchor) return;
    const nextUser = anchor.role === "user" ? users[users.indexOf(anchor) + 1] : undefined;
    runs.forEach((snapshot) => {
      if (inserted.has(snapshot.run.id)) return;
      const requestMatch = Boolean(anchor.requestID && snapshot.run.request_id === anchor.requestID);
      const timeMatch = anchor.role === "user" && runFollowsUser(snapshot, anchor, nextUser);
      if (requestMatch || timeMatch) {
        result.push({ type: "run", id: `agent-run-${snapshot.run.id}`, snapshot });
        inserted.add(snapshot.run.id);
      }
    });
  });

  runs.forEach((snapshot) => {
    if (!inserted.has(snapshot.run.id)) {
      result.push({ type: "run", id: `agent-run-${snapshot.run.id}`, snapshot });
    }
  });
  return result;
}

function runFollowsUser(snapshot: AgentRunSnapshot, user: ChatItem, nextUser?: ChatItem) {
  const startedAt = Date.parse(snapshot.run.started_at);
  const userAt = user.createdAt ? Date.parse(user.createdAt) : Number.NaN;
  const nextUserAt = nextUser?.createdAt ? Date.parse(nextUser.createdAt) : Number.POSITIVE_INFINITY;
  return Number.isFinite(startedAt) && Number.isFinite(userAt) && startedAt >= userAt - 1000 && startedAt < nextUserAt;
}

function isCoveredByRun(message: ChatItem, runs: AgentRunSnapshot[]) {
  if (message.requestID && runs.some((snapshot) => snapshot.run.request_id === message.requestID)) {
    return true;
  }
  if (!message.createdAt) return false;
  const messageAt = Date.parse(message.createdAt);
  if (!Number.isFinite(messageAt)) return false;
  return runs.some(({ run }) => {
    const startedAt = Date.parse(run.started_at);
    const finishedAt = run.finished_at ? Date.parse(run.finished_at) : Date.now();
    return Number.isFinite(startedAt) && messageAt >= startedAt - 1000 && messageAt <= finishedAt + 30000;
  });
}

function isWaitingSubagent(pendingRequestID: string, messages: ChatItem[], runs: AgentRunSnapshot[]) {
  const activeRuns = runs.filter(({ run }) => run.status === "running");
  for (const { events } of activeRuns) {
    const waitEvent = [...events]
      .reverse()
      .find(
        (e) =>
          (e.tool_name === "wait_agent" || e.title === "wait_agent") &&
          (e.status === "waiting" || e.type === "tool_call"),
      );
    if (waitEvent) return true;
  }

  const waitMessage = [...messages].reverse().find((m) => m.toolName === "wait_agent" && m.status === "tool_running");
  if (waitMessage) return true;

  return false;
}

function loadingLabel(pendingRequestID: string, activeAssistantID: string, messages: ChatItem[], runs: AgentRunSnapshot[]) {
  if (isWaitingSubagent(pendingRequestID, messages, runs)) {
    return "等待子代理结果";
  }
  const hasRunningContinuation = runs.some(({ run }) => run.status === "running" && run.title?.includes("子代理"));
  if (hasRunningContinuation) {
    return "正在处理子代理结果";
  }
  if (hasRunningTool(pendingRequestID, activeAssistantID, messages, runs)) {
    return "正在运行工具";
  }
  return activeAssistantID ? "正在生成" : "正在思考";
}

function hasRunningTool(pendingRequestID: string, activeAssistantID: string, messages: ChatItem[], runs: AgentRunSnapshot[]) {
  const matchingRun = [...runs].reverse().find(({ run }) => run.request_id === pendingRequestID && run.status === "running");
  if (matchingRun) {
    const lastToolCall = lastIndex(matchingRun.events, (event) => event.type === "tool_call");
    const lastToolResult = lastIndex(matchingRun.events, (event) => event.type === "tool_result");
    if (lastToolCall > lastToolResult) return true;
  }

  const requestMessages = messages.filter((message) => message.requestID === pendingRequestID);
  const lastToolCall = lastIndex(requestMessages, (message) => message.role === "tool_call");
  const lastToolResult = lastIndex(requestMessages, (message) => message.role === "tool");
  if (lastToolCall > lastToolResult) return true;

  return messages.some((message) => message.id === activeAssistantID && message.status === "tool_running");
}

function lastIndex<T>(items: T[], matches: (item: T) => boolean) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (matches(items[index])) return index;
  }
  return -1;
}

const makeStyles = ({ font, palette, shadow }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    chatContainer: {
      flex: 1,
      minHeight: 0,
      position: "relative",
    },
    messagesScroll: {
      flex: 1,
    },
    messages: {
      gap: space.xxl,
      position: "relative",
      paddingBottom: 22,
      paddingHorizontal: space.xl,
    },
    messagesEmpty: {
      flexGrow: 1,
    },
    inlineLoading: {
      alignItems: "center",
      alignSelf: "center",
      flexDirection: "row",
      gap: 8,
      marginTop: 32,
    },
    inlineLoadingText: {
      color: palette.ink2,
      fontSize: font.label,
    },
    scrollBottomBtn: {
      alignItems: "center",
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderRadius: radius.pill,
      borderWidth: 1,
      bottom: 14,
      flexDirection: "row",
      gap: 6,
      minHeight: 34,
      paddingHorizontal: 13,
      position: "absolute",
      right: 16,
      ...shadow.card,
    },
    scrollBottomPressed: {
      opacity: 0.78,
    },
    scrollBottomPulse: {
      backgroundColor: palette.brand,
      borderRadius: radius.pill,
      height: 7,
      width: 7,
    },
    scrollBottomText: {
      color: palette.ink1,
      fontSize: 12,
      fontWeight: weight.semibold,
    },
  });
