const assert = require("node:assert/strict");
const fs = require("node:fs");
const Module = require("node:module");
const { test } = require("node:test");
const ts = require("typescript");

require.extensions[".ts"] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    fileName: filename,
  });
  module._compile(outputText, filename);
};

const { applySessionRename, mergeSessionList, upsertSession } = require("../src/utils/session.ts");
const { createSessionRenameTracker, validateSessionTitle } = require("../src/utils/sessionRename.ts");
const load = Module._load;
Module._load = function (name, ...args) {
  if (name === "react") return { useCallback: (fn) => fn, useMemo: (fn) => fn(), useRef: (current) => ({ current }) };
  return load.call(this, name, ...args);
};
let useRemoteMessageHandler, useRemoteResultAppliers;
try {
  ({ useRemoteMessageHandler } = require("../src/hooks/useRemoteMessageHandler.ts"));
  ({ useRemoteResultAppliers } = require("../src/hooks/useRemoteResultAppliers.ts"));
} finally {
  Module._load = load;
}

const target = { id: "target", title: "Original" };
const other = { id: "other", title: "Selected" };
const noop = () => {};
const result = (request, revision = 1) => ({
  type: "session_rename_result",
  request_id: request.request_id,
  session_id: request.session_id,
  payload: { ...request.payload, title_revision: revision },
});

function harness(t, initial = [target, other]) {
  const scope = { current: { pairingID: "pair-a", connectionID: "connection-a" } };
  const sent = [];
  const feedback = new Map();
  const applied = [];
  let refreshes = 0;
  let sessions = initial;
  const tracker = createSessionRenameTracker({
    getScope: () => scope.current,
    getSession: (id) => sessions.find((session) => session.id === id),
    onFeedback: (item) => feedback.set(item.sessionID, item),
    onRenamed: (payload) => {
      applied.push(payload);
      sessions = applySessionRename(sessions, payload);
    },
    refreshSessions: () => { refreshes += 1; },
  });
  t.after(tracker.clear);
  return {
    tracker, scope, sent, feedback, applied,
    get refreshes() { return refreshes; },
    get sessions() { return sessions; },
    setSessions(update) { sessions = update(sessions); },
    start(id = target.id, title = "  项目 🧪　", send = () => true) {
      return tracker.start(id, title, (type, envelope) => {
        sent.push({ type, ...envelope });
        return send();
      });
    },
  };
}

test("validates Unicode code points and sends one explicit request per active session", (t) => {
  assert.deepEqual(validateSessionTitle("\u0085 中文 🧪　"), { title: "中文 🧪", length: 4, error: "" });
  assert.equal(validateSessionTitle("🧪".repeat(100)).error, "");
  assert.match(validateSessionTitle("🧪".repeat(101)).error, /100/);
  assert.match(validateSessionTitle(" \r\n　").error, /请输入/);
  const h = harness(t, [target, other, { id: "trash", deleted: true }]);
  for (const id of ["", "missing", " target ", "trash"]) assert.equal(h.start(id), false);
  assert.equal(h.start(target.id, "　"), false);
  assert.equal(h.sent.length, 0);
  assert.equal(h.start(), true);
  assert.equal(h.start(), false);
  assert.equal(h.start(other.id), true);
  assert.equal(h.sent.length, 2);
  const [request, simultaneous] = h.sent;
  assert.equal(request.type, "session_rename");
  assert.equal(request.session_id, target.id);
  assert.deepEqual(request.payload, { session_id: target.id, title: "项目 🧪" });
  assert.notEqual(request.request_id, simultaneous.request_id);
});

