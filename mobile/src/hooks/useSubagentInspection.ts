import { useCallback, useRef, useState } from "react";

import type {
  AssetSummary,
  AgentRunSnapshot,
  SessionHistoryMessage,
  SubagentRunSummary,
  SubagentTask,
  SubagentTaskDetailResultPayload,
  SubagentTaskEventsResultPayload,
  SubagentTaskResultPayload,
} from "../protocol";

type RequestTaskDetailFn = (
  taskID: string,
  afterMessageID?: string,
  messageLimit?: number,
  targetSessionID?: string,
) => string;

type RequestTaskEventsFn = (
  options: {
    taskID?: string;
    runID?: string;
    afterSequence?: number;
    throughSequence?: number;
    limit?: number;
  },
  targetSessionID?: string,
) => string;

type Args = {
  currentSessionID: string;
  requestTaskDetail: RequestTaskDetailFn;
  requestTaskEvents: RequestTaskEventsFn;
  tasks: SubagentTask[];
};

export function useSubagentInspection({
  currentSessionID,
  requestTaskDetail,
  requestTaskEvents,
  tasks,
}: Args) {
  // 1.0 状态定义：存储当前打开的子代理任务、Run、消息与事件
  const [inspectedTaskID, setInspectedTaskID] = useState<string | null>(null);
  const [inspectedTask, setInspectedTask] = useState<SubagentTask | null>(null);
  const [selectedRunID, setSelectedRunID] = useState<string | null>(null);
  const [runs, setRuns] = useState<SubagentRunSummary[]>([]);
  const [agentRuns, setAgentRuns] = useState<AgentRunSnapshot[]>([]);
  const [messages, setMessages] = useState<SessionHistoryMessage[]>([]);
  const [nextMessageID, setNextMessageID] = useState<string>("");
  const [hasMoreMessages, setHasMoreMessages] = useState(false);
  const [assets, setAssets] = useState<AssetSummary[]>([]);
  const [sessionReady, setSessionReady] = useState(true);
  const [readOnly, setReadOnly] = useState(true);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [loadingEvents, setLoadingEvents] = useState(false);
  const [loadingMoreMessages, setLoadingMoreMessages] = useState(false);
  const [taskEvents, setTaskEvents] = useState<SubagentTaskResultPayload[]>([]);
  const [error, setError] = useState<string | null>(null);

  // 2.0 引用引用：用于断线补取、实时事件缓冲与迟到响应隔离
  const inspectedTaskIDRef = useRef<string | null>(null);
  const inspectedTaskRef = useRef<SubagentTask | null>(null);
  const pendingDetailReqIDRef = useRef<string>("");
  const pendingEventsReqIDRef = useRef<string>("");
  const realtimeBufferRef = useRef<SubagentTaskResultPayload[]>([]);
  const eventsMapRef = useRef<Map<number, SubagentTaskResultPayload>>(new Map());
  const isReplayingRef = useRef<boolean>(false);
  const lastReplaySequenceRef = useRef<number>(0);
  const throughSequenceRef = useRef<number>(0);

  // 3.0 打开子任务详情：清空前序状态、缓冲实时事件、发起详情与事件回放
  const openInspection = useCallback(
    (taskOrID: SubagentTask | string) => {
      const taskID = typeof taskOrID === "string" ? taskOrID : taskOrID.id;
      if (!taskID) return;

      // 3.1 记录当前查看的目标任务 ID，隔离后续切换或关闭后的迟到响应
      inspectedTaskIDRef.current = taskID;
      setInspectedTaskID(taskID);
      setError(null);
      setMessages([]);
      setNextMessageID("");
      setHasMoreMessages(false);
      setRuns([]);
      setAgentRuns([]);
      setAssets([]);
      setTaskEvents([]);
      setSelectedRunID(null);

      // 3.2 优先从传入参数或已有任务列表中解析初始任务快照
      const initialTask =
        typeof taskOrID === "object"
          ? taskOrID
          : tasks.find((t) => t.id === taskID) || null;
      inspectedTaskRef.current = initialTask;
      setInspectedTask(initialTask);
      if (initialTask?.current_run_id) {
        setSelectedRunID(initialTask.current_run_id);
      }

      // 3.3 开启事件回放状态，实时推送事件先进入缓冲区
      isReplayingRef.current = true;
      realtimeBufferRef.current = [];
      eventsMapRef.current = new Map();
      lastReplaySequenceRef.current = 0;
      throughSequenceRef.current = 0;

      // 3.4 发送只读详情查询（使用直接父会话 ID，不调用 LoadSession）
      setLoadingDetail(true);
      const detailReqID = requestTaskDetail(taskID, "", 100, currentSessionID);
      pendingDetailReqIDRef.current = detailReqID;

      // 3.5 发送首次历史事件回放（从序列 0 开始固定水位）
      setLoadingEvents(true);
      const eventsReqID = requestTaskEvents(
        {
          taskID,
          afterSequence: 0,
          throughSequence: 0,
          limit: 100,
        },
        currentSessionID,
      );
      pendingEventsReqIDRef.current = eventsReqID;
    },
    [currentSessionID, requestTaskDetail, requestTaskEvents, tasks],
  );

  // 4.0 关闭子代理查看面板：重置活跃 ID，忽略迟到消息
  const closeInspection = useCallback(() => {
    inspectedTaskIDRef.current = null;
    inspectedTaskRef.current = null;
    pendingDetailReqIDRef.current = "";
    pendingEventsReqIDRef.current = "";
    isReplayingRef.current = false;
    realtimeBufferRef.current = [];
    eventsMapRef.current = new Map();
    setInspectedTaskID(null);
    setInspectedTask(null);
    setSelectedRunID(null);
    setTaskEvents([]);
    setMessages([]);
    setAssets([]);
    setError(null);
  }, []);

  // 5.0 切换选中的 Run
  const selectRun = useCallback(
    (runID: string) => {
      setSelectedRunID(runID);
    },
    [],
  );

  // 6.0 加载更多子会话历史消息
  const loadMoreMessages = useCallback(() => {
    const taskID = inspectedTaskIDRef.current;
    if (!taskID || !hasMoreMessages || !nextMessageID || loadingMoreMessages) {
      return;
    }
    setLoadingMoreMessages(true);
    requestTaskDetail(taskID, nextMessageID, 100, currentSessionID);
  }, [currentSessionID, hasMoreMessages, loadingMoreMessages, nextMessageID, requestTaskDetail]);

  // 7.0 刷新详情与事件（支持断线手动重试）
  const refreshInspection = useCallback(() => {
    const taskID = inspectedTaskIDRef.current;
    if (!taskID) return;

    setLoadingDetail(true);
    requestTaskDetail(taskID, "", 100, currentSessionID);

    // 重新从 0 检查并补取事件
    setLoadingEvents(true);
    isReplayingRef.current = true;
    requestTaskEvents(
      {
        taskID,
        afterSequence: 0,
        throughSequence: 0,
        limit: 100,
      },
      currentSessionID,
    );
  }, [currentSessionID, requestTaskDetail, requestTaskEvents]);

  // 8.0 接收并处理 subagent_task_detail_result 详情响应
  const handleDetailResult = useCallback(
    (payload?: SubagentTaskDetailResultPayload, requestID?: string) => {
      if (!payload?.task?.id) return;
      // 8.1 迟到响应隔离：如果当前已切换到其他任务或已关闭，直接丢弃
      if (payload.task.id !== inspectedTaskIDRef.current) {
        return;
      }
      setLoadingDetail(false);
      setLoadingMoreMessages(false);

      // 8.2 比较 task.updated_at 避免旧数据覆盖较新快照
      const current = inspectedTaskRef.current;
      if (
        !current ||
        !current.updated_at ||
        !payload.task.updated_at ||
        Date.parse(payload.task.updated_at) >= Date.parse(current.updated_at)
      ) {
        inspectedTaskRef.current = payload.task;
        setInspectedTask(payload.task);
      }

      setReadOnly(payload.read_only);
      setSessionReady(payload.session_ready);
      setRuns(payload.runs || []);
      setAgentRuns(payload.agent_runs || []);

      // 8.3 消息处理：若要求 full_sync_required 则重置；否则追加/更新
      if (payload.full_sync_required) {
        setMessages(payload.messages || []);
      } else if (payload.messages && payload.messages.length > 0) {
        setMessages((prev) => mergeHistoryMessages(prev, payload.messages));
      }
      setNextMessageID(payload.next_message_id || "");
      setHasMoreMessages(Boolean(payload.has_more_messages));

      // 8.4 资源合并：按 asset.id 合并
      if (payload.assets && payload.assets.length > 0) {
        setAssets((prev) => mergeAssets(prev, payload.assets));
      }
    },
    [],
  );

  // 9.0 接收并处理 subagent_task_events_result 历史事件分页响应
  const handleEventsResult = useCallback(
    (payload?: SubagentTaskEventsResultPayload, requestID?: string) => {
      if (!payload) return;
      const currentTaskID = inspectedTaskIDRef.current;
      // 9.1 迟到响应隔离
      if (!currentTaskID || (payload.task_id && payload.task_id !== currentTaskID)) {
        return;
      }

      // 9.2 将本页事件按 sequence 存入 map 去重
      if (payload.events && payload.events.length > 0) {
        for (const ev of payload.events) {
          if (ev.sequence) {
            eventsMapRef.current.set(ev.sequence, ev);
          }
        }
      }

      // 9.3 固定水位分页与下一页推进
      if (payload.has_more) {
        throughSequenceRef.current = payload.through_sequence;
        // 继续拉取下一页
        requestTaskEvents(
          {
            taskID: currentTaskID,
            runID: payload.run_id || undefined,
            afterSequence: payload.next_sequence,
            throughSequence: payload.through_sequence,
            limit: 100,
          },
          currentSessionID,
        );
      } else {
        // 9.4 本轮回放结束：合并实时缓冲区并结束回放状态
        isReplayingRef.current = false;
        setLoadingEvents(false);
        lastReplaySequenceRef.current = payload.next_sequence || 0;

        // 合并缓冲中的实时事件
        if (realtimeBufferRef.current.length > 0) {
          for (const ev of realtimeBufferRef.current) {
            if (ev.sequence && !eventsMapRef.current.has(ev.sequence)) {
              eventsMapRef.current.set(ev.sequence, ev);
            }
          }
          realtimeBufferRef.current = [];
        }

        // 排序并发布完整事件列表
        const sorted = Array.from(eventsMapRef.current.values()).sort(
          (a, b) => (a.sequence || 0) - (b.sequence || 0),
        );
        setTaskEvents(sorted);
      }
    },
    [currentSessionID, requestTaskEvents],
  );

  // 10.0 接收并处理实时推送 subagent_task_event
  const handleRealtimeTaskEvent = useCallback((payload?: SubagentTaskResultPayload) => {
    if (!payload?.task?.id) return;
    const currentTaskID = inspectedTaskIDRef.current;
    if (payload.task.id !== currentTaskID) return;

    // 10.1 如果正在回放，先推入实时缓冲区，防止乱序打乱分页游标
    if (isReplayingRef.current) {
      realtimeBufferRef.current.push(payload);
      return;
    }

    // 10.2 去重并更新事件列表
    if (payload.sequence) {
      if (eventsMapRef.current.has(payload.sequence)) {
        return;
      }
      eventsMapRef.current.set(payload.sequence, payload);
      const sorted = Array.from(eventsMapRef.current.values()).sort(
        (a, b) => (a.sequence || 0) - (b.sequence || 0),
      );
      setTaskEvents(sorted);
    }

    // 10.3 资源事件创建：合并资源引用
    if (payload.kind === "task.asset_created" && payload.assets && payload.assets.length > 0) {
      setAssets((prev) => mergeAssets(prev, payload.assets || []));
    }

    // 10.4 更新任务快照（依据 updated_at）
    const current = inspectedTaskRef.current;
    if (
      !current ||
      !current.updated_at ||
      !payload.task.updated_at ||
      Date.parse(payload.task.updated_at) >= Date.parse(current.updated_at)
    ) {
      inspectedTaskRef.current = payload.task;
      setInspectedTask(payload.task);
    }
  }, []);

  return {
    agentRuns,
    assets,
    closeInspection,
    error,
    handleDetailResult,
    handleEventsResult,
    handleRealtimeTaskEvent,
    hasMoreMessages,
    inspectedTask,
    inspectedTaskID,
    loadingDetail,
    loadingEvents,
    loadingMoreMessages,
    loadMoreMessages,
    messages,
    openInspection,
    readOnly,
    refreshInspection,
    runs,
    selectedRunID,
    selectRun,
    sessionReady,
    taskEvents,
  };
}

// 辅助函数：按 message.id 去重合并历史消息
function mergeHistoryMessages(
  existing: SessionHistoryMessage[],
  incoming: SessionHistoryMessage[],
): SessionHistoryMessage[] {
  const map = new Map<string, SessionHistoryMessage>();
  for (const m of existing) {
    map.set(m.id, m);
  }
  for (const m of incoming) {
    map.set(m.id, m);
  }
  return Array.from(map.values()).sort(
    (a, b) => Date.parse(a.created_at || "") - Date.parse(b.created_at || ""),
  );
}

// 辅助函数：按 asset.id 去重合并共享资源
function mergeAssets(existing: AssetSummary[], incoming: AssetSummary[]): AssetSummary[] {
  const map = new Map<string, AssetSummary>();
  for (const a of existing) {
    map.set(a.id, a);
  }
  for (const a of incoming) {
    map.set(a.id, a);
  }
  return Array.from(map.values()).sort(
    (a, b) => Date.parse(b.created_at || "") - Date.parse(a.created_at || ""),
  );
}
