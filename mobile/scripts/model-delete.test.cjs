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

const { createModelDeleteTracker } = require("../src/utils/modelDelete.ts");
const load = Module._load;
Module._load = function (name, ...args) {
  if (name === "react") return {
    useCallback: (fn) => fn,
    useMemo: (fn) => fn(),
    useRef: (current) => ({ current }),
  };
  return load.call(this, name, ...args);
};
let useRemoteMessageHandler, useRemoteResultAppliers;
try {
  ({ useRemoteMessageHandler } = require("../src/hooks/useRemoteMessageHandler.ts"));
  ({ useRemoteResultAppliers } = require("../src/hooks/useRemoteResultAppliers.ts"));
} finally {
  Module._load = load;
}

const target = { id: "config-remove", name: "Same name", model_name: "same-provider-model" };
const copy = { ...target, id: "config-keep" };
const snapshot = [copy];
const mutationResult = (requestID, deletedID, models = snapshot) => ({
  type: "model_config_mutation_result",
  request_id: requestID,
  payload: { deleted_id: deletedID, models },
});
const error = (requestID, payload) => ({ type: "error", request_id: requestID, payload });
const noop = () => {};

function harness(t, { connectionID = "connection-1", send = () => true } = {}) {
  const relayScopeRef = { current: { pairingID: "pairing-1", connectionID } };
  const sent = [];
  const feedback = [];
  const updatedLists = [];
  const tracker = createModelDeleteTracker({
    getScope: () => relayScopeRef.current,
    onFeedback: (value) => feedback.push(value),
    onDeleted: (models) => updatedLists.push(models),
  });
  t.after(tracker.clear);
  return {
    tracker, relayScopeRef, sent, feedback, updatedLists,
    get latest() { return feedback.at(-1); },
    start(id) {
      return tracker.start(id, (type, envelope) => {
        sent.push({ type, ...envelope });
        return send();
      });
    },
  };
}

test("only matching result with the exact config ID and server catalog completes a deletion", (t) => {
  const h = harness(t);
  assert.equal(h.start(` ${target.id} `), true);
  const [request] = h.sent;
  assert.equal(request.type, "model_config_delete");
  assert.deepEqual(request.payload, { id: target.id });
  assert.equal(request.session_id, "");
  assert.ok(request.request_id);
  assert.equal(h.latest.status, "pending");
  assert.equal(h.start(copy.id), false);
  assert.equal(h.sent.length, 1);

  assert.equal(h.tracker.handleMessage(mutationResult("different-request", target.id)), true);
  assert.equal(h.tracker.handleMessage(mutationResult(request.request_id, copy.id)), true);
  assert.equal(h.tracker.handleMessage(mutationResult(request.request_id, target.id, [target, copy])), true);
  assert.equal(h.tracker.handleMessage({ type: "model_config_mutation_result", request_id: request.request_id, payload: { deleted_id: target.id } }), true);
  assert.equal(h.latest.status, "pending");
  assert.deepEqual(h.updatedLists, []);

  assert.equal(h.tracker.handleMessage(mutationResult(request.request_id, target.id)), true);
  assert.equal(h.latest.status, "success");
  assert.equal(h.latest.modelID, target.id);
  assert.deepEqual(h.updatedLists, [snapshot]);
  h.tracker.handleMessage(mutationResult(request.request_id, target.id, []));
  assert.deepEqual(h.updatedLists, [snapshot]);
});

test("model lists, unrelated mutations and unrelated errors do not settle the delete", (t) => {
  const h = harness(t);
  const modelFeedback = [];
  const effects = [];
  const dispatch = useRemoteMessageHandler({
    handleAssetUploadMessage: () => false,
    handleModelDeleteMessage: h.tracker.handleMessage,
    applyModelList: () => effects.push("list"),
    applyModelConfigMutation: () => effects.push("other-mutation"),
    applyIntentError: () => false,
    applyModelConfigError: (message) => modelFeedback.push(message),
    addErrorMessage: () => effects.push("chat-error"),
    relayScopeRef: h.relayScopeRef,
    sessionIDRef: { current: "session" },
    requestSessionMapRef: { current: {} },
    stopPending: (action) => effects.push(`stop-${action}`),
    isModelOperationPending: true,
  });
  h.start(target.id);
  const requestID = h.sent[0].request_id;
  dispatch({ type: "model_list_result", request_id: "refresh", payload: { models: [target, copy] } });
  dispatch({ type: "model_config_mutation_result", request_id: "disable", payload: { model: copy } });
  dispatch(error("other-model-operation", { request_type: "model_config_update", message: "other operation failed" }));
  dispatch(error("other-delete", { request_type: "model_config_delete", message: "wrong request" }));
  dispatch(error(requestID, { request_type: "model_config_update", message: "wrong request kind" }));
  assert.equal(h.latest.status, "pending");
  assert.deepEqual(effects, ["stop-models", "list", "stop-models", "other-mutation", "stop-models", "chat-error"]);
  assert.deepEqual(modelFeedback, ["other operation failed"]);

  dispatch(error(requestID, { request_type: "model_config_delete", message: "cannot delete model config-remove because it is used by a session" }));
  assert.equal(h.latest.status, "error");
  assert.match(h.latest.message, /在每个引用它的会话中切换/);
  assert.deepEqual(modelFeedback, ["other operation failed"]);
  assert.equal(effects.filter((item) => item === "chat-error").length, 1);
  assert.deepEqual(h.updatedLists, []);

  assert.equal(h.start(target.id), true);
  const nextRequestID = h.sent[1].request_id;
  assert.notEqual(nextRequestID, requestID);
  dispatch(mutationResult(requestID, target.id));
  assert.equal(h.latest.status, "pending");
  dispatch(mutationResult(nextRequestID, target.id));
  assert.equal(h.latest.status, "success");
  assert.deepEqual(h.updatedLists, [snapshot]);
});

