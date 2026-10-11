import { useCallback, useMemo, useState } from "react";

import type {
  SubagentDefinition,
  SubagentDefinitionListResultPayload,
  SubagentDefinitionMutationResultPayload,
  SubagentTask,
  SubagentTaskEvent,
  SubagentTaskListResultPayload,
  SubagentTaskResultPayload,
} from "../protocol";

const emptyTasks: SubagentTask[] = [];

export function useSubagentState(currentSessionID: string) {
  const [definitions, setDefinitions] = useState<SubagentDefinition[]>([]);
  // 1.0 按直接父会话归档任务；当前会话沿子会话 ID 构建完整任务树。
  const [tasksBySession, setTasksBySession] = useState<Record<string, SubagentTask[]>>({});
  const tasks = useMemo(
    () => tasksForSession(tasksBySession, currentSessionID),
    [tasksBySession, currentSessionID],
  );
  const [message, setMessage] = useState("");
  const [events, setEvents] = useState<Record<string, SubagentTaskEvent[]>>({});

  const applyDefinitionList = useCallback((payload?: SubagentDefinitionListResultPayload) => {
    setDefinitions(payload?.definitions || []);
  }, []);

  const applyDefinitionMutation = useCallback((payload?: SubagentDefinitionMutationResultPayload) => {
    if (payload?.definitions) {
      setDefinitions(payload.definitions);
    } else if (payload?.definition) {
      setDefinitions((current) => upsertDefinition(current, payload.definition as SubagentDefinition));
    } else if (payload?.deleted_id) {
      setDefinitions((current) => current.filter((item) => item.id !== payload.deleted_id));
    }
    setMessage(payload?.message || "");
  }, []);

  const applyTaskList = useCallback((payload?: SubagentTaskListResultPayload) => {
    const sessionID = payload?.session_id?.trim();
    if (!sessionID) return;
    const incoming = payload?.tasks || emptyTasks;
    setTasksBySession((current) => {
      const next = { ...current };
      if (!next[sessionID]) next[sessionID] = emptyTasks;
      const grouped = new Map<string, SubagentTask[]>();
      for (const task of incoming) {
        const parentSessionID = task.parent_session_id?.trim();
        if (!parentSessionID) continue;
        grouped.set(parentSessionID, [...(grouped.get(parentSessionID) || emptyTasks), task]);
      }
      // 1.1 列表含嵌套任务，但每个任务只在自己的直接父会话存一份。
      // 1.2 合并列表与已到达的实时事件，避免旧快照覆盖新状态。
      for (const [parentSessionID, items] of grouped) {
        next[parentSessionID] = mergeTaskList(next[parentSessionID] || emptyTasks, items);
      }
      return next;
    });
  }, []);

  const applyTaskResult = useCallback((payload?: SubagentTaskResultPayload) => {
    // 1.0 若携带事件序列和类型，记录子代理实时事件
    if (payload?.task && payload.sequence && payload.kind) {
      const runtimeEvent: SubagentTaskEvent = {
        task_id: payload.task.id,
        sequence: payload.sequence,
        kind: payload.kind,
        run_id: payload.run_id,
        tool_call_id: payload.tool_call_id,
        assets: payload.assets,
        content: payload.content,
        tool_name: payload.tool_name,
        arguments: payload.arguments,
        status: payload.status,
        error_code: payload.error_code,
        error_message: payload.error_message,
        truncated: payload.truncated,
        delta: payload.delta,
        emitted_at: payload.emitted_at,
      };
      setEvents((current) => appendTaskEvent(current, runtimeEvent));
    }

    // 2.0 更新子任务快照状态
    if (payload?.task) {
      const task = payload.sequence
        ? { ...payload.task, event_sequence: payload.sequence, event_kind: payload.kind }
        : payload.task;
      setTasksBySession((current) => {
        const parentSessionID = task.parent_session_id?.trim();
        if (!parentSessionID) return current;
        // 2.1 实时更新只写直接父会话；祖先会话从同一任务树读取，不存副本。
        return {
          ...current,
          [parentSessionID]: upsertTask(current[parentSessionID] || emptyTasks, task),
        };
      });
    }
    setMessage(
      payload?.message
      || (payload?.timed_out ? "等待窗口已结束，子智能体仍在后台执行。" : "")
      || payload?.task?.change_set?.message
      || "",
    );
  }, []);

  const clear = useCallback(() => {
    setDefinitions([]);
    setTasksBySession({});
    setMessage("");
    setEvents({});
  }, []);

  return {
    applyDefinitionList,
    applyDefinitionMutation,
    applyTaskList,
    applyTaskResult,
    clear,
    definitions,
    message,
    setMessage,
    tasks,
    events,
  };
}

