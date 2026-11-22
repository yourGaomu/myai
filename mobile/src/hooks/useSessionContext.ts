import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";

import type { ContextInfo, ErrorPayload, RelayMessage, SessionContextQueryResultPayload } from "../protocol";
import type { RelayScope } from "../types/app";
import { newRequestID } from "../utils/ids";
import { mergeSessionContext, type ContextUpdateSource } from "../utils/sessionContext";

const queryTimeoutMs = 15_000;

type ContextQuery = RelayScope & {
  sessionID: string;
  requestID: string;
  timeout: ReturnType<typeof setTimeout>;
  receivedLiveSample: boolean;
};

type Args = {
  relayScopeRef: RefObject<RelayScope>;
  sessionID: string;
  sessionIDRef: RefObject<string>;
  sendEnvelope: (type: RelayMessage["type"], overrides: Partial<RelayMessage>) => boolean;
};

export function useSessionContext({ relayScopeRef, sessionID, sessionIDRef, sendEnvelope }: Args) {
  const [sessionContexts, setSessionContexts] = useState<Record<string, ContextInfo>>({});
  const [pendingContextSessionID, setPendingContextSessionID] = useState("");
  const queryRef = useRef<ContextQuery | null>(null);

  const cancelQuery = useCallback(() => {
    clearTimeout(queryRef.current?.timeout);
    queryRef.current = null;
  }, []);
  useEffect(() => cancelQuery, [cancelQuery]);

  const resetContextQuery = useCallback(() => {
    cancelQuery();
    setPendingContextSessionID("");
  }, [cancelQuery]);

  useLayoutEffect(() => {
    if (queryRef.current && queryRef.current.sessionID !== sessionID) resetContextQuery();
  }, [resetContextQuery, sessionID]);

  const clearSessionContexts = useCallback(() => {
    resetContextQuery();
    setSessionContexts({});
  }, [resetContextQuery]);

  const removeSessionContext = useCallback((sessionID: string) => {
    if (queryRef.current?.sessionID === sessionID) resetContextQuery();
    setSessionContexts((current) => {
      if (!Object.hasOwn(current, sessionID)) return current;
      const next = { ...current };
      delete next[sessionID];
      return next;
    });
  }, [resetContextQuery]);

  const setSessionContext = useCallback((sessionID: string, context: ContextInfo, source: ContextUpdateSource = "snapshot") => {
    if (!sessionID) return;
    const query = queryRef.current;
    if (source === "live" && context.request_context && query?.sessionID === sessionID) {
      query.receivedLiveSample = true;
    }
    setSessionContexts((current) => ({
      ...current,
      [sessionID]: mergeSessionContext(current[sessionID], context, source),
    }));
  }, []);

  const requestContextInfo = useCallback(() => {
    const sessionID = sessionIDRef.current.trim();
    const scope = relayScopeRef.current;
    if (!sessionID || !scope.connectionID) return;

    // Opening the sheet while the header refresh is pending can reuse that read.
    const pending = queryRef.current;
    if (pending?.sessionID === sessionID && pending.pairingID === scope.pairingID &&
        pending.connectionID === scope.connectionID) return;

    cancelQuery();
    const requestID = newRequestID();
    const timeout = setTimeout(() => {
      if (queryRef.current?.requestID === requestID) resetContextQuery();
    }, queryTimeoutMs);
    queryRef.current = { ...scope, sessionID, requestID, timeout, receivedLiveSample: false };
    setPendingContextSessionID(sessionID);
    if (!sendEnvelope("session_context_query", {
      request_id: requestID,
      session_id: sessionID,
      payload: { session_id: sessionID },
    })) resetContextQuery();
  }, [cancelQuery, relayScopeRef, resetContextQuery, sendEnvelope, sessionIDRef]);

  const handleContextMessage = useCallback((message: RelayMessage) => {
    const reply = message.type === "session_context_query_result";
    const error = message.type === "error" ? message.payload as ErrorPayload | undefined : undefined;
    if (!reply && error?.request_type !== "session_context_query") return false;

    const query = queryRef.current;
    const scope = relayScopeRef.current;
    const payload = reply ? message.payload as SessionContextQueryResultPayload | undefined : undefined;
    const envelopeSession = message.session_id?.trim();
    const payloadSession = payload?.session_id?.trim();
    if (!query || message.request_id !== query.requestID || sessionIDRef.current !== query.sessionID ||
        scope.pairingID !== query.pairingID || scope.connectionID !== query.connectionID ||
        (reply && !envelopeSession && !payloadSession) ||
        (envelopeSession && envelopeSession !== query.sessionID) ||
        (payloadSession && payloadSession !== query.sessionID)) return true;

    resetContextQuery();
    if (payload?.context) {
      const { sessionID, receivedLiveSample } = query;
      const context = payload.context;
      setSessionContexts((current) => ({
        ...current,
        [sessionID]: mergeSessionContext(current[sessionID], context, "snapshot", receivedLiveSample),
      }));
    }
    return true;
  }, [relayScopeRef, resetContextQuery, sessionIDRef]);

  return {
    clearSessionContexts, handleContextMessage, pendingContextSessionID, removeSessionContext, requestContextInfo,
    resetContextQuery, sessionContexts, setSessionContext,
  };
}
