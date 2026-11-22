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

let hooks;
let useSessionContext;
const originalLoad = Module._load;
Module._load = function (name, ...args) {
  if (name === "react") {
    return {
      useCallback: (callback) => callback,
      useEffect: (setup) => { hooks.cleanup ||= setup(); },
      useLayoutEffect: (setup, deps) => {
        if (hooks.layoutSession !== deps[1]) {
          hooks.layoutSession = deps[1];
          setup();
        }
      },
      useRef: (initial) => hooks.ref ||= { current: initial },
      useState: (initial) => {
        const index = hooks.stateIndex++;
        if (!(index in hooks.state)) hooks.state[index] = initial;
        return [hooks.state[index], (value) => {
          hooks.state[index] = typeof value === "function" ? value(hooks.state[index]) : value;
        }];
      },
    };
  }
  return originalLoad.call(this, name, ...args);
};
try {
  ({ useSessionContext } = require("../src/hooks/useSessionContext.ts"));
} finally {
  Module._load = originalLoad;
}
const { describeSessionContext, formatContextUsage, mergeSessionContext } = require("../src/utils/sessionContext.ts");

const sample = {
  model: "sample-model",
  window_tokens: 32768,
  message_tokens: 104,
  system_tokens: 23,
  skill_tokens: 0,
  mcp_tool_tokens: 19,
  other_tool_tokens: 5,
  other_tokens: 7,
  output_reserve_tokens: 1024,
  safety_reserve_tokens: 0,
};

const checkpoint = { version: 2, source_start_message: 0, source_end_message: 5, summary: "checkpoint text" };
const snapshot = {
  selected_tokens: 250, window_k: 64, summary: "query summary", checkpoint,
  has_summary: true, summary_version: 2, summary_hash: "query-hash",
};

function fixture(t) {
  const state = { state: [], cleanup: null };
  t.after(() => state.cleanup?.());
  const relayScopeRef = { current: { connectionID: "conn-1", pairingID: "pair" } };
  const sessionIDRef = { current: "a" };
  const sent = [];
  const sendEnvelope = (type, overrides) => {
    sent.push({ type, ...overrides });
    return true;
  };
  const render = (sessionID = sessionIDRef.current) => {
    sessionIDRef.current = sessionID;
    hooks = state;
    hooks.stateIndex = 0;
    return useSessionContext({ relayScopeRef, sessionID, sessionIDRef, sendEnvelope });
  };
  const reply = (request, context, payloadSession = request.session_id) => ({
    type: "session_context_query_result", request_id: request.request_id, session_id: request.session_id,
    payload: { session_id: payloadSession, context },
  });
  return { relayScopeRef, render, reply, sent };
}

test("request input sums the six categories and keeps the precise sample window and reserve values", () => {
  const view = describeSessionContext({ ...snapshot, request_context: sample }, 128);
  assert.equal(view.used, 158);
  assert.equal(view.window, 32768);
  assert.equal(view.snapshotUsed, 250);
  assert.equal(view.snapshotWindow, 64000);
  assert.deepEqual(view.categories.map(({ tokens }) => tokens), [104, 23, 0, 19, 5, 7]);
  assert.equal(view.sample.output_reserve_tokens, 1024);
  assert.equal(view.sample.safety_reserve_tokens, 0);
  assert.equal(formatContextUsage(view.used, view.window), "158 / 32.8k");
});

test("legacy context uses only transcript estimates and preserves zero instead of treating it as missing", () => {
  const legacy = describeSessionContext({ selected_tokens: 0 }, 16);
  assert.equal(legacy.sample, undefined);
  assert.deepEqual(legacy.categories, []);
  assert.equal(formatContextUsage(legacy.used, legacy.window), "0 / 16k");
  const unknown = describeSessionContext(undefined, 16);
  assert.equal(formatContextUsage(unknown.used, unknown.window), "— / 16k");
});

