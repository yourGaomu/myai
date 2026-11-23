import { useEffect, useLayoutEffect, useMemo, useState, type RefObject } from "react";

import type {
  ErrorPayload, MCPDetailPayload, MCPDetailResultPayload, MCPListResultPayload, MCPReloadResultPayload,
  MCPServerSummary, MCPToolSchemaSummary, MCPToolSummary, RelayMessage, SessionSummary,
} from "../protocol";
import type { RelayScope } from "../types/app";
import { newRequestID } from "../utils/ids";

const requestPrefixes = { mcp_list: "mcp-list-", mcp_reload: "mcp-reload-", mcp_detail: "mcp-detail-" };
type Operation = keyof typeof requestPrefixes;
type SendEnvelope = (type: Operation, overrides: Partial<RelayMessage>) => boolean;
type Request = {
  id: string;
  scope: RelayScope;
  deadline: number;
  timer: ReturnType<typeof setTimeout>;
  detail?: { serverName: string; sessionID: string; cursor?: string; listRevision?: number };
};

export type MCPInventory = {
  servers: MCPServerSummary[] | null;
  status: "idle" | "loading" | "ready" | "unsupported" | "timeout" | "error";
  catalogRevision?: number;
};

export type MCPDetail = {
  serverName: string;
  selectedToolID?: string;
  // A non-null result may remain visible if loading the next page fails.
  result: MCPDetailResultPayload | null;
  status: "loading" | "ready" | "unsupported" | "timeout" | "error" | "changed";
};

type MCPState = {
  inventory: MCPInventory;
  detail: MCPDetail | null;
  reloading: boolean;
  reloadMessage: string;
};

const initialState: MCPState = {
  inventory: { servers: null, status: "idle" },
  detail: null,
  reloading: false,
  reloadMessage: "",
};
const timeouts = { mcp_list: 15_000, mcp_reload: 60_000, mcp_detail: 15_000 };
const reloadFailed = "MCP 重载失败，请检查电脑端 Agent 后重试。";

function isOperation(value: unknown): value is Operation {
  return typeof value === "string" && Object.hasOwn(requestPrefixes, value);
}

function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function isServerSummary(value: unknown): value is MCPServerSummary {
  const server = value as MCPServerSummary | undefined;
  return typeof server?.name === "string" &&
    (server.state === "loaded" || server.state === "disabled" || server.state === "unavailable") &&
    isCount(server.inventory_tool_count) && isCount(server.exposed_tool_count);
}

function isOptionalString(value: unknown) {
  return value === undefined || typeof value === "string";
}

function isOptionalBoolean(value: unknown) {
  return value === undefined || typeof value === "boolean";
}

function isSchemaSummary(value: unknown): value is MCPToolSchemaSummary {
  const schema = value as MCPToolSchemaSummary | undefined;
  return !!schema && ["undeclared", "empty", "unsupported", "fields"].includes(schema.state) &&
    isOptionalBoolean(schema.truncated) && (schema.fields === undefined ||
      (Array.isArray(schema.fields) && schema.fields.every((field) =>
        typeof field?.name === "string" && isOptionalString(field.type) && typeof field.required === "boolean")));
}

function isToolSummary(value: unknown, sessionEvaluated: boolean): value is MCPToolSummary {
  const tool = value as MCPToolSummary | undefined;
  return typeof tool?.id === "string" && tool.id.length > 0 && typeof tool.name === "string" && tool.name.length > 0 &&
    isOptionalString(tool.title) && isOptionalString(tool.description) && isOptionalString(tool.alias) &&
    typeof tool.host_only === "boolean" && isOptionalBoolean(tool.metadata_truncated) &&
    (tool.host_only ? tool.eligibility === "host_only"
      : sessionEvaluated ? tool.eligibility === "available" || tool.eligibility === "filtered"
      : tool.eligibility === "not_evaluated") &&
    isSchemaSummary(tool.input) && isSchemaSummary(tool.output);
}

function isDetailResult(value: unknown): value is MCPDetailResultPayload {
  const result = value as MCPDetailResultPayload | undefined;
  return !!result && isServerSummary(result.server) && isCount(result.catalog_revision) &&
    typeof result.session_evaluated === "boolean" && isOptionalString(result.next_cursor) &&
    Array.isArray(result.tools) && result.tools.length <= 25 &&
    result.tools.every((tool) => isToolSummary(tool, result.session_evaluated));
}