test("ignores mismatched request, envelope session, payload session, connection, pairing and revision", (t) => {
  const h = harness(t, [{ ...target, title_revision: 2 }, other]);
  h.start();
  const goodReply = result(h.sent[0], 3);
  for (const reply of [
    { ...goodReply, request_id: undefined },
    { ...goodReply, request_id: "different" },
    { ...goodReply, session_id: undefined },
    { ...goodReply, session_id: other.id },
    { ...goodReply, payload: { ...goodReply.payload, session_id: other.id } },
    { ...goodReply, payload: { ...goodReply.payload, title_revision: 2 } },
    { ...goodReply, payload: { ...goodReply.payload, title_revision: 2.5 } },
  ]) h.tracker.handleMessage(reply);
  const originalScope = h.scope.current;
  h.scope.current = { ...originalScope, connectionID: "connection-b" };
  h.tracker.handleMessage(goodReply);
  h.scope.current = { ...originalScope, pairingID: "pair-b" };
  h.tracker.handleMessage(goodReply);
  assert.equal(h.feedback.get(target.id).status, "pending");
  assert.deepEqual(h.applied, []);
  h.scope.current = originalScope;
  h.tracker.handleMessage(goodReply);
  h.tracker.handleMessage(goodReply);
  assert.equal(h.feedback.get(target.id).status, "success");
  assert.deepEqual(h.applied, [goodReply.payload]);
  assert.equal(h.sessions[0].title, "项目 🧪");
  assert.equal(h.sessions[1], other);
});

test("rename responses reach the editor without selection, chat errors, or session navigation side effects", (t) => {
  const h = harness(t);
  const selection = { current: other.id };
  const effects = [];
  const dispatch = useRemoteMessageHandler({
    handleSessionRenameMessage: h.tracker.handleMessage,
    handleAssetUploadMessage: () => false,
    handleModelDeleteMessage: () => false,
    sessionIDRef: selection,
    addEventMessage: () => effects.push("chat event"),
    addErrorMessage: () => effects.push("chat error"),
    applySessionChanged: () => effects.push("navigate"),
    stopPending: () => effects.push("stop sessions spinner"),
  });
  h.start();
  const request = h.sent[0];
  const error = {
    type: "error", request_id: request.request_id, session_id: request.session_id,
    payload: { request_type: "session_rename", message: "session storage unavailable" },
  };
  dispatch({ ...error, session_id: other.id });
  assert.equal(h.feedback.get(target.id).status, "pending");
  dispatch(error);
  assert.deepEqual(h.feedback.get(target.id), {
    sessionID: target.id, title: "项目 🧪", status: "error", message: "session storage unavailable",
  });
  h.start();
  dispatch(result(request));
  assert.equal(h.feedback.get(target.id).status, "pending");
  dispatch(result(h.sent[1]));
  assert.equal(h.feedback.get(target.id).status, "success");
  assert.equal(selection.current, other.id);
  assert.deepEqual(effects, []);
});

test("disconnect and timeout discard late confirmations and reconcile with the server before retry", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "Date"] });
  const h = harness(t);
  h.scope.current.connectionID = "";
  assert.equal(h.start(), false);
  assert.equal(h.feedback.get(target.id).status, "error");
  assert.equal(h.sent.length, 0);
  h.scope.current.connectionID = "connection-a";
  h.start();
  const first = h.sent[0];
  t.mock.timers.tick(30_000);
  assert.equal(h.feedback.get(target.id).status, "unconfirmed");
  assert.equal(h.refreshes, 1);
  assert.equal(h.start(), false);
  h.tracker.handleMessage(result(first));
  assert.deepEqual(h.applied, []);
  h.tracker.reconcile([target, other]);
  assert.match(h.feedback.get(target.id).message, /尚未确认/);
  h.start();
  const second = h.sent[1];
  h.scope.current.connectionID = "";
  h.tracker.disconnect();
  assert.equal(h.feedback.get(target.id).status, "unconfirmed");
  assert.equal(h.start(), false);
  h.scope.current.connectionID = "connection-b";
  h.tracker.handleMessage(result(second));
  h.tracker.reconcile([{ ...target, title: second.payload.title, title_revision: 1 }, other]);
  assert.equal(h.feedback.get(target.id).status, "success");
  assert.equal(h.sent.length, 2);
  assert.equal(h.refreshes, 1);
  assert.deepEqual(h.applied, []);
});

