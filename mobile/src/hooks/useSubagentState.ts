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
    if (payload?.task && payload.sequence && payload.kind) {
      const runtimeEvent: SubagentTaskEvent = {
        task_id: payload.task.id,
        sequence: payload.sequence,
        kind: payload.kind,
        run_id: payload.run_id,
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

function upsertTask(current: SubagentTask[], value: SubagentTask) {
  const existing = current.find((item) => item.id === value.id);
  if (
    existing?.event_sequence !== undefined &&
    value.event_sequence !== undefined &&
    value.event_sequence < existing.event_sequence
  ) {
    return current;
  }
  const next = current.filter((item) => item.id !== value.id);
  next.push(existing ? { ...existing, ...value } : value);
  return next.sort((left, right) => Date.parse(right.created_at) - Date.parse(left.created_at));
}

function appendTaskEvent(current: Record<string, SubagentTaskEvent[]>, value: SubagentTaskEvent) {
  const previous = current[value.task_id] || [];
  if (previous.some((event) => event.sequence === value.sequence)) {
    return current;
  }
  const next = [...previous, value].sort((left, right) => left.sequence - right.sequence);
  return { ...current, [value.task_id]: next.slice(-256) };
}
