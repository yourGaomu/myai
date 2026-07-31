import { useCallback, useRef, useState } from "react";

import type {
  AgentRun,
  AgentRunCompletedPayload,
  AgentRunEvent,
  AgentRunEventPayload,
  AgentRunListResultPayload,
  AgentRunSnapshot,
  AgentRunStartedPayload,
} from "../protocol";

type RunsBySession = Record<string, AgentRunSnapshot[]>;

export function useAgentRunState() {
  const runsRef = useRef<RunsBySession>({});
  const [version, setVersion] = useState(0);

  const updateSession = useCallback(
    (sessionID: string, update: (runs: AgentRunSnapshot[]) => AgentRunSnapshot[]) => {
      const key = sessionID.trim();
      if (!key) {
        return;
      }
      runsRef.current = {
        ...runsRef.current,
        [key]: sortSnapshots(update(runsRef.current[key] || [])),
      };
      setVersion((value) => value + 1);
    },
    [],
  );

  const applyRunStarted = useCallback(
    (payload?: AgentRunStartedPayload) => {
      const run = payload?.run;
      if (!run?.id || !run.session_id) {
        return;
      }
      updateSession(run.session_id, (runs) => upsertRun(runs, run));
    },
    [updateSession],
  );

  const applyRunEvent = useCallback(
    (payload?: AgentRunEventPayload) => {
      const event = payload?.event;
      if (!event?.id || !event.run_id || !event.session_id) {
        return;
      }
      updateSession(event.session_id, (runs) => upsertEvent(runs, event));
    },
    [updateSession],
  );

  const applyRunCompleted = useCallback(
    (payload?: AgentRunCompletedPayload) => {
      const run = payload?.run;
      if (!run?.id || !run.session_id) {
        return;
      }
      updateSession(run.session_id, (runs) => upsertRun(runs, run));
    },
    [updateSession],
  );

  const applyRunList = useCallback(
    (payload?: AgentRunListResultPayload) => {
      const sessionID = payload?.session_id?.trim();
      if (!sessionID) {
        return;
      }
      updateSession(sessionID, (current) => mergeRunList(payload?.runs || [], current));
    },
    [updateSession],
  );

  const getSessionRuns = useCallback((sessionID: string) => {
    return runsRef.current[sessionID.trim()] || [];
  }, []);

  const hasRunForRequest = useCallback((sessionID: string, requestID?: string) => {
    if (!requestID) {
      return false;
    }
    return (runsRef.current[sessionID.trim()] || []).some((snapshot) => snapshot.run.request_id === requestID);
  }, []);

  const mergeSessionRuns = useCallback((fromSessionID: string, toSessionID: string) => {
    const from = fromSessionID.trim();
    const to = toSessionID.trim();
    if (!from || !to || from === to || !runsRef.current[from]) {
      return;
    }
    runsRef.current = {
      ...runsRef.current,
      [to]: mergeRunList(runsRef.current[to] || [], runsRef.current[from]),
    };
    delete runsRef.current[from];
    setVersion((value) => value + 1);
  }, []);

  return {
    applyRunCompleted,
    applyRunEvent,
    applyRunList,
    applyRunStarted,
    getSessionRuns,
    hasRunForRequest,
    mergeSessionRuns,
    version,
  };
}

function upsertRun(snapshots: AgentRunSnapshot[], run: AgentRun) {
  const index = snapshots.findIndex((snapshot) => snapshot.run.id === run.id);
  if (index < 0) {
    return [...snapshots, { run, events: [] }];
  }
  return snapshots.map((snapshot, currentIndex) =>
    currentIndex === index ? { ...snapshot, run: { ...snapshot.run, ...run } } : snapshot,
  );
}

function upsertEvent(snapshots: AgentRunSnapshot[], event: AgentRunEvent) {
  const runIndex = snapshots.findIndex((snapshot) => snapshot.run.id === event.run_id);
  const base = runIndex >= 0
    ? snapshots[runIndex]
    : { run: placeholderRun(event), events: [] };
  const eventIndex = base.events.findIndex((item) => item.id === event.id);
  let events: AgentRunEvent[];
  if (eventIndex < 0) {
    events = [...base.events, { ...event, delta: false }];
  } else {
    events = base.events.map((item, index) => {
      if (index !== eventIndex) {
        return item;
      }
      return event.delta
        ? { ...item, content: `${item.content || ""}${event.content || ""}`, truncated: item.truncated || event.truncated }
        : { ...item, ...event, delta: false };
    });
  }
  events.sort((left, right) => left.sequence - right.sequence);
  const next = { ...base, run: runWithEventProgress(base.run, event), events };
  if (runIndex < 0) {
    return [...snapshots, next];
  }
  return snapshots.map((snapshot, index) => (index === runIndex ? next : snapshot));
}

function placeholderRun(event: AgentRunEvent): AgentRun {
  return {
    id: event.run_id,
    session_id: event.session_id,
    kind: "chat",
    status: "running",
    started_at: event.created_at,
  };
}

function runWithEventProgress(run: AgentRun, event: AgentRunEvent): AgentRun {
  if (!event.current_step && !event.total_steps) {
    return run;
  }
  return {
    ...run,
    current_step: event.current_step || run.current_step,
    total_steps: event.total_steps || run.total_steps,
  };
}

function mergeRunList(incoming: AgentRunSnapshot[], current: AgentRunSnapshot[]) {
  const byID = new Map<string, AgentRunSnapshot>();
  incoming.forEach((snapshot) => byID.set(snapshot.run.id, normalizeSnapshot(snapshot)));
  current.forEach((snapshot) => {
    const stored = byID.get(snapshot.run.id);
    if (!stored) {
      if (snapshot.run.status === "running") {
        byID.set(snapshot.run.id, snapshot);
      }
      return;
    }
    const events = [...stored.events];
    snapshot.events.forEach((event) => {
      const eventIndex = events.findIndex((item) => item.id === event.id);
      if (eventIndex < 0) {
        events.push(event);
      } else if ((event.content || "").length > (events[eventIndex].content || "").length) {
        events[eventIndex] = { ...events[eventIndex], ...event, delta: false };
      }
    });
    byID.set(snapshot.run.id, {
      run: stored.run.status === "running" ? { ...stored.run, ...snapshot.run } : stored.run,
      events: events.sort((left, right) => left.sequence - right.sequence),
    });
  });
  return [...byID.values()];
}

function normalizeSnapshot(snapshot: AgentRunSnapshot): AgentRunSnapshot {
  return {
    run: snapshot.run,
    events: [...(snapshot.events || [])]
      .map((event) => ({ ...event, delta: false }))
      .sort((left, right) => left.sequence - right.sequence),
  };
}

function sortSnapshots(snapshots: AgentRunSnapshot[]) {
  return [...snapshots].sort(
    (left, right) => Date.parse(left.run.started_at) - Date.parse(right.run.started_at),
  );
}
