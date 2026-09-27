import { useCallback, useEffect, useRef, useState } from "react";

import type {
  IntentConfigPayload,
  IntentConfigResultPayload,
  IntentConfigTestResultPayload,
  IntentStrategy,
  IntentTraceClearResultPayload,
  IntentTraceGetResultPayload,
  IntentTraceListResultPayload,
  IntentTracePayload,
  RelayMessage,
} from "../protocol";
import type { PendingAction } from "../types/app";
import { newRequestID, shortID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type IntentRequestKind =
  | "config_query"
  | "config_set"
  | "config_test"
  | "trace_list"
  | "trace_get"
  | "trace_clear";

type PendingIntentRequest = {
  kind: IntentRequestKind;
  traceID?: string;
  timer: ReturnType<typeof setTimeout>;
};

const intentRequestTimeoutMs = 8000;

export const defaultIntentConfig: IntentConfigResultPayload = {
  strategy: "system",
  base_url: "https://api.typesafe.ai",
  has_api_key: false,
  model: "jev-latest",
  plan_confidence: 0.55,
  execute_confidence: 0.85,
};

type Args = {
  clientToken: string;
  sendEnvelope: SendEnvelope;
  sessionID: string;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function normalizeIntentStrategy(value?: string): IntentStrategy {
  if (value === "jev" || value === "off") {
    return value;
  }
  return "system";
}

function normalizeIntentConfigResult(payload?: IntentConfigResultPayload | null): IntentConfigResultPayload {
  return {
    strategy: normalizeIntentStrategy(payload?.strategy),
    base_url: payload?.base_url?.trim() || defaultIntentConfig.base_url,
    has_api_key: Boolean(payload?.has_api_key),
    model: payload?.model?.trim() || defaultIntentConfig.model,
    plan_confidence:
      typeof payload?.plan_confidence === "number" && payload.plan_confidence > 0
        ? payload.plan_confidence
        : defaultIntentConfig.plan_confidence,
    execute_confidence:
      typeof payload?.execute_confidence === "number" && payload.execute_confidence > 0
        ? payload.execute_confidence
        : defaultIntentConfig.execute_confidence,
  };
}

export function useIntentSettings({
  clientToken,
  sendEnvelope,
  sessionID,
  startPending,
  stopPending,
}: Args) {
  const [intentConfig, setIntentConfig] = useState<IntentConfigResultPayload | null>(null);
  const [intentConfigLoading, setIntentConfigLoading] = useState(false);
  const [intentConfigSaving, setIntentConfigSaving] = useState(false);
  const [intentConfigTesting, setIntentConfigTesting] = useState(false);
  const [intentConfigMessage, setIntentConfigMessage] = useState("");
  const [intentConfigError, setIntentConfigError] = useState(false);
  const [intentTestResult, setIntentTestResult] = useState<IntentConfigTestResultPayload | null>(null);

  const [intentTraces, setIntentTraces] = useState<IntentTracePayload[]>([]);
  const [intentTraceDetails, setIntentTraceDetails] = useState<Record<string, IntentTracePayload>>({});
  const [intentTraceDetailErrors, setIntentTraceDetailErrors] = useState<Record<string, string>>({});
  const [intentTraceListLoading, setIntentTraceListLoading] = useState(false);
  const [intentTraceClearing, setIntentTraceClearing] = useState(false);
  const [intentTraceDetailLoadingID, setIntentTraceDetailLoadingID] = useState("");
  const [intentTraceMessage, setIntentTraceMessage] = useState("");
  const [intentTraceError, setIntentTraceError] = useState(false);

  const pendingRequestsRef = useRef<Record<string, PendingIntentRequest>>({});

  const syncGlobalPending = useCallback(() => {
    const hasRemaining = Object.keys(pendingRequestsRef.current).length > 0;
    if (!hasRemaining) {
      stopPending("intent");
    }
  }, [stopPending]);

  const finishRequestsByPredicate = useCallback(
    (predicate: (requestID: string, entry: PendingIntentRequest) => boolean): PendingIntentRequest[] => {
      const matched: PendingIntentRequest[] = [];
      for (const [reqID, entry] of Object.entries(pendingRequestsRef.current)) {
        if (predicate(reqID, entry)) {
          clearTimeout(entry.timer);
          delete pendingRequestsRef.current[reqID];
          matched.push(entry);
        }
      }
      syncGlobalPending();
      return matched;
    },
    [syncGlobalPending],
  );

  useEffect(() => {
    return () => {
      for (const entry of Object.values(pendingRequestsRef.current)) {
        clearTimeout(entry.timer);
      }
      pendingRequestsRef.current = {};
    };
  }, []);

  const registerPendingRequest = useCallback(
    (requestID: string, kind: IntentRequestKind, traceID?: string) => {
      startPending("intent");
      const timer = setTimeout(() => {
        const entry = pendingRequestsRef.current[requestID];
        if (!entry) {
          return;
        }
        delete pendingRequestsRef.current[requestID];
        syncGlobalPending();

        switch (entry.kind) {
          case "config_query":
            setIntentConfigLoading(false);
            setIntentConfigError(true);
            setIntentConfigMessage("读取自动规划配置超时，请检查连接后重试。");
            break;
          case "config_set":
            setIntentConfigSaving(false);
            setIntentConfigError(true);
            setIntentConfigMessage("保存自动规划配置超时，请检查连接后重试。");
            break;
          case "config_test":
            setIntentConfigTesting(false);
            setIntentConfigError(true);
            setIntentConfigMessage("测试 Jev 连接超时，请检查服务地址与网络连接。");
            break;
          case "trace_list":
            setIntentTraceListLoading(false);
            setIntentTraceError(true);
            setIntentTraceMessage("读取判断记录列表超时，请重试。");
            break;
          case "trace_get":
            setIntentTraceDetailLoadingID((current) => (current === (entry.traceID || "") ? "" : current));
            if (entry.traceID) {
              setIntentTraceDetailErrors((current) => ({
                ...current,
                [entry.traceID as string]: "读取判断详情超时，请点击重试。",
              }));
            }
            break;
          case "trace_clear":
            setIntentTraceClearing(false);
            setIntentTraceError(true);
            setIntentTraceMessage("清理判断记录超时，请重试。");
            break;
        }
      }, intentRequestTimeoutMs);

      pendingRequestsRef.current[requestID] = { kind, traceID, timer };
    },
    [startPending, syncGlobalPending],
  );

  const clearIntentConfigFeedback = useCallback(() => {
    setIntentConfigMessage("");
    setIntentConfigError(false);
  }, []);

  const clearIntentTraceFeedback = useCallback(() => {
    setIntentTraceMessage("");
    setIntentTraceError(false);
  }, []);

  const requestIntentConfig = useCallback(() => {
    if (!clientToken) {
      setIntentConfigLoading(false);
      stopPending("intent");
      return false;
    }
    const requestID = newRequestID();
    setIntentConfigLoading(true);
    registerPendingRequest(requestID, "config_query");
    if (!sendEnvelope("intent_config_query", { request_id: requestID, payload: {} })) {
      finishRequestsByPredicate((id) => id === requestID);
      setIntentConfigLoading(false);
      return false;
    }
    return true;
  }, [clientToken, finishRequestsByPredicate, registerPendingRequest, sendEnvelope, stopPending]);

  const saveIntentConfig = useCallback(
    (payload: IntentConfigPayload) => {
      if (!clientToken) {
        setIntentConfigError(true);
        setIntentConfigMessage("请先完成配对并连接 Agent。");
        return false;
      }
      // 严禁在日志或本地持久化存储中输出 api_key
      const sanitizedPayload: IntentConfigPayload = {
        strategy: payload.strategy,
        base_url: payload.base_url.trim(),
        model: payload.model.trim(),
        plan_confidence: payload.plan_confidence,
        execute_confidence: payload.execute_confidence,
      };
      if (payload.clear_api_key) {
        sanitizedPayload.clear_api_key = true;
      } else if (payload.api_key && payload.api_key.trim()) {
        sanitizedPayload.api_key = payload.api_key.trim();
      }

      const requestID = newRequestID();
      setIntentConfigSaving(true);
      setIntentConfigError(false);
      setIntentConfigMessage("");
      registerPendingRequest(requestID, "config_set");
      if (!sendEnvelope("intent_config_set", { request_id: requestID, payload: sanitizedPayload })) {
        finishRequestsByPredicate((id) => id === requestID);
        setIntentConfigSaving(false);
        setIntentConfigError(true);
        setIntentConfigMessage("请求未发送，请检查 Relay 连接。");
        return false;
      }
      return true;
    },
    [clientToken, finishRequestsByPredicate, registerPendingRequest, sendEnvelope],
  );

  const testIntentConfig = useCallback(
    (payload: IntentConfigPayload) => {
      if (!clientToken) {
        setIntentConfigError(true);
        setIntentConfigMessage("请先完成配对并连接 Agent。");
        return false;
      }
      const sanitizedPayload: IntentConfigPayload = {
        strategy: payload.strategy,
        base_url: payload.base_url.trim(),
        model: payload.model.trim(),
        plan_confidence: payload.plan_confidence,
        execute_confidence: payload.execute_confidence,
      };
      if (payload.api_key && payload.api_key.trim()) {
        sanitizedPayload.api_key = payload.api_key.trim();
      }

      const requestID = newRequestID();
      setIntentConfigTesting(true);
      setIntentTestResult(null);
      setIntentConfigError(false);
      setIntentConfigMessage("");
      registerPendingRequest(requestID, "config_test");
      if (!sendEnvelope("intent_config_test", { request_id: requestID, payload: sanitizedPayload })) {
        finishRequestsByPredicate((id) => id === requestID);
        setIntentConfigTesting(false);
        setIntentConfigError(true);
        setIntentConfigMessage("测试请求未发送，请检查 Relay 连接。");
        return false;
      }
      return true;
    },
    [clientToken, finishRequestsByPredicate, registerPendingRequest, sendEnvelope],
  );

  const requestIntentTraces = useCallback(
    (filterSessionID?: string, limit = 50) => {
      if (!clientToken) {
        setIntentTraceListLoading(false);
        stopPending("intent");
        return false;
      }
      const normalizedSessionID = (filterSessionID ?? "").trim();
      const requestID = newRequestID();
      setIntentTraceListLoading(true);
      setIntentTraceError(false);
      registerPendingRequest(requestID, "trace_list");
      const payload: { session_id?: string; limit: number } = { limit };
      if (normalizedSessionID) {
        payload.session_id = normalizedSessionID;
      }
      if (
        !sendEnvelope("intent_trace_list", {
          request_id: requestID,
          session_id: normalizedSessionID || sessionID.trim(),
          payload,
        })
      ) {
        finishRequestsByPredicate((id) => id === requestID);
        setIntentTraceListLoading(false);
        setIntentTraceError(true);
        setIntentTraceMessage("无法读取判断记录，请检查连接状态。");
        return false;
      }
      return true;
    },
    [clientToken, finishRequestsByPredicate, registerPendingRequest, sendEnvelope, sessionID, stopPending],
  );

  const requestIntentTraceDetail = useCallback(
    (traceID: string) => {
      const normalizedTraceID = traceID.trim();
      if (!normalizedTraceID) {
        return false;
      }
      if (!clientToken) {
        setIntentTraceDetailErrors((current) => ({
          ...current,
          [normalizedTraceID]: "请先完成配对并连接 Agent。",
        }));
        return false;
      }
      const requestID = newRequestID();
      setIntentTraceDetailLoadingID(normalizedTraceID);
      setIntentTraceDetailErrors((current) => {
        const next = { ...current };
        delete next[normalizedTraceID];
        return next;
      });
      registerPendingRequest(requestID, "trace_get", normalizedTraceID);
      if (
        !sendEnvelope("intent_trace_get", {
          request_id: requestID,
          payload: { trace_id: normalizedTraceID },
        })
      ) {
        finishRequestsByPredicate((id) => id === requestID);
        setIntentTraceDetailLoadingID((current) => (current === normalizedTraceID ? "" : current));
        setIntentTraceDetailErrors((current) => ({
          ...current,
          [normalizedTraceID]: "详情请求未发送，请检查连接。",
        }));
        return false;
      }
      return true;
    },
    [clientToken, finishRequestsByPredicate, registerPendingRequest, sendEnvelope],
  );

  const clearIntentTraces = useCallback(
    (filterSessionID?: string) => {
      if (!clientToken) {
        setIntentTraceError(true);
        setIntentTraceMessage("请先完成配对并连接 Agent。");
        return false;
      }
      const normalizedSessionID = (filterSessionID ?? "").trim();
      const requestID = newRequestID();
      setIntentTraceClearing(true);
      setIntentTraceError(false);
      setIntentTraceMessage("");
      registerPendingRequest(requestID, "trace_clear");
      const payload: { session_id?: string } = {};
      if (normalizedSessionID) {
        payload.session_id = normalizedSessionID;
      }
      if (
        !sendEnvelope("intent_trace_clear", {
          request_id: requestID,
          session_id: normalizedSessionID || sessionID.trim(),
          payload,
        })
      ) {
        finishRequestsByPredicate((id) => id === requestID);
        setIntentTraceClearing(false);
        setIntentTraceError(true);
        setIntentTraceMessage("清理请求未发送，请检查连接。");
        return false;
      }
      return true;
    },
    [clientToken, finishRequestsByPredicate, registerPendingRequest, sendEnvelope, sessionID],
  );

  const applyIntentConfigQuery = useCallback(
    (payload?: IntentConfigResultPayload, requestID?: string) => {
      finishRequestsByPredicate((id, entry) =>
        requestID ? id === requestID : entry.kind === "config_query",
      );
      setIntentConfigLoading(false);
      setIntentConfig(normalizeIntentConfigResult(payload));
    },
    [finishRequestsByPredicate],
  );

  const applyIntentConfigSet = useCallback(
    (payload?: IntentConfigResultPayload, requestID?: string) => {
      finishRequestsByPredicate((id, entry) =>
        requestID ? id === requestID : entry.kind === "config_set",
      );
      setIntentConfigSaving(false);
      setIntentConfig(normalizeIntentConfigResult(payload));
      setIntentConfigError(false);
      setIntentConfigMessage("自动规划配置已保存。");
    },
    [finishRequestsByPredicate],
  );

  const applyIntentConfigTest = useCallback(
    (payload?: IntentConfigTestResultPayload, requestID?: string) => {
      finishRequestsByPredicate((id, entry) =>
        requestID ? id === requestID : entry.kind === "config_test",
      );
      setIntentConfigTesting(false);
      const result: IntentConfigTestResultPayload = {
        success: Boolean(payload?.success),
        latency_ms: typeof payload?.latency_ms === "number" ? payload.latency_ms : 0,
      };
      setIntentTestResult(result);
      setIntentConfigError(!result.success);
      setIntentConfigMessage(
        result.success ? `测试连接成功 · 延迟 ${result.latency_ms} ms` : "测试连接未通过。",
      );
    },
    [finishRequestsByPredicate],
  );

  const applyIntentTraceList = useCallback(
    (payload?: IntentTraceListResultPayload, requestID?: string) => {
      finishRequestsByPredicate((id, entry) =>
        requestID ? id === requestID : entry.kind === "trace_list",
      );
      setIntentTraceListLoading(false);
      setIntentTraces(payload?.traces || []);
      setIntentTraceError(false);
    },
    [finishRequestsByPredicate],
  );

  const applyIntentTraceGet = useCallback(
    (payload?: IntentTraceGetResultPayload, requestID?: string) => {
      const trace = payload?.trace;
      const traceID = trace?.trace_id?.trim() || "";
      finishRequestsByPredicate((id, entry) =>
        requestID
          ? id === requestID
          : entry.kind === "trace_get" && (!traceID || entry.traceID === traceID),
      );
      setIntentTraceDetailLoadingID((current) =>
        !traceID || current === traceID ? "" : current,
      );
      if (!trace || !traceID) {
        return;
      }
      setIntentTraceDetails((current) => ({ ...current, [traceID]: trace }));
      setIntentTraceDetailErrors((current) => {
        const next = { ...current };
        delete next[traceID];
        return next;
      });
      setIntentTraces((current) =>
        current.map((item) => (item.trace_id === traceID ? { ...item, ...trace } : item)),
      );
    },
    [finishRequestsByPredicate],
  );

  const applyIntentTraceClear = useCallback(
    (payload?: IntentTraceClearResultPayload, requestID?: string) => {
      finishRequestsByPredicate((id, entry) =>
        requestID ? id === requestID : entry.kind === "trace_clear",
      );
      setIntentTraceClearing(false);
      const clearedSessionID = payload?.session_id?.trim() || "";
      if (!clearedSessionID) {
        setIntentTraces([]);
        setIntentTraceDetails({});
        setIntentTraceDetailErrors({});
      } else {
        setIntentTraces((current) => {
          const remaining = current.filter((item) => item.session_id !== clearedSessionID);
          const remainingIDs = new Set(remaining.map((item) => item.trace_id));
          setIntentTraceDetails((details) => {
            const next: Record<string, IntentTracePayload> = {};
            for (const [id, detail] of Object.entries(details)) {
              if (remainingIDs.has(id)) {
                next[id] = detail;
              }
            }
            return next;
          });
          return remaining;
        });
      }
      setIntentTraceError(false);
      setIntentTraceMessage(
        clearedSessionID
          ? `已清理会话 #${shortID(clearedSessionID)} 的判断记录。`
          : "已清理全部判断记录。",
      );
    },
    [finishRequestsByPredicate],
  );

  const applyIntentError = useCallback(
    (message: string, requestID?: string): boolean => {
      const normalizedError = message.trim() || "自动规划操作失败";
      let matched = finishRequestsByPredicate((id) => Boolean(requestID && id === requestID));
      if (matched.length === 0 && Object.keys(pendingRequestsRef.current).length > 0 && !requestID) {
        matched = finishRequestsByPredicate(() => true);
      }
      if (matched.length === 0) {
        return false;
      }

      for (const entry of matched) {
        switch (entry.kind) {
          case "config_query":
            setIntentConfigLoading(false);
            setIntentConfigError(true);
            setIntentConfigMessage(normalizedError);
            break;
          case "config_set":
            setIntentConfigSaving(false);
            setIntentConfigError(true);
            setIntentConfigMessage(normalizedError);
            break;
          case "config_test":
            setIntentConfigTesting(false);
            setIntentTestResult({ success: false, latency_ms: 0 });
            setIntentConfigError(true);
            setIntentConfigMessage(normalizedError);
            break;
          case "trace_list":
            setIntentTraceListLoading(false);
            setIntentTraceError(true);
            setIntentTraceMessage(normalizedError);
            break;
          case "trace_get":
            setIntentTraceDetailLoadingID((current) =>
              current === (entry.traceID || "") ? "" : current,
            );
            if (entry.traceID) {
              setIntentTraceDetailErrors((current) => ({
                ...current,
                [entry.traceID as string]: normalizedError,
              }));
            } else {
              setIntentTraceError(true);
              setIntentTraceMessage(normalizedError);
            }
            break;
          case "trace_clear":
            setIntentTraceClearing(false);
            setIntentTraceError(true);
            setIntentTraceMessage(normalizedError);
            break;
        }
      }
      return true;
    },
    [finishRequestsByPredicate],
  );

  return {
    intentConfig,
    intentConfigLoading,
    intentConfigSaving,
    intentConfigTesting,
    intentConfigMessage,
    intentConfigError,
    intentTestResult,
    intentTraces,
    intentTraceDetails,
    intentTraceDetailErrors,
    intentTraceListLoading,
    intentTraceClearing,
    intentTraceDetailLoadingID,
    intentTraceMessage,
    intentTraceError,
    clearIntentConfigFeedback,
    clearIntentTraceFeedback,
    requestIntentConfig,
    saveIntentConfig,
    testIntentConfig,
    requestIntentTraces,
    requestIntentTraceDetail,
    clearIntentTraces,
    applyIntentConfigQuery,
    applyIntentConfigSet,
    applyIntentConfigTest,
    applyIntentTraceList,
    applyIntentTraceGet,
    applyIntentTraceClear,
    applyIntentError,
  };
}