function isUnsupported(error: ErrorPayload | undefined, operation: Operation) {
  // Older relays reject unknown requests. Older Agents may silently ignore them.
  return typeof error?.message === "string" &&
    new RegExp(`^(?:unsupported|unknown) (?:client )?message type:\\s*${operation}\\s*$`, "i").test(error.message);
}

// Summary/reload are global to the paired Agent. Detail belongs to the selected server and explicit session as well.
export function createMCPRequests({ getScope, getSessionID, sendEnvelope, onChange, onReloaded }: {
  getScope: () => RelayScope;
  getSessionID: () => string;
  sendEnvelope: SendEnvelope;
  onChange: (state: MCPState) => void;
  onReloaded?: () => void;
}) {
  let state = initialState;
  let scope: RelayScope | undefined;
  let sessionID: string | undefined;
  let sessionPolicy: string | undefined;
  let agentSessionID: string | undefined;
  const pending: Partial<Record<Operation, Request>> = {};

  function update(patch: Partial<MCPState>) {
    state = { ...state, ...patch };
    onChange(state);
  }

  function setListStatus(status: MCPInventory["status"]) {
    update({ inventory: { ...state.inventory, status } });
  }

  function setDetailStatus(status: MCPDetail["status"]) {
    if (state.detail) update({ detail: { ...state.detail, status } });
  }

  function cancel(operation: Operation) {
    clearTimeout(pending[operation]?.timer);
    delete pending[operation];
  }

  function closeDetail() {
    cancel("mcp_detail");
    if (state.detail) update({ detail: null });
  }

  function openTool(toolID: string) {
    syncScope();
    const detail = state.detail;
    if (detail?.result?.tools.some((tool) => tool.id === toolID)) {
      update({ detail: { ...detail, selectedToolID: toolID } });
    }
  }

  function backFromDetail() {
    if (state.detail?.selectedToolID) update({ detail: { ...state.detail, selectedToolID: undefined } });
    else closeDetail();
  }

  function clear() {
    cancel("mcp_list");
    cancel("mcp_reload");
    cancel("mcp_detail");
  }

  function syncScope() {
    const current = getScope();
    const previous = scope;
    scope = { ...current };
    const currentSessionID = getSessionID();
    if (!previous || previous.connectionID !== current.connectionID || previous.pairingID !== current.pairingID) {
      agentSessionID = undefined;
      sessionPolicy = undefined;
    }
    if (previous && previous.pairingID !== current.pairingID) {
      clear();
      update(initialState);
    } else if (previous && previous.connectionID !== current.connectionID) {
      const reloadMessage = pending.mcp_reload ? "中继连接中断，MCP 重载结果未确认。" : state.reloadMessage;
      clear();
      update({ inventory: { ...state.inventory, status: "idle" }, detail: null, reloading: false, reloadMessage });
    } else if (sessionID !== currentSessionID) {
      closeDetail();
      sessionPolicy = undefined;
    }
    sessionID = currentSessionID;
    return current;
  }

  // Called when session replies are accepted, before React can batch a newer detail response with them.
  // Agent mode and permission mode are the selection policy fields currently published in SessionSummary.
  function observeSessionPolicy(session: SessionSummary | undefined, currentAgentSessionID?: string) {
    syncScope();
    if (currentAgentSessionID !== undefined) {
      if (sessionID && agentSessionID !== currentAgentSessionID) closeDetail();
      agentSessionID = currentAgentSessionID;
    }
    if (sessionID && session?.id === sessionID) {
      const policy = JSON.stringify([session.agent_mode, session.permission_mode]);
      if (sessionPolicy !== policy) closeDetail();
      sessionPolicy = policy;
    }
  }

  function expire(operation: Operation) {
    cancel(operation);
    if (operation === "mcp_list") setListStatus("timeout");
    else if (operation === "mcp_detail") setDetailStatus("timeout");
    else update({ reloading: false, reloadMessage: "等待 MCP 重载确认超时，请检查电脑端 Agent。" });
  }

  function start(operation: Operation, detail?: Request["detail"]) {
    const current = syncScope();
    if (pending[operation] || !current.connectionID) return false;

    const id = requestPrefixes[operation] + newRequestID();
    const timeout = timeouts[operation];
    pending[operation] = {
      id,
      scope: { ...current },
      detail: detail && { ...detail, listRevision: state.inventory.catalogRevision },
      deadline: Date.now() + timeout,
      timer: setTimeout(() => {
        syncScope();
        if (pending[operation]?.id === id) expire(operation);
      }, timeout),
    };
    if (operation === "mcp_list") setListStatus("loading");
    else if (detail) {
      const result = detail.cursor ? state.detail?.result ?? null : null;
      update({ detail: { serverName: detail.serverName, selectedToolID: detail.cursor ? state.detail?.selectedToolID : undefined, result, status: "loading" } });
    } else update({ reloading: true, reloadMessage: "" });

    const payload: MCPDetailPayload | {} = detail
      ? { server_name: detail.serverName, ...(detail.cursor ? { cursor: detail.cursor } : {}) }
      : {};
    // An empty detail session explicitly opts out; the relay sender omits it on the wire.
    if (sendEnvelope(operation, { request_id: id, session_id: detail?.sessionID ?? "", payload })) return true;
    cancel(operation);
    if (operation === "mcp_list") setListStatus("error");
    else if (operation === "mcp_detail") setDetailStatus("error");
    else update({ reloading: false, reloadMessage: "MCP 重载请求未发出。请在连接恢复后重试。" });
    return false;
  }

  function openDetail(serverName: string) {
    const current = syncScope();
    if (!current.connectionID || !state.inventory.servers?.some((server) => server.name === serverName) ||
        (pending.mcp_detail && state.detail?.serverName === serverName)) return false;
    cancel("mcp_detail");
    return start("mcp_detail", { serverName, sessionID: getSessionID() });
  }

  function loadMoreDetail() {
    syncScope();
    const detail = state.detail;
    const cursor = detail?.result?.next_cursor;
    if (!detail || !cursor) return false;
    return start("mcp_detail", { serverName: detail.serverName, sessionID: getSessionID(), cursor });
  }

  function retryDetail() {
    syncScope();
    const detail = state.detail;
    if (!detail || detail.status === "loading") return false;
    if (detail.result) return loadMoreDetail();
    return openDetail(detail.serverName);
  }

  function catalogChanged() {
    cancel("mcp_detail");
    update({
      detail: state.detail ? { serverName: state.detail.serverName, result: null, status: "changed" } : null,
      inventory: state.inventory.status === "ready" ? { ...state.inventory, status: "idle" } : state.inventory,
    });
  }

  function applyDetailResult(message: RelayMessage, request: NonNullable<Request["detail"]>) {
    const result = message.payload;
    if (!isDetailResult(result) || (!request.sessionID && result.session_evaluated)) {
      setDetailStatus("error");
      return;
    }
    const knownRevision = state.inventory.catalogRevision;
    if (knownRevision !== undefined && request.listRevision !== knownRevision && result.catalog_revision < knownRevision) {
      catalogChanged();
      return;
    }
    const previous = request.cursor ? state.detail?.result : null;
    if (request.cursor && (!previous || previous.catalog_revision !== result.catalog_revision ||
        previous.session_evaluated !== result.session_evaluated)) {
      catalogChanged();
      return;
    }
    const tools = [...(previous?.tools ?? []), ...result.tools];
    if (new Set(tools.map((tool) => tool.id)).size !== tools.length ||
        (request.cursor && result.next_cursor === request.cursor)) {
      setDetailStatus("error");
      return;
    }
    update({
      detail: { serverName: request.serverName, selectedToolID: state.detail?.selectedToolID, result: { ...result, tools }, status: "ready" },
      ...(state.inventory.status === "ready" && state.inventory.catalogRevision !== undefined &&
        state.inventory.catalogRevision !== result.catalog_revision ? { inventory: { ...state.inventory, status: "idle" } } : {}),
    });
  }

  function handleMessage(message: RelayMessage) {
    const error = message.type === "error" ? message.payload as ErrorPayload | undefined : undefined;
    const requestID = typeof message.request_id === "string" ? message.request_id : "";
    const ownedID = (Object.keys(requestPrefixes) as Operation[]).find((op) => requestID.startsWith(requestPrefixes[op]));
    const operation = message.type === "mcp_list_result" ? "mcp_list"
      : message.type === "mcp_reload_result" ? "mcp_reload"
      : message.type === "mcp_detail_result" ? "mcp_detail"
      : message.type === "error" ? ownedID ?? (isOperation(error?.request_type) ? error.request_type : undefined)
      : undefined;
    if (!operation) return false;

    const current = syncScope();
    const request = pending[operation];
    if (!request || request.id !== requestID || request.scope.pairingID !== current.pairingID ||
        request.scope.connectionID !== current.connectionID || (error?.request_type && error.request_type !== operation)) return true;
    if (Date.now() >= request.deadline) {
      expire(operation);
      return true;
    }
    if (request.detail && (request.detail.serverName !== state.detail?.serverName ||
        request.detail.sessionID !== sessionID || request.detail.sessionID !== (message.session_id ?? ""))) return true;
    if (request.detail && message.type === "mcp_detail_result") {
      const target = (message.payload as MCPDetailResultPayload | undefined)?.server?.name;
      if (typeof target === "string" && target !== request.detail.serverName) return true;
    }
    if (request.detail && message.type === "error") {
      // Detail errors can echo their server; relay-level and legacy errors may omit it.
      const target = (error as (ErrorPayload & { server_name?: string }) | undefined)?.server_name;
      if (target !== undefined && target !== request.detail.serverName) return true;
    }
    cancel(operation);

    if (message.type === "error") {
      if (operation === "mcp_list") setListStatus(isUnsupported(error, operation) ? "unsupported" : "error");
      else if (operation === "mcp_detail") {
        if (error?.code === "mcp_catalog_changed") catalogChanged();
        else setDetailStatus(isUnsupported(error, operation) ? "unsupported" : "error");
      } else update({ reloading: false, reloadMessage: reloadFailed });
    } else if (operation === "mcp_list") {
      const payload = message.payload as MCPListResultPayload | undefined;
      if (Array.isArray(payload?.servers) && payload.servers.every(isServerSummary)) {
        const catalogRevision = isCount(payload.catalog_revision) ? payload.catalog_revision : undefined;
        const detail = state.detail;
        if (detail && !payload.servers.some((server) => server.name === detail.serverName)) closeDetail();
        else if (detail?.result && catalogRevision !== undefined && catalogRevision !== detail.result.catalog_revision) catalogChanged();
        update({ inventory: { servers: payload.servers, status: "ready", ...(catalogRevision === undefined ? {} : { catalogRevision }) } });
      } else setListStatus("error");
    } else if (request.detail) {
      applyDetailResult(message, request.detail);
    } else {
      const reloaded = (message.payload as MCPReloadResultPayload | undefined)?.reloaded === true;
      update({ reloading: false, reloadMessage: reloaded ? "MCP 重载成功。" : reloadFailed });
      if (reloaded) {
        closeDetail();
        // Replace any earlier inventory request so only a post-reload snapshot can be applied.
        cancel("mcp_list");
        start("mcp_list");
        onReloaded?.();
      }
    }
    return true;
  }

  return {
    backFromDetail,
    clear,
    closeDetail,
    handleDisconnect: syncScope,
    handleMessage,
    loadMoreDetail,
    observeSessionPolicy,
    openDetail,
    openTool,
    refresh: () => start("mcp_list"),
    reload: () => start("mcp_reload"),
    retryDetail,
    syncScope,
  };
}

export function useMCPState({ relayScopeRef, sessionIDRef, sessionID, sendEnvelope, onReloaded }: {
  relayScopeRef: RefObject<RelayScope>;
  sessionIDRef: RefObject<string>;
  sessionID: string;
  sendEnvelope: SendEnvelope;
  onReloaded?: () => void;
}) {
  const [state, setState] = useState(initialState);
  const requests = useMemo(() => createMCPRequests({
    getScope: () => relayScopeRef.current,
    getSessionID: () => sessionIDRef.current,
    sendEnvelope,
    onChange: setState,
    onReloaded,
  }), [onReloaded, relayScopeRef, sendEnvelope, sessionIDRef]);
  useLayoutEffect(() => { requests.syncScope(); }, [requests, sessionID]);
  useEffect(() => requests.clear, [requests]);
  return { ...state, ...requests };
}
