import { useEffect, useMemo, useState, type RefObject } from "react";

import type { ErrorPayload, MCPListResultPayload, MCPReloadResultPayload, MCPServerSummary, RelayMessage } from "../protocol";
import type { RelayScope } from "../types/app";
import { newRequestID } from "../utils/ids";

type Operation = "mcp_list" | "mcp_reload";
type SendEnvelope = (type: Operation, overrides: Partial<RelayMessage>) => boolean;
type Request = {
  id: string;
  scope: RelayScope;
  deadline: number;
  timer: ReturnType<typeof setTimeout>;
};

export type MCPInventory = {
  servers: MCPServerSummary[] | null;
  status: "idle" | "loading" | "ready" | "unsupported" | "timeout" | "error";
};

type MCPState = {
  inventory: MCPInventory;
  reloading: boolean;
  reloadMessage: string;
};

const initialState: MCPState = {
  inventory: { servers: null, status: "idle" },
  reloading: false,
  reloadMessage: "",
};
const timeouts = { mcp_list: 15_000, mcp_reload: 60_000 };
const requestPrefixes = { mcp_list: "mcp-list-", mcp_reload: "mcp-reload-" };
const reloadFailed = "MCP 重载失败，请检查电脑端 Agent 后重试。";

function isOperation(value: unknown): value is Operation {
  return value === "mcp_list" || value === "mcp_reload";
}

function isServerSummary(value: unknown): value is MCPServerSummary {
  const server = value as MCPServerSummary | undefined;
  return typeof server?.name === "string" &&
    (server.state === "loaded" || server.state === "disabled" || server.state === "unavailable") &&
    Number.isSafeInteger(server.inventory_tool_count) && server.inventory_tool_count >= 0 &&
    Number.isSafeInteger(server.exposed_tool_count) && server.exposed_tool_count >= 0;
}

// Both operations are global to the paired Agent. Only a reply from their owning connection can settle them.
export function createMCPRequests({ getScope, sendEnvelope, onChange }: {
  getScope: () => RelayScope;
  sendEnvelope: SendEnvelope;
  onChange: (state: MCPState) => void;
}) {
  let state = initialState;
  let scope: RelayScope | undefined;
  const pending: Partial<Record<Operation, Request>> = {};

  function update(patch: Partial<MCPState>) {
    state = { ...state, ...patch };
    onChange(state);
  }

  function setListStatus(status: MCPInventory["status"]) {
    update({ inventory: { ...state.inventory, status } });
  }

  function cancel(operation: Operation) {
    clearTimeout(pending[operation]?.timer);
    delete pending[operation];
  }

  function clear() {
    cancel("mcp_list");
    cancel("mcp_reload");
  }

  function syncScope() {
    const current = getScope();
    const previous = scope;
    scope = { ...current };
    if (previous && previous.pairingID !== current.pairingID) {
      clear();
      update(initialState);
    } else if (previous && previous.connectionID !== current.connectionID) {
      markDisconnected();
    }
    return current;
  }

  function markDisconnected() {
    const reloadMessage = pending.mcp_reload ? "中继连接中断，MCP 重载结果未确认。" : state.reloadMessage;
    clear();
    update({ inventory: { ...state.inventory, status: "idle" }, reloading: false, reloadMessage });
  }

  function sameScope(left: RelayScope, right: RelayScope) {
    return left.pairingID === right.pairingID && left.connectionID === right.connectionID;
  }

  function expire(operation: Operation) {
    cancel(operation);
    if (operation === "mcp_list") setListStatus("timeout");
    else update({ reloading: false, reloadMessage: "等待 MCP 重载确认超时，请检查电脑端 Agent。" });
  }

  function start(operation: Operation) {
    const current = syncScope();
    if (pending[operation] || !current.connectionID) return false;

    const id = requestPrefixes[operation] + newRequestID();
    const timeout = timeouts[operation];
    pending[operation] = {
      id,
      scope: { ...current },
      deadline: Date.now() + timeout,
      timer: setTimeout(() => {
        syncScope();
        if (pending[operation]?.id === id) expire(operation);
      }, timeout),
    };
    if (operation === "mcp_list") setListStatus("loading");
    else update({ reloading: true, reloadMessage: "" });

    if (sendEnvelope(operation, { request_id: id, session_id: "", payload: {} })) return true;
    cancel(operation);
    if (operation === "mcp_list") setListStatus("error");
    else update({ reloading: false, reloadMessage: "MCP 重载请求未发出。请在连接恢复后重试。" });
    return false;
  }

  function handleMessage(message: RelayMessage) {
    const error = message.type === "error" ? message.payload as ErrorPayload | undefined : undefined;
    const requestID = typeof message.request_id === "string" ? message.request_id : "";
    const ownedID = requestID.startsWith(requestPrefixes.mcp_list) ? "mcp_list"
      : requestID.startsWith(requestPrefixes.mcp_reload) ? "mcp_reload" : undefined;
    const operation = message.type === "mcp_list_result" ? "mcp_list"
      : message.type === "mcp_reload_result" ? "mcp_reload"
      : message.type === "error" ? ownedID ?? (isOperation(error?.request_type) ? error.request_type : undefined)
      : undefined;
    if (!operation) return false;

    const current = syncScope();
    const request = pending[operation];
    if (!request || request.id !== requestID || !sameScope(request.scope, current) ||
        (error?.request_type && error.request_type !== operation)) return true;
    if (Date.now() >= request.deadline) {
      expire(operation);
      return true;
    }
    cancel(operation);

    if (message.type === "error") {
      if (operation === "mcp_list") {
        // Older relays reject the unknown request. Older Agents may silently ignore it.
        const unsupported = typeof error?.message === "string" &&
          /^(?:unsupported|unknown) (?:client )?message type:\s*mcp_list\s*$/i.test(error.message);
        setListStatus(unsupported ? "unsupported" : "error");
      } else {
        update({ reloading: false, reloadMessage: reloadFailed });
      }
    } else if (operation === "mcp_list") {
      const servers = (message.payload as MCPListResultPayload | undefined)?.servers;
      if (Array.isArray(servers) && servers.every(isServerSummary)) {
        update({ inventory: { servers, status: "ready" } });
      } else {
        setListStatus("error");
      }
    } else {
      const reloaded = (message.payload as MCPReloadResultPayload | undefined)?.reloaded === true;
      update({ reloading: false, reloadMessage: reloaded ? "MCP 重载成功。" : reloadFailed });
      if (reloaded) {
        // Replace any earlier inventory request so only a post-reload snapshot can be applied.
        cancel("mcp_list");
        start("mcp_list");
      }
    }
    return true;
  }

  return {
    clear,
    handleDisconnect: syncScope,
    handleMessage,
    refresh: () => start("mcp_list"),
    reload: () => start("mcp_reload"),
  };
}

export function useMCPState({ relayScopeRef, sendEnvelope }: {
  relayScopeRef: RefObject<RelayScope>;
  sendEnvelope: SendEnvelope;
}) {
  const [state, setState] = useState(initialState);
  const requests = useMemo(() => createMCPRequests({
    getScope: () => relayScopeRef.current,
    sendEnvelope,
    onChange: setState,
  }), [relayScopeRef, sendEnvelope]);
  useEffect(() => requests.clear, [requests]);
  return { ...state, ...requests };
}
