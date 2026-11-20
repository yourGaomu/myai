import type { SubagentTask } from "../protocol";
import type { ToolCallStep } from "./chatRenderItems";

type LaunchState = "starting" | "queued" | "started" | "failed" | "canceled" | "skipped" | "unconfirmed";

export function findSubagentForLaunch(
  tool: ToolCallStep,
  tasks: SubagentTask[] | undefined,
  sessionID: string,
  runID?: string,
) {
  if (!tool.toolCallID) return undefined;
  const candidates = tasks?.filter((task) =>
    task.parent_session_id === sessionID && task.parent_tool_call_id === tool.toolCallID,
  );
  if (!candidates?.length) return undefined;
  if (runID) {
    const exact = candidates.find((task) => task.parent_run_id === runID);
    if (exact) return exact;
  }
  const only = candidates.length === 1 ? candidates[0] : undefined;
  return only && (!runID || !only.parent_run_id || only.parent_run_id === runID) ? only : undefined;
}

// 启动调用的结果和子任务的生命周期是两回事。
export function subagentLaunchState(tool: ToolCallStep, task: SubagentTask | undefined, turnRunning: boolean): LaunchState {
  if (tool.toolStatus === "skipped") return "skipped";
  if (tool.status === "error" || tool.error) return "failed";
  if (tool.status === "completed") return "started";
  if (task) {
    if (task.started_at || task.status === "running" || task.status === "waiting_subagents" || task.status === "waiting_permission" || task.status === "succeeded") {
      return "started";
    }
    return task.status;
  }
  return turnRunning ? "starting" : "unconfirmed";
}

export const subagentLaunchLabel: Record<LaunchState, string> = {
  starting: "正在启动子代理",
  queued: "子代理已排队",
  started: "已启动子代理",
  failed: "启动子代理失败",
  canceled: "子代理启动已取消",
  skipped: "已跳过子代理启动",
  unconfirmed: "未收到子代理启动结果",
};
