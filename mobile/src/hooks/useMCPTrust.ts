import { useEffect, useMemo, useState, type RefObject } from "react";

import type { ErrorPayload, MCPTrustPayload, MCPTrustResultPayload, MCPTrustService, RelayMessage } from "../protocol";
import type { RelayScope } from "../types/app";
import { newRequestID } from "../utils/ids";

const requestPrefix = "mcp-trust-";
const timeout = 15_000;
const denied = "mcp_trust_denied";
const reviewRequired = "mcp_trust_review_required";
const needsNewReview = "确认已失效或配置已变更，请重新查看并确认。";
export const mcpTrustOutcomeUnclear = "未能确认信任审批的结果。请以重新获取的信任状态为准；不会自动重试审批或重载 MCP。";

type Review = { id: string; service: MCPTrustService };
type Request = {
  id: string;
  payload: MCPTrustPayload;
  scope: RelayScope;
  deadline: number;
  timer: ReturnType<typeof setTimeout>;
  reviewedService?: MCPTrustService;
};
type TrustState = {
  // null means there is no positive capability in the current connection generation.
  services: MCPTrustService[] | null;
  pendingAction: MCPTrustPayload["action"] | null;
  review: Review | null;
  unclear: boolean;
  notice: string;
};
type SendEnvelope = (type: "mcp_trust", overrides: Partial<RelayMessage<MCPTrustPayload>>) => boolean;

export type MCPTrustControls = TrustState & {
  refresh: () => boolean;
  requestReview: (name: string) => boolean;
  cancelReview: () => void;
  confirm: (reviewID: string) => boolean;
};

const initialState: TrustState = { services: null, pendingAction: null, review: null, unclear: false, notice: "" };

function safeText(value: unknown, max: number): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= max &&
    value.trim() === value && !/[\p{Cc}\p{Cf}]/u.test(value);
}