test("synchronous and uncertain asynchronous enqueue failures settle independently", (t) => {
  const h = harness(t);
  assert.equal(h.start(target.id, "Test", () => false), false);
  assert.match(h.feedback.get(target.id).message, /未发出/);
  assert.equal(h.start(target.id, "Test", () => { throw new Error("write failed"); }), false);
  assert.equal(h.refreshes, 0);
  h.start();
  const request = h.sent.at(-1);
  h.tracker.handleMessage({
    type: "error", request_id: request.request_id, session_id: target.id,
    payload: { request_type: "session_rename", transport_error: true },
  });
  assert.equal(h.feedback.get(target.id).status, "unconfirmed");
  assert.equal(h.refreshes, 1);
  assert.equal(h.start(), false);
});

test("a confirmed delete discards a pending title even if the same row is restored before the late reply", (t) => {
  const h = harness(t);
  const navigation = { current: { current: null, version: 0, mutations: new Map() } };
  const applier = useRemoteResultAppliers({
    relayScopeRef: h.scope,
    sessionNavigationRef: navigation,
    sessionIDRef: { current: other.id },
    setSessions: h.setSessions,
    setDeletedSessions: noop,
    onSessionDeleted: h.tracker.invalidateSession,
    removeSessionContext: noop,
  });
  h.start();
  const request = h.sent[0];
  for (const type of ["session_delete", "session_restore"]) {
    navigation.current.mutations.set(type, { type, connectionID: h.scope.current.connectionID, targetSessionID: target.id });
    assert.equal(applier.applySessionChanged({
      type: `${type}_result`, request_id: type,
      payload: { current_session_id: other.id, sessions: type === "session_delete" ? [other] : [other, target] },
    }), true);
    if (type === "session_delete") assert.deepEqual(h.sessions, [other]);
  }
  h.tracker.handleMessage(result(request));
  assert.equal(h.feedback.get(target.id).status, "unavailable");
  assert.deepEqual(h.applied, []);
  assert.deepEqual(h.sessions, [other, target]);
});

test("title revisions merge independently from newer session metadata and list membership", () => {
  const update = { session_id: target.id, title: "手动标题", title_revision: 2 };
  const current = [{ ...target, updated_at: "2026-11-21" }, other];
  const renamed = applySessionRename(current, update);
  assert.equal(renamed[1], other);
  const stale = { ...target, title: "stale", updated_at: "2026-11-22", usage: { prompt_tokens: 12 } };
  const merged = mergeSessionList(renamed, [stale]);
  assert.deepEqual(merged, [{ ...stale, title: update.title, title_revision: 2 }]);
  assert.deepEqual(mergeSessionList(merged, [{ ...stale, title: "conflicting", title_revision: 2 }]), merged);
  assert.deepEqual(upsertSession(merged, { ...target, title: "old revision", title_revision: 1, updated_at: "2026-11-23", model: "new-model" }), [
    { ...target, title: update.title, title_revision: 2, updated_at: "2026-11-23", model: "new-model" },
  ]);
  assert.deepEqual(upsertSession(merged, { ...target, title: "new revision", title_revision: 3, updated_at: "2026-11-20" }), [
    { ...merged[0], title: "new revision", title_revision: 3 },
  ]);
  assert.equal(applySessionRename(merged, { ...update, title_revision: 1 }), merged);
  assert.deepEqual(applySessionRename([other], update), [other]);
  assert.deepEqual(applySessionRename([{ ...target, deleted: true }], update), [{ ...target, deleted: true }]);
  assert.deepEqual(mergeSessionList([target], [{ ...stale, title: "new automatic title" }]), [{ ...stale, title: "new automatic title" }]);
});