test("live and snapshot updates preserve their complementary data", () => {
  const updated = mergeSessionContext(snapshot, { selected_tokens: 300, request_context: sample }, "live");
  assert.equal(updated.summary, "query summary");
  assert.equal(updated.summary_hash, "query-hash");
  assert.equal(updated.window_k, 64);
  assert.equal(updated.checkpoint, checkpoint);
  assert.equal(updated.selected_tokens, 300);
  assert.equal(mergeSessionContext(updated, snapshot, "snapshot").request_context, sample);
  const next = { ...checkpoint, version: 3, summary: "new checkpoint" };
  assert.equal(mergeSessionContext(updated, { checkpoint: next }, "live").summary, "new checkpoint");
  assert.equal(mergeSessionContext(updated, { has_summary: false }, "snapshot").summary, undefined);
  for (const reset of [{ has_summary: false }, { summary_version: 0 }]) {
    const cleared = mergeSessionContext(updated, reset, "live");
    assert.equal(cleared.has_summary, false);
    assert.equal(cleared.summary_version, 0);
    assert.equal(cleared.summary, undefined);
    assert.equal(cleared.checkpoint, undefined);
    assert.equal(cleared.summary_hash, undefined);
    assert.equal(cleared.request_context, sample);
  }
});

test("switching sessions isolates pending state and drops stale and conflicting replies", (t) => {
  const { render, reply, sent } = fixture(t);
  render().requestContextInfo();
  render().requestContextInfo();
  assert.equal(sent.length, 1);
  const [a] = sent;
  assert.equal(render().pendingContextSessionID, "a");
  render("b");
  const bView = render();
  assert.equal(bView.pendingContextSessionID, "");
  bView.requestContextInfo();
  const b = sent[1];
  bView.handleContextMessage(reply(a, { selected_tokens: 10 }));
  bView.handleContextMessage({
    type: "error", request_id: a.request_id, session_id: "a", payload: { request_type: "session_context_query" },
  });
  bView.handleContextMessage(reply(b, { selected_tokens: 99 }, "a"));
  assert.deepEqual(render().sessionContexts, {});
  assert.equal(render().pendingContextSessionID, "b");
  bView.handleContextMessage(reply(b, snapshot));
  assert.equal(render().pendingContextSessionID, "");
  assert.equal(render().sessionContexts.b.summary, "query summary");
  render("a").requestContextInfo();
  bView.handleContextMessage(reply(a, { selected_tokens: 10 }));
  assert.equal(render().pendingContextSessionID, "a");
  assert.equal(render().sessionContexts.a, undefined);
});

test("a late query adds the snapshot and summary without restoring an older live sample", (t) => {
  const { render, reply, sent } = fixture(t);
  const view = render();
  view.requestContextInfo();
  view.setSessionContext("a", { request_context: sample }, "live");
  view.handleContextMessage(reply(sent[0], { ...snapshot, request_context: { ...sample, model: "older" } }));
  assert.equal(render().sessionContexts.a.request_context, sample);
  assert.equal(render().sessionContexts.a.summary, "query summary");
});

test("removing a session evicts its context and invalidates that session’s outstanding read", (t) => {
  const { render, reply, sent } = fixture(t);
  const view = render();
  view.setSessionContext("a", snapshot);
  view.setSessionContext("b", { selected_tokens: 8 });
  view.requestContextInfo();
  view.removeSessionContext("a");
  view.handleContextMessage(reply(sent[0], snapshot));
  assert.equal(render().pendingContextSessionID, "");
  assert.deepEqual(render().sessionContexts, { b: { selected_tokens: 8, request_context: undefined } });
});

test("reconnect and query errors only settle the matching request", (t) => {
  const { relayScopeRef, render, reply, sent } = fixture(t);
  const view = render();
  view.requestContextInfo();
  relayScopeRef.current = { ...relayScopeRef.current, connectionID: "conn-2" };
  view.handleContextMessage(reply(sent[0], snapshot));
  assert.deepEqual(render().sessionContexts, {});
  view.resetContextQuery();
  view.requestContextInfo();
  const error = (request) => ({
    type: "error", request_id: request.request_id, session_id: "a", payload: { request_type: "session_context_query" },
  });
  view.handleContextMessage(error(sent[0]));
  assert.equal(render().pendingContextSessionID, "a");
  view.handleContextMessage(error(sent[1]));
  assert.equal(render().pendingContextSessionID, "");
});