function safeEndpoint(value: unknown): value is string {
  if (typeof value !== "string" || value.length > 512 || /[^\x21-\x7e]|[\\?#%]/.test(value)) return false;
  const address = /^https?:\/\/[^/@]+(?=\/|$)/.exec(value)?.[0];
  if (!address) return false;
  try {
    // Parse only the authority; React Native's URL polyfill can misread @ in the path.
    return !!new URL(address.toLowerCase()).hostname;
  } catch {
    return false;
  }
}

function readService(raw: unknown): MCPTrustService | undefined {
  const service = raw as MCPTrustService | undefined;
  if (!safeText(service?.name, 128) || !safeEndpoint(service.endpoint) ||
    (service.permission !== "read" && service.permission !== "write" && service.permission !== "execute") ||
    (service.authentication !== "none" && service.authentication !== "headers_configured") ||
    typeof service.trusted !== "boolean" ||
    (service.runtime !== "loaded" && service.runtime !== "older" && service.runtime !== "not_loaded")) return;
  const { name, endpoint, permission, authentication, trusted, runtime } = service;
  return { name, endpoint, permission, authentication, trusted, runtime };
}

function readServices(raw: unknown, allowOmitted = false) {
  if (allowOmitted && raw === undefined) return [];
  if (!Array.isArray(raw) || raw.length > 32) return;
  const services: MCPTrustService[] = [];
  const names = new Set<string>();
  for (const item of raw) {
    const service = readService(item);
    if (!service || names.has(service.name)) return;
    services.push(service);
    names.add(service.name);
  }
  return services;
}

function sameScope(a: RelayScope, b: RelayScope) {
  return a.pairingID === b.pairingID && a.connectionID === b.connectionID;
}

export function createMCPTrustRequests({ getScope, sendEnvelope, onChange }: {
  getScope: () => RelayScope;
  sendEnvelope: SendEnvelope;
  onChange: (state: TrustState) => void;
}) {
  let state = initialState;
  let scope = { ...getScope() };
  let active = false;
  let pending: Request | undefined;
  let recovering = false;
  let refreshAfterApprove = false;

  function update(patch: Partial<TrustState>) {
    state = { ...state, ...patch };
    onChange(state);
  }

  function clearRequest() {
    clearTimeout(pending?.timer);
    pending = undefined;
  }

  function syncScope() {
    const next = getScope();
    if (sameScope(scope, next)) return;
    const samePairing = scope.pairingID === next.pairingID;
    const interruptedApproval = pending?.payload.action === "approve";
    scope = { ...next };
    clearRequest();
    refreshAfterApprove = false;
    recovering = samePairing && (recovering || interruptedApproval);
    update({ ...initialState, unclear: samePairing && (state.unclear || interruptedApproval) });
    if (scope.connectionID && (active || recovering)) start({ action: "list" });
  }

  function refresh() {
    syncScope();
    if (!active || pending?.payload.action === "approve") return false;
    if (pending?.payload.action === "list") return true;
    clearRequest();
    return start({ action: "list" });
  }

  function setActive(next: boolean) {
    if (active === next) return;
    active = next;
    syncScope();
    if (active) refresh();
    else cancelReview();
  }

  function afterReload() {
    syncScope();
    if (pending?.payload.action === "approve") {
      refreshAfterApprove = true;
      return;
    }
    if (!active && !recovering && state.services === null) return;
    clearRequest();
    start({ action: "list" });
  }

  function unclearOutcome() {
    clearRequest();
    recovering = true;
    refreshAfterApprove = false;
    update({ services: null, pendingAction: null, review: null, unclear: true, notice: "" });
    // Only a new read is safe here. This path must never submit approval or reload MCP.
    if (scope.connectionID) start({ action: "list" });
  }

  function expire(request: Request) {
    clearRequest();
    if (request.payload.action === "approve") unclearOutcome();
    else failedRead(request.payload.action, "请求超时，请检查连接后刷新或重新查看。");
  }

  function failedRead(action: "list" | "review", notice: string, dropCapability = false) {
    update({
      ...(action === "list" || dropCapability ? { services: null } : {}),
      pendingAction: null, review: null, notice,
    });
  }

  function start(payload: MCPTrustPayload, reviewedService?: MCPTrustService) {
    if (!scope.connectionID || pending) return false;
    const id = requestPrefix + newRequestID();
    const request: Request = {
      id, payload, scope: { ...scope }, reviewedService, deadline: Date.now() + timeout,
      timer: setTimeout(() => {
        syncScope();
        if (pending === request) expire(request);
      }, timeout),
    };
    pending = request;
    update({ pendingAction: payload.action, review: null, notice: "", ...(payload.action === "approve" ? { unclear: false } : {}) });
    if (sendEnvelope("mcp_trust", { request_id: id, payload })) return true;
    syncScope();
    if (pending !== request) return false;
    clearRequest();
    if (payload.action === "approve") unclearOutcome();
    else failedRead(payload.action, "请求未能发出，请检查连接后重试。");
    return false;
  }

  function requestReview(name: string) {
    syncScope();
    if (!active || pending || state.review || !state.services?.some((service) => service.name === name && !service.trusted)) return false;
    return start({ action: "review", server_name: name });
  }

  function cancelReview() {
    if (pending?.payload.action === "review") {
      clearRequest();
      update({ pendingAction: null });
    }
    if (state.review) update({ review: null });
  }

  function confirm(reviewID: string) {
    syncScope();
    const review = state.review;
    if (!active || pending || !review || review.id !== reviewID || !state.services) return false;
    return start({ action: "approve", server_name: review.service.name, review_id: review.id }, review.service);
  }

  function updateService(service: MCPTrustService, notice: string, review: Review | null = null) {
    update({ services: state.services?.map((entry) => entry.name === service.name ? service : entry) ?? null,
      pendingAction: null, review, notice });
  }

  function handleMessage(message: RelayMessage) {
    const result = message.type === "mcp_trust_result" ? message.payload as MCPTrustResultPayload | undefined : undefined;
    const error = message.type === "error" ? message.payload as ErrorPayload | undefined : undefined;
    const id = message.request_id;
    if (message.type !== "mcp_trust_result" &&
        !(message.type === "error" && (error?.request_type === "mcp_trust" || (typeof id === "string" && id.startsWith(requestPrefix))))) return false;
    syncScope();
    const request = pending;
    if (!request || request.id !== id || !sameScope(request.scope, scope) ||
        (error?.request_type && error.request_type !== "mcp_trust")) return true;
    if (Date.now() >= request.deadline) {
      expire(request);
      return true;
    }
    const payload = request.payload;
    if (result && (result.action !== payload.action ||
        (result.server_name !== undefined && (payload.action === "list" || result.server_name !== payload.server_name)) ||
        (payload.action === "approve" && result.review_id !== undefined && result.review_id !== payload.review_id))) return true;
    const services = !result || result.code !== undefined ? undefined : readServices(result.services, payload.action === "list");
    const service = services?.length === 1 ? services[0] : undefined;
    if (service && payload.action !== "list" && service.name !== payload.server_name) return true;
    clearRequest();

    if (payload.action === "approve") {
      if (result?.code === denied) {
        refreshAfterApprove = false;
        update({ services: null, pendingAction: null, notice: "" });
        start({ action: "list" });
        return true;
      } else if (result?.code === reviewRequired) {
        update({ pendingAction: null, notice: needsNewReview });
      } else if (service?.trusted && request.reviewedService && service.endpoint === request.reviewedService.endpoint &&
        service.permission === request.reviewedService.permission && service.authentication === request.reviewedService.authentication) {
        updateService(service, "信任审批已保存。审批不会重载 MCP，请按需手动重载。");
      } else {
        unclearOutcome();
        return true;
      }
      if (refreshAfterApprove) {
        refreshAfterApprove = false;
        start({ action: "list" });
      }
    } else if (payload.action === "list") {
      if (services) {
        recovering = false;
        update({ services, pendingAction: null, notice: "" });
      } else failedRead("list", "无法获取 MCP 信任状态。请检查电脑端 Agent 后刷新。");
    } else if (service && safeText(result?.review_id, 64)) {
      if (service.trusted) updateService(service, "此服务已获得信任，无需再次审批。");
      else updateService(service, "", { id: result.review_id, service });
    } else if (result?.code === denied) failedRead("review", "", true);
    else failedRead("review", result?.code === reviewRequired ? needsNewReview : "无法查看此服务的审批信息。请刷新后重试。");
    return true;
  }

  return { afterReload, cancelReview, clear: clearRequest, confirm, handleMessage, refresh, requestReview, setActive, syncScope };
}

export function useMCPTrust({ relayScopeRef, sendEnvelope }: { relayScopeRef: RefObject<RelayScope>; sendEnvelope: SendEnvelope }) {
  const [state, setState] = useState(initialState);
  const requests = useMemo(() => createMCPTrustRequests({ getScope: () => relayScopeRef.current, sendEnvelope, onChange: setState }),
    [relayScopeRef, sendEnvelope]);
  useEffect(() => requests.clear, [requests]);
  return { ...state, ...requests };
}