function upsertDefinition(current: SubagentDefinition[], value: SubagentDefinition) {
  const next = current.filter((item) => item.id !== value.id);
  next.push(value);
  return next.sort((left, right) => left.name.localeCompare(right.name));
}

function tasksForSession(tasksBySession: Record<string, SubagentTask[]>, sessionID: string) {
  if (!sessionID) return emptyTasks;
  const tasks: SubagentTask[] = [];
  const pending = [sessionID];
  const visitedSessions = new Set<string>();
  const visitedTasks = new Set<string>();
  while (pending.length > 0 && tasks.length < 500) {
    const parentSessionID = pending.shift()!;
    if (visitedSessions.has(parentSessionID)) continue;
    visitedSessions.add(parentSessionID);
    for (const task of tasksBySession[parentSessionID] || emptyTasks) {
      if (visitedTasks.has(task.id)) continue;
      visitedTasks.add(task.id);
      tasks.push(task);
      if (task.child_session_id) pending.push(task.child_session_id);
      if (tasks.length >= 500) break;
    }
  }
  return tasks.length > 0 ? tasks : emptyTasks;
}

// 3.0 任务快照更新：比较 task.updated_at，避免旧事件或迟到响应覆盖新状态
function upsertTask(current: SubagentTask[], value: SubagentTask) {
  const existing = current.find((item) => item.id === value.id);
  if (existing) {
    if (existing.updated_at && value.updated_at) {
      const existingTime = Date.parse(existing.updated_at);
      const valueTime = Date.parse(value.updated_at);
      if (Number.isFinite(existingTime) && Number.isFinite(valueTime) && valueTime < existingTime) {
        return current;
      }
    }
    if (
      existing.event_sequence !== undefined &&
      value.event_sequence !== undefined &&
      value.event_sequence < existing.event_sequence
    ) {
      return current;
    }
  }
  const next = current.filter((item) => item.id !== value.id);
  next.push(existing ? { ...existing, ...value } : value);
  const sorted = next.sort((left, right) => Date.parse(right.created_at) - Date.parse(left.created_at));
  // 3.1 历史任务不能在一次实时更新后被截成 64 条；服务端列表上限为 500。
  return sorted.slice(0, 500);
}

function mergeTaskList(current: SubagentTask[], incoming: SubagentTask[]) {
  // 4.0 列表响应可能早于实时事件生成、却晚于事件到达；合并而非整份覆盖。
  const merged = new Map(current.map((task) => [task.id, task]));
  for (const task of incoming) {
    const existing = merged.get(task.id);
    if (existing && isOlderTask(task, existing)) continue;
    merged.set(task.id, existing ? { ...existing, ...task } : task);
  }
  return [...merged.values()]
    .sort((left, right) => Date.parse(right.created_at) - Date.parse(left.created_at))
    .slice(0, 500);
}

function isOlderTask(value: SubagentTask, existing: SubagentTask) {
  const valueTime = Date.parse(value.updated_at);
  const existingTime = Date.parse(existing.updated_at);
  if (Number.isFinite(valueTime) && Number.isFinite(existingTime)) {
    if (valueTime < existingTime) return true;
    if (valueTime > existingTime) return false;
  }
  // 4.1 同一更新时间内，列表快照不携带事件序号，保留已收到的实时事件。
  return existing.event_sequence !== undefined
    && (value.event_sequence === undefined || value.event_sequence < existing.event_sequence);
}

function appendTaskEvent(current: Record<string, SubagentTaskEvent[]>, value: SubagentTaskEvent) {
  const previous = current[value.task_id] || [];
  if (previous.some((event) => event.sequence === value.sequence)) {
    return current;
  }
  const next = [...previous, value].sort((left, right) => left.sequence - right.sequence);
  const updated = { ...current, [value.task_id]: next.slice(-256) };
  const keys = Object.keys(updated);
  if (keys.length > 64) {
    delete updated[keys[0]];
  }
  return updated;
}
