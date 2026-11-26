import { useCallback, useEffect, useRef, type RefObject } from "react";

import type { IntentTracePayload, RelayMessage, SessionHistoryDeltaResultPayload, SessionHistoryMessage, SessionHistoryMetaResultPayload, SessionHistoryResultPayload } from "../protocol";
import { loadCachedSessionHistory, replaceCachedSessionHistory } from "../storage/sessionHistoryCache";
import type { PendingAction, RelayScope } from "../types/app";
import type { ChatItem } from "../types/chat";
import { historyMessageToChatItem } from "../utils/chatHistory";
import { newRequestID } from "../utils/ids";

const pageSize = 100;
const timeoutMs = 15000;
const maximumDeltaPages = 100;

type HistoryPhase = "session_history" | "session_history_meta" | "session_history_delta";
type Sync = RelayScope & {
  cacheKey: string;
  requestID: string;
  phase?: HistoryPhase;
  messages: SessionHistoryMessage[];
  seen: Set<string>;
  minimumCount?: number;
  expectedLastID?: string;
  remainingPages: number;
  timeout?: ReturnType<typeof setTimeout>;
};

type Args = {
  historySessionIDRef: RefObject<string>;
  pendingHistorySessionIDRef: RefObject<string>;
  relayScopeRef: RefObject<RelayScope>;
  sessionIDRef: RefObject<string>;
  upsertIntentTrace: (sessionID: string, trace: IntentTracePayload, historical?: boolean) => void;
  mergeSessionHistory: (sessionID: string, messages: ChatItem[], source: "cache" | "server") => void;
  sendEnvelope: (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function useSessionHistorySync({
  historySessionIDRef, pendingHistorySessionIDRef, relayScopeRef, sessionIDRef,
  upsertIntentTrace, mergeSessionHistory, sendEnvelope, startPending, stopPending,
}: Args) {
  const syncsRef = useRef(new Map<string, Sync>());
  const cacheWritesRef = useRef(new Map<string, Promise<void>>());

  const clearHistoryRequests = useCallback(() => {
    for (const sync of syncsRef.current.values()) clearTimeout(sync.timeout);
    syncsRef.current.clear();
  }, []);
  useEffect(() => clearHistoryRequests, [clearHistoryRequests]);

  const resetHistorySync = useCallback(() => {
    clearHistoryRequests();
    historySessionIDRef.current = "";
    if (pendingHistorySessionIDRef.current) {
      pendingHistorySessionIDRef.current = "";
      stopPending("sessions");
    }
  }, [clearHistoryRequests, historySessionIDRef, pendingHistorySessionIDRef, stopPending]);

  const isCurrent = useCallback((sessionID: string, sync: Sync) => {
    const scope = relayScopeRef.current;
    return syncsRef.current.get(sessionID) === sync && scope.pairingID === sync.pairingID && scope.connectionID === sync.connectionID;
  }, [relayScopeRef]);

  const finish = useCallback((sessionID: string, sync: Sync, complete: boolean) => {
    if (!isCurrent(sessionID, sync)) return;
    clearTimeout(sync.timeout);
    syncsRef.current.delete(sessionID);
    if (sessionIDRef.current !== sessionID) return;
    if (complete) historySessionIDRef.current = sessionID;
    if (pendingHistorySessionIDRef.current === sessionID) {
      pendingHistorySessionIDRef.current = "";
      stopPending("sessions");
    }
  }, [historySessionIDRef, isCurrent, pendingHistorySessionIDRef, sessionIDRef, stopPending]);

  const armTimeout = useCallback((sessionID: string, sync: Sync) => {
    clearTimeout(sync.timeout);
    sync.timeout = setTimeout(() => finish(sessionID, sync, false), timeoutMs);
  }, [finish]);

  const begin = useCallback((sessionID: string) => {
    const scope = relayScopeRef.current;
    if (!scope.pairingID || !scope.connectionID || !sessionID) return null;
    const syncs = syncsRef.current;
    for (const [id, previous] of syncs) {
      if (id === sessionID || !isCurrent(id, previous)) {
        clearTimeout(previous.timeout);
        syncs.delete(id);
      }
    }
    const sync: Sync = {
      ...scope, requestID: "", messages: [], seen: new Set(), remainingPages: maximumDeltaPages,
      cacheKey: JSON.stringify([scope.pairingID, sessionID]),
    };
    syncs.set(sessionID, sync);
    if (sessionIDRef.current === sessionID) {
      historySessionIDRef.current = "";
      pendingHistorySessionIDRef.current = sessionID;
      startPending("sessions");
    }
    armTimeout(sessionID, sync);
    return sync;
  }, [armTimeout, historySessionIDRef, isCurrent, pendingHistorySessionIDRef, relayScopeRef, sessionIDRef, startPending]);

  const send = useCallback((sessionID: string, sync: Sync, phase: HistoryPhase, payload: object = {}) => {
    if (!isCurrent(sessionID, sync)) return false;
    sync.requestID = newRequestID();
    sync.phase = phase;
    armTimeout(sessionID, sync);
    const sent = sendEnvelope(phase, { request_id: sync.requestID, session_id: sessionID, payload: { ...payload, session_id: sessionID, ...(phase !== "session_history_delta" ? { include_intent_traces: true } : {}) } });
    if (!sent) finish(sessionID, sync, false);
    return sent;
  }, [armTimeout, finish, isCurrent, sendEnvelope]);

  const requestSessionHistoryFull = useCallback((nextSessionID = sessionIDRef.current) => {
    const sessionID = nextSessionID.trim();
    const sync = begin(sessionID);
    return sync ? send(sessionID, sync, "session_history") : false;
  }, [begin, send, sessionIDRef]);

  const requestHistoryWithCache = useCallback((sessionID: string) => {
    const sync = begin(sessionID);
    if (!sync) return false;
    void (async () => {
      try {
        await cacheWritesRef.current.get(sync.cacheKey)?.catch(() => {});
        if (!isCurrent(sessionID, sync)) return;
        const { messages, meta } = await loadCachedSessionHistory(sync.cacheKey);
        if (!isCurrent(sessionID, sync)) return;
        if (messages.length && meta.local_last_message_id) {
          sync.messages = messages;
          sync.seen = new Set(messages.map((item) => item.id));
          mergeSessionHistory(sessionID, messages.map(historyMessageToChatItem), "cache");
          send(sessionID, sync, "session_history_meta", meta);
        } else {
          send(sessionID, sync, "session_history");
        }
      } catch {
        send(sessionID, sync, "session_history");
      }
    })();
    return true;
  }, [begin, isCurrent, mergeSessionHistory, send]);

  const commit = useCallback((sessionID: string, sync: Sync, messages: SessionHistoryMessage[], write: boolean) => {
    mergeSessionHistory(sessionID, messages.map(historyMessageToChatItem), "server");
    if (write) {
      // Serialize complete snapshots so a native transaction cannot finish behind a newer write.
      const previous = cacheWritesRef.current.get(sync.cacheKey) || Promise.resolve();
      const next = previous.catch(() => {}).then(() => replaceCachedSessionHistory(sync.cacheKey, messages));
      cacheWritesRef.current.set(sync.cacheKey, next);
      void next.catch(() => {}).then(() => {
        if (cacheWritesRef.current.get(sync.cacheKey) === next) cacheWritesRef.current.delete(sync.cacheKey);
      });
    }
    finish(sessionID, sync, true);
  }, [finish, mergeSessionHistory]);

  const handleSessionHistoryMessage = useCallback((message: RelayMessage) => {
    const payload = message.payload as SessionHistoryResultPayload | SessionHistoryMetaResultPayload | SessionHistoryDeltaResultPayload | undefined;
    const sessionID = (payload?.session_id || message.session_id || "").trim();
    const sync = sessionID ? syncsRef.current.get(sessionID) : undefined;
    if (!sessionID || !sync || !message.request_id || !isCurrent(sessionID, sync) || sync.requestID !== message.request_id) return false;
    if (message.type === "error") {
      finish(sessionID, sync, false);
      return true;
    }
    if (!payload || message.type !== sync.phase + "_result" || (message.session_id && message.session_id !== sessionID)) return false;
    if (message.type === "session_history_result") {
      const result = payload as SessionHistoryResultPayload;
      const messages = result.messages || [];
      if (result.count !== undefined && result.count !== messages.length) {
        finish(sessionID, sync, false);
      } else {
        commit(sessionID, sync, messages, true);
      }
    } else if (message.type === "session_history_meta_result") {
      const meta = payload as SessionHistoryMetaResultPayload;
      if (meta.up_to_date) {
        commit(sessionID, sync, sync.messages, false);
      } else if (meta.can_delta && sync.messages.length > 0) {
        sync.minimumCount = meta.message_count;
        sync.expectedLastID = meta.last_message_id;
        send(sessionID, sync, "session_history_delta", { after_message_id: sync.messages[sync.messages.length - 1].id, limit: pageSize });
      } else {
        send(sessionID, sync, "session_history");
      }
    } else if (message.type === "session_history_delta_result") {
      const delta = payload as SessionHistoryDeltaResultPayload;
      const page = delta.messages || [];
      if (delta.full_sync_required || --sync.remainingPages < 0 ||
          (delta.count !== undefined && delta.count !== page.length) || page.some((item) => {
        if (!item.id || sync.seen.has(item.id)) return true;
        sync.seen.add(item.id);
        return false;
      })) {
        send(sessionID, sync, "session_history");
        return true;
      }
      sync.messages.push(...page);
      if (page.length >= pageSize) {
        // No has_more on this wire shape. A full page can never finish sync.
        send(sessionID, sync, "session_history_delta", { after_message_id: page[page.length - 1].id, limit: pageSize });
      } else if (sync.messages.length < (sync.minimumCount ?? 0) ||
          (sync.expectedLastID && !sync.seen.has(sync.expectedLastID))) {
        send(sessionID, sync, "session_history");
      } else {
        commit(sessionID, sync, sync.messages, true);
      }
    }
    if ("intent_traces" in payload) {
      for (const trace of [...(payload.intent_traces || [])].reverse()) {
        if (trace.session_id === sessionID) upsertIntentTrace(sessionID, trace, true);
      }
    }
    return true;
  }, [commit, finish, isCurrent, send, upsertIntentTrace]);

  return { requestHistoryWithCache, requestSessionHistoryFull, handleSessionHistoryMessage, resetHistorySync };
}
