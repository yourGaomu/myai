import { useCallback, useState } from "react";

import type {
  SubagentDefinition,
  SubagentDefinitionListResultPayload,
  SubagentDefinitionMutationResultPayload,
  SubagentTask,
  SubagentTaskListResultPayload,
  SubagentTaskResultPayload,
} from "../protocol";

export function useSubagentState() {
  const [definitions, setDefinitions] = useState<SubagentDefinition[]>([]);
  const [tasks, setTasks] = useState<SubagentTask[]>([]);
  const [message, setMessage] = useState("");

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
    if (payload?.task) {
      setTasks((current) => upsertTask(current, payload.task));
    }
    setMessage(payload?.message || payload?.task?.change_set?.message || "");
  }, []);

  const clear = useCallback(() => {
    setDefinitions([]);
    setTasks([]);
    setMessage("");
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
  };
}

function upsertDefinition(current: SubagentDefinition[], value: SubagentDefinition) {
  const next = current.filter((item) => item.id !== value.id);
  next.push(value);
  return next.sort((left, right) => left.name.localeCompare(right.name));
}

function upsertTask(current: SubagentTask[], value: SubagentTask) {
  const next = current.filter((item) => item.id !== value.id);
  next.push(value);
  return next.sort((left, right) => Date.parse(right.created_at) - Date.parse(left.created_at));
}
