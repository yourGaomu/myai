import { useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type RefObject, type SetStateAction } from "react";

import type { RelayMessage, SessionSummary } from "../protocol";
import type { RelayScope } from "../types/app";
import { applySessionRename } from "../utils/session";
import { createSessionRenameTracker, type SessionRenameFeedback } from "../utils/sessionRename";

export type RenameEntry = "list" | "sheet";
export type SessionRenameDraft = {
  entry: RenameEntry;
  sessionID: string;
  input: string;
  feedback?: SessionRenameFeedback;
};

type Args = {
  getScope: () => RelayScope;
  refreshSessions: () => void;
  sendEnvelope: (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
  sessions: SessionSummary[];
  setSessions: Dispatch<SetStateAction<SessionSummary[]>>;
};

export function useSessionRename({ getScope, refreshSessions, sendEnvelope, sessions, setSessions }: Args) {
  const sessionsRef: RefObject<SessionSummary[]> = useRef(sessions);
  sessionsRef.current = sessions;
  const [draft, setDraft] = useState<SessionRenameDraft | null>(null);
  const tracker = useMemo(() => createSessionRenameTracker({
    getScope,
    getSession: (sessionID) => sessionsRef.current.find((session) => session.id === sessionID),
    onFeedback: (feedback) => setDraft((current) => {
      if (!current || current.sessionID !== feedback.sessionID) return current;
      return feedback.status === "success" ? null : { ...current, feedback };
    }),
    onRenamed: (result) => setSessions((current) => applySessionRename(current, result)),
    refreshSessions,
  }), [getScope, refreshSessions, setSessions]);
  useEffect(() => tracker.clear, [tracker]);

  const open = useCallback((sessionID: string, entry: RenameEntry) => {
    const scope = getScope();
    if (!scope.pairingID || !scope.connectionID || !sessionID || sessionID !== sessionID.trim()) return;
    const session = sessionsRef.current.find((item) => item.id === sessionID && !item.deleted);
    if (!session) return;
    const feedback = tracker.getFeedback(sessionID);
    setDraft((current) => {
      if (current?.sessionID === sessionID) return { ...current, entry };
      return { entry, sessionID, input: feedback?.title ?? session.title ?? "", feedback };
    });
  }, [getScope, tracker]);

  const close = useCallback((entry?: RenameEntry) => {
    setDraft((current) => !entry || current?.entry === entry ? null : current);
  }, []);

  const edit = useCallback((input: string) => {
    setDraft((current) => {
      if (!current || current.feedback?.status === "pending" || current.feedback?.status === "unconfirmed") return current;
      return { ...current, input, feedback: current.feedback?.status === "error" ? undefined : current.feedback };
    });
  }, []);

  const save = useCallback(() => {
    if (draft) tracker.start(draft.sessionID, draft.input, sendEnvelope);
  }, [draft, sendEnvelope, tracker]);

  return { draft, open, close, edit, save, tracker };
}
