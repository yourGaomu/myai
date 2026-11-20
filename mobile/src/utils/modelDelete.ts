import type { ErrorPayload, ModelConfigMutationResultPayload, ModelSummary, RelayMessage } from "../protocol";
import type { RelayScope } from "../types/app";
import { newRequestID } from "./ids";

const deleteType = "model_config_delete";
const deleteRequestPrefix = "model-delete-";
const deleteTimeoutMs = 30_000;
const timeoutMessage = "等待电脑端确认超时。请先刷新模型列表确认结果，再决定是否重新删除。";
const disconnectedMessage = "连接中断，删除结果未确认。请在重连后刷新模型列表。";
const notSentMessage = "删除请求未发出。请等中继连接恢复后重新操作。";

export type ModelDeleteFeedback = {
  modelID: string;
  status: "pending" | "success" | "error";
  message: string;
};

type DeleteError = ErrorPayload & { transport_error?: boolean; error_code?: string };
type SendDelete = (type: typeof deleteType, envelope: Partial<RelayMessage>) => boolean;

type PendingDelete = {
  modelID: string;
  requestID: string;
  scope: RelayScope;
  deadline: number;
  timer: ReturnType<typeof setTimeout>;
};

// Keep destructive requests independent of list refreshes and other model actions.
export function createModelDeleteTracker({
  getScope,
  onFeedback,
  onDeleted,
}: {
  getScope: () => RelayScope;
  onFeedback: (feedback: ModelDeleteFeedback) => void;
  onDeleted: (models: ModelSummary[]) => void;
}) {
  let pending: PendingDelete | undefined;

  function clear() {
    if (pending) clearTimeout(pending.timer);
    pending = undefined;
  }

  function finish(status: "success" | "error", message: string) {
    if (!pending) return;
    const { modelID } = pending;
    clear();
    onFeedback({ modelID, status, message });
  }

  function sameScope(scope: RelayScope) {
    const current = getScope();
    return scope.pairingID === current.pairingID && scope.connectionID === current.connectionID;
  }

  function start(id: string, send: SendDelete) {
    const modelID = id.trim();
    if (!modelID || pending) return false;
    const scope = { ...getScope() };
    if (!scope.connectionID) {
      onFeedback({ modelID, status: "error", message: notSentMessage });
      return false;
    }

    const requestID = deleteRequestPrefix + newRequestID();
    const request: PendingDelete = {
      modelID,
      requestID,
      scope,
      deadline: Date.now() + deleteTimeoutMs,
      timer: setTimeout(() => {
        if (pending === request) finish("error", sameScope(scope) ? timeoutMessage : disconnectedMessage);
      }, deleteTimeoutMs),
    };
    pending = request;
    onFeedback({ modelID, status: "pending", message: "正在等待电脑端确认删除…" });
    let sent: boolean;
    try {
      sent = send(deleteType, { request_id: requestID, session_id: "", payload: { id: modelID } });
    } catch {
      sent = false;
    }
    if (!sent && pending === request) finish("error", notSentMessage);
    return sent;
  }

  function handleMessage(message: RelayMessage) {
    const requestID = message.request_id;
    const sentByTracker = typeof requestID === "string" && requestID.startsWith(deleteRequestPrefix);
    if (message.type === "error") {
      const payload = (message.payload || {}) as DeleteError;
      if (!sentByTracker && payload.request_type !== deleteType) return false;
      const request = match(message.request_id);
      if (request && (!payload.request_type || payload.request_type === deleteType)) {
        finish("error", deletionErrorMessage(payload));
      }
      return true;
    }
    if (message.type === "model_config_mutation_result") {
      const payload = message.payload as ModelConfigMutationResultPayload | undefined;
      if (!sentByTracker && !payload?.deleted_id) return false;
      const request = match(message.request_id);
      if (request && payload?.deleted_id === request.modelID && Array.isArray(payload.models) &&
          payload.models.every((model) => typeof model?.id === "string" && model.id !== request.modelID)) {
        finish("success", "模型配置已删除。");
        onDeleted(payload.models);
      }
      return true;
    }
    return false;
  }

  function match(requestID?: string) {
    if (!pending || requestID !== pending.requestID) return undefined;
    if (!sameScope(pending.scope)) {
      finish("error", disconnectedMessage);
      return undefined;
    }
    if (Date.now() >= pending.deadline) {
      finish("error", timeoutMessage);
      return undefined;
    }
    return pending;
  }

  return {
    start,
    handleMessage,
    disconnect: () => finish("error", disconnectedMessage),
    clear,
  };
}

function deletionErrorMessage(payload: DeleteError) {
  const reason = typeof payload.message === "string" ? payload.message : "";
  if (payload.transport_error) return "无法确认中继是否收到删除请求。请在连接恢复后先刷新模型列表。";
  if (payload.error_code === "client_auth_unavailable") return "中继暂时无法验证配对。请检查中继状态，连接恢复后再操作。";
  if (payload.error_code === "client_auth_rejected" || reason === "client token is invalid or expired") {
    return "中继配对已失效。请到「连接与配对」重新配对。";
  }
  if (reason.startsWith("cannot delete the default model")) return "这是默认模型。请先将另一个已启用的模型设为默认。";
  if (reason.startsWith("cannot delete the last enabled model")) return "这是最后一个启用的模型。请先启用或添加另一个模型。";
  if (/^cannot delete model .+ because it is used by a session\b/.test(reason)) {
    return "仍有会话使用此模型。请在每个引用它的会话中切换到其他已启用的模型，再删除此配置。";
  }
  if (/^(model not found:|repository record not found$)/.test(reason)) return "找不到此配置。请刷新模型列表，核对配置 ID。";
  if (reason.startsWith("agent is not online")) return "电脑端 Agent 离线。请确认电脑端已启动并连上中继，再重新操作。";
  return "电脑端返回删除错误。请先刷新模型列表确认结果，如果问题持续，请检查电脑端 Agent。";
}