test("model protection errors give actionable guidance (including legacy untyped errors)", (t) => {
  const h = harness(t);
  for (const [payload, expected] of [
    [{ message: "cannot delete the default model; select another default model first" }, /设为默认/],
    [{ message: "cannot delete the last enabled model; enable or add another model first" }, /启用或添加/],
    [{ message: "model not found: config-remove" }, /刷新模型列表.*配置 ID/],
    [{ message: "agent is not online: user=test device=test" }, /确认电脑端已启动/],
    [{ error_code: "client_auth_rejected" }, /重新配对/],
    [{ error_code: "client_auth_unavailable", message: "client token is invalid or expired" }, /检查中继状态/],
    [{ message: "storage error" }, /刷新模型列表确认结果/],
  ]) {
    h.start(target.id);
    assert.equal(h.tracker.handleMessage(error(h.sent.at(-1).request_id, payload)), true);
    assert.equal(h.latest.status, "error");
    assert.match(h.latest.message, expected);
  }
  assert.deepEqual(h.updatedLists, []);
});

test("missing transport, synchronous rejection and async enqueue failures are shown in settings", (t) => {
  const offline = harness(t, { connectionID: "" });
  assert.equal(offline.start(target.id), false);
  assert.equal(offline.sent.length, 0);
  assert.match(offline.latest.message, /未发出/);
  for (const send of [() => false, () => { throw new Error("fake socket write failed"); }]) {
    const failing = harness(t, { send });
    assert.equal(failing.start(target.id), false);
    assert.equal(failing.latest.status, "error");
    assert.match(failing.latest.message, /未发出/);
    assert.equal(failing.sent.length, 1);
  }
  const h = harness(t);
  h.start(target.id);
  assert.equal(h.tracker.handleMessage(error(h.sent[0].request_id, {
    request_type: "model_config_delete", transport_error: true,
  })), true);
  assert.equal(h.latest.status, "error");
  assert.match(h.latest.message, /无法确认.*先刷新/);
});

test("timeout and disconnect never resend or let a late reply affect a later delete", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "Date"] });
  const h = harness(t);
  h.start(target.id);
  const first = h.sent[0];
  t.mock.timers.tick(30_000);
  assert.equal(h.latest.status, "error");
  assert.match(h.latest.message, /超时.*先刷新/);
  h.tracker.handleMessage(mutationResult(first.request_id, target.id));
  assert.deepEqual(h.updatedLists, []);
  assert.equal(h.sent.length, 1);

  h.start(target.id);
  const second = h.sent[1];
  h.tracker.disconnect();
  assert.equal(h.latest.status, "error");
  assert.match(h.latest.message, /结果未确认.*重连后刷新/);
  h.relayScopeRef.current = { ...h.relayScopeRef.current, connectionID: "connection-2" };
  h.tracker.handleMessage(error(second.request_id, { message: "late error" }));
  t.mock.timers.tick(30_000);
  assert.deepEqual(h.updatedLists, []);
  assert.equal(h.sent.length, 2);

  h.start(copy.id);
  h.tracker.handleMessage(mutationResult(first.request_id, target.id));
  assert.equal(h.latest.status, "pending");
  h.relayScopeRef.current = { ...h.relayScopeRef.current, pairingID: "replacement" };
  h.tracker.handleMessage(mutationResult(h.sent[2].request_id, copy.id, [target]));
  assert.equal(h.latest.status, "error");
  assert.match(h.latest.message, /结果未确认/);
  assert.deepEqual(h.updatedLists, []);
  assert.equal(h.sent.length, 3);
});

test("only an accepted session change marks that session's active model", () => {
  const selected = { current: "old-session" };
  const activeModels = [];
  const applier = useRemoteResultAppliers({
    sessionIDRef: selected,
    relayScopeRef: { current: { pairingID: "pairing", connectionID: "current" } },
    sessionNavigationRef: { current: { current: {
      connectionID: "current", requestID: "current-navigation", targetSessionID: "session-b",
    }, version: 1, mutations: new Map() } },
    historySessionIDRef: { current: "old-session" },
    pendingHistorySessionIDRef: { current: "" },
    requestSessionHistory: noop,
    requestAssets: noop,
    setSessions: noop,
    setSessionID: noop,
    setSessionLastUsage: noop,
    setCurrentModelID: (id) => activeModels.push(id),
  });
  const change = {
    type: "session_changed", request_id: "current-navigation", payload: {
      current_session_id: "session-b",
      session: { id: "old-session", model: target.id },
      sessions: [{ id: "session-b", model: copy.id }],
    },
  };
  assert.equal(applier.applySessionChanged({ ...change, request_id: "old-navigation" }), false);
  assert.deepEqual(activeModels, []);
  assert.equal(applier.applySessionChanged(change), true);
  assert.equal(selected.current, "session-b");
  assert.deepEqual(activeModels, [copy.id]);
});
