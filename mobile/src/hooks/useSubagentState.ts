import { useCallback, useState } from "react";

import type {
  SubagentDefinition,
  SubagentDefinitionListResultPayload,
  SubagentDefinitionMutationResultPayload,
  SubagentTask,
  SubagentTaskEvent,
  SubagentTaskListResultPayload,
  SubagentTaskResultPayload,
} from "../protocol";

export function useSubagentState() {
  const [definitions, setDefinitions] = useState<SubagentDefinition[]>([]);
  const [tasks, setTasks] = useState<SubagentTask[]>([]);
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
    setTasks(payload?.tasks || []);
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
      setTasks((current) => upsertTask(current, task));
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
    setTasks([]);
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
  return sorted.slice(0, 64);
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
