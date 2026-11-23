const assert = require("node:assert/strict");
const fs = require("node:fs");
const Module = require("node:module");
const { test } = require("node:test");
const ts = require("typescript");

for (const extension of [".ts", ".tsx"]) {
  require.extensions[extension] = (module, filename) => {
    const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX },
      fileName: filename,
    });
    module._compile(outputText, filename);
  };
}

let fixture;
const originalLoad = Module._load;
Module._load = function (name, ...args) {
  if (name === "react") {
    return {
      useCallback: (callback) => callback,
      useRef: (initial) => ({ current: initial }),
      useEffect: (setup) => { fixture.cleanups.push(setup()); },
    };
  }
  if (name === "../storage/sessionHistoryCache") {
    return {
      loadCachedSessionHistory: (key) => fixture.readCache(key),
      replaceCachedSessionHistory: async (key, messages) => { fixture.writes.push({ key, messages }); },
    };
  }
  return originalLoad.call(this, name, ...args);
};
let useRelaySender, useSessionHistorySync, useRemoteRequests;
try {
  ({ useRelaySender } = require("../src/hooks/useRelaySender.ts"));
  ({ useSessionHistorySync } = require("../src/hooks/useSessionHistorySync.ts"));
  ({ useRemoteRequests } = require("../src/hooks/useRemoteRequests.ts"));
} finally {
  Module._load = originalLoad;
}
const { normalizeRelayUrl, restoreRelaySettings } = require("../src/utils/relayPairing.ts");
const { startRelayWebSocket } = require("../src/utils/relayWebSocket.ts");
const { createMCPRequests } = require("../src/hooks/useMCPState.ts");
const { createMCPTrustRequests } = require("../src/hooks/useMCPTrust.ts");

const noop = () => {};
const tick = () => new Promise((resolve) => setImmediate(resolve));
const pairing = {
  pairingID: "pairing-a", relayURL: "https://relay.example.invalid",
  userID: "test-user", deviceID: "test-agent", clientToken: "test-credential",
};
const identity = { user_id: pairing.userID, device_id: pairing.deviceID };

function socketFixture(t) {
  const previous = globalThis.WebSocket;
  const sockets = [];
  globalThis.WebSocket = class FakeWebSocket {
    static OPEN = 1;
    constructor(url) {
      this.url = url;
      this.readyState = 0;
      this.sent = [];
      this.closed = 0;
      sockets.push(this);
    }
    open() { this.readyState = 1; this.onopen?.(); }
    close() { this.readyState = 3; this.closed++; }
    send(data) { this.sent.push(JSON.parse(data)); }
    receive(message) { this.onmessage?.({ data: JSON.stringify(message) }); }
  };
  const available = [];
  const received = [];
  const losses = [];
  const close = startRelayWebSocket(pairing, {
    onAuthenticating: noop,
    onAuthenticated: (handle) => available.push(handle),
    onMessage: (message) => received.push(message),
    onLost: (loss) => losses.push(loss),
  });
  t.after(() => { close(); globalThis.WebSocket = previous; });
  return { socket: sockets[0], available, received, losses, close };
}

const heartbeatAck = (requestID, fields = {}) => ({
  ...identity,
  type: "heartbeat", request_id: requestID, payload: { received: "heartbeat" }, ...fields,
});

test("Relay URL migration preserves the issuer and rejects incomplete settings", () => {
  assert.equal(normalizeRelayUrl("WSS://Relay.Example.Invalid:443/ws/client/"), pairing.relayURL);
  assert.equal(normalizeRelayUrl("10.0.2.2:18080"), "http://10.0.2.2:18080");
  assert.equal(normalizeRelayUrl("https://relay.example.invalid:99999"), "");
  assert.equal(normalizeRelayUrl("https://relay.example.invalid/?x=y"), "");
  const restored = restoreRelaySettings({ ...pairing, pairingID: "", relayURL: "http://127.0.0.1:18080" });
  assert.equal(restored.pairing.relayURL, "http://127.0.0.1:18080");
  assert.ok(restored.pairing.pairingID);
  assert.equal(restoreRelaySettings({ ...pairing, relayURL: "" }).pairing, null);
  assert.equal(restoreRelaySettings({ ...pairing, deviceID: "" }).pairing, null);
});

test("pairing authorization ID lookup uses the paired HTTPS issuer with no redirect or token in URL", async () => {
  let fetchOwnAuthorizationID;
  const calls = [];
  const id = "ab".repeat(32);
  let response = {};
  try {
    Module._load = (name, ...args) => {
      if (name === "expo/fetch") return { fetch: async (url, options) => {
        calls.push({ url, options });
        return { url, ok: true, redirected: false, json: async () => ({ authorizations: [
          { current: false, id: "cd".repeat(32), ...identity }, { current: true, id, ...identity },
        ] }), ...response };
      } };
      if (name === "../utils/relay") return { clientName: () => "test" };
      return originalLoad.call(Module, name, ...args);
    };
    ({ fetchOwnAuthorizationID } = require("../src/services/pairing.ts"));
  } finally {
    Module._load = originalLoad;
  }
  const { signal } = new AbortController();
  assert.equal(await fetchOwnAuthorizationID(pairing, signal), id);
  assert.deepEqual(calls[0], {
    url: `${pairing.relayURL}/authorizations?user_id=${pairing.userID}&device_id=${pairing.deviceID}`,
    options: { headers: { Authorization: `Bearer ${pairing.clientToken}`, Accept: "application/json" }, credentials: "omit", redirect: "error", signal },
  });
  await assert.rejects(fetchOwnAuthorizationID({ ...pairing, relayURL: "http://relay.example.invalid" }, signal));
  assert.equal(calls.length, 1);
  response = { redirected: true };
  await assert.rejects(fetchOwnAuthorizationID(pairing, signal));
  response = { json: async () => ({ authorizations: [{ current: true, id: pairing.clientToken, ...identity }] }) };
  await assert.rejects(fetchOwnAuthorizationID(pairing, signal));
  response = { json: async () => ({ authorizations: [{ current: true, id, ...identity }] }) };
  await assert.rejects(fetchOwnAuthorizationID({ ...pairing, clientToken: id }, signal));
  response = { json: async () => ({ authorizations: [{ current: true, id, ...identity, device_id: "different-device" }] }) };
  await assert.rejects(fetchOwnAuthorizationID(pairing, signal));
});

test("a WebSocket authenticates only on its own heartbeat and then drains replay", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { socket, available, received, losses } = socketFixture(t);
  assert.equal(socket.url, "wss://relay.example.invalid/ws/client");
  socket.open();
  const first = socket.sent[0];
  assert.equal(first.type, "heartbeat");
  assert.equal(first.client_token, pairing.clientToken);
  socket.receive({ ...identity, type: "permission_ask", request_id: "existing-turn", client_token: "omit" });
  socket.receive(heartbeatAck("another-request"));
  socket.receive(heartbeatAck(first.request_id, { device_id: "different-device" }));
  socket.receive(heartbeatAck(first.request_id, { payload: { received: "session_list" } }));
  assert.deepEqual(available, []);
  assert.deepEqual(received, []);
  socket.receive(heartbeatAck(first.request_id));
  assert.equal(available.length, 1);
  assert.equal(available[0].pairing, pairing);
  assert.deepEqual(received, [{ ...identity, type: "permission_ask", request_id: "existing-turn" }]);

  t.mock.timers.tick(25_000);
  const next = socket.sent[1];
  assert.notEqual(next.request_id, first.request_id);
  socket.receive(heartbeatAck(first.request_id));
  t.mock.timers.tick(10_000);
  assert.deepEqual(losses.map((loss) => loss.state), ["reconnecting"]);
  assert.equal(available.length, 1);
});

test("typed pairing rejection and temporary storage failure stay offline with different retry states", (t) => {
  for (const [payload, expected] of [
    [{ error_code: "client_auth_rejected" }, "auth_rejected"],
    [{ error_code: "client_auth_unavailable" }, "reconnecting"],
    [{ message: "client token is invalid or expired" }, "reconnecting"],
  ]) {
    const { socket, available, received, losses, close } = socketFixture(t);
    socket.open();
    socket.receive({ ...identity, type: "error", request_id: socket.sent[0].request_id, payload });
    assert.deepEqual(losses.map((loss) => loss.state), [expected]);
    assert.equal(available.length, 0);
    assert.equal(received.length, 0);
    close();
  }
});

test("a retired socket closes on a late upgrade and sends no heartbeat", (t) => {
  const { socket, available, received, close } = socketFixture(t);
  close();
  socket.open();
  assert.equal(socket.closed, 2);
  assert.equal(socket.sent.length, 0);
  assert.equal(available.length, 0);
  assert.equal(received.length, 0);
});

test("a definitive native rejection reaches the original turn after disconnect, but never after re-pair", async () => {
  const socketRef = { current: null };
  const currentPairingRef = { current: pairing };
  const failures = [];
  const errors = [];
  const frames = [];
  const replies = [];
  socketRef.current = {
    pairing, connectionID: "socket-a",
    send: (raw) => { frames.push(JSON.parse(raw)); return new Promise((resolve) => replies.push(resolve)); },
  };
  const send = useRelaySender({
    socketRef, currentPairingRef, activeRequestIDRef: { current: "" }, sessionIDRef: { current: "session-a" },
    onEnqueueFailure: (frame) => failures.push(frame), addErrorMessage: (text) => errors.push(text),
  });
  assert.equal(send("user_message", { request_id: "chat-a" }), true);
  assert.equal(send("asset_list", { request_id: "asset-a" }), true);
  assert.equal(frames[0].user_id, pairing.userID);
  assert.equal(frames[0].session_id, "session-a");
  const old = socketRef.current;
  socketRef.current = null;
  replies[1](false);
  replies[0](false);
  await tick();
  assert.equal(failures.length, 1);
  assert.equal(failures[0].request_id, "chat-a");
  assert.equal(failures[0].payload.request_type, "user_message");
  assert.equal(failures[0].payload.transport_error, true);
  socketRef.current = old;
  assert.equal(send("user_message", { request_id: "chat-b" }), true);
  currentPairingRef.current = { ...pairing, pairingID: "pairing-b" };
  assert.equal(send("user_message"), false);
  replies[2](false);
  await tick();
  assert.equal(failures.length, 1);
  assert.equal(errors.length, 1);
});

function historyFixture(t, readCache = async () => ({ messages: [], meta: {} }), hook = useSessionHistorySync) {
  const args = {
    historySessionIDRef: { current: "" }, pendingHistorySessionIDRef: { current: "" },
    relayScopeRef: { current: { pairingID: "pairing-a", connectionID: "connection-a" } },
    sessionIDRef: { current: "session-a" },
  };
  const sent = [], merged = [], pending = [], writes = [], cleanups = [];
  fixture = { readCache, writes, cleanups };
  t.after(() => { for (const cleanup of cleanups) cleanup?.(); });
  const api = hook({
    ...args, clientToken: pairing.clientToken,
    sendEnvelope: (type, frame) => { sent.push({ type, ...frame }); return true; },
    mergeSessionHistory: (sessionID, messages, source) => merged.push({ sessionID, messages, source }),
    startPending: (group) => pending.push([group, true]), stopPending: (group) => pending.push([group, false]),
  });
  const frame = (request, payload) => ({
    type: `${request.type}_result`, request_id: request.request_id, session_id: request.session_id,
    payload: { session_id: request.session_id, ...payload },
  });
  return { api, args, sent, merged, pending, writes, frame };
}

const stored = (id) => ({ id, role: "user", content: `Message ${id}` });

test("history follows multiple pages to the server's last known row", async (t) => {
  const { api, args, sent, merged, writes, frame } = historyFixture(t, async () => ({
    messages: [stored("m0")], meta: { local_last_message_id: "m0", local_message_count: 1 },
  }));
  assert.equal(api.requestHistoryWithCache("session-a"), true);
  await tick();
  assert.equal(sent[0].type, "session_history_meta");
  assert.equal(api.handleSessionHistoryMessage(frame(sent[0], { can_delta: true, message_count: 102, last_message_id: "m101" })), true);
  const page = Array.from({ length: 100 }, (_, index) => stored(`m${index + 1}`));
  assert.equal(api.handleSessionHistoryMessage(frame(sent[1], { messages: page, count: 100 })), true);
  assert.equal(args.pendingHistorySessionIDRef.current, "session-a");
  assert.equal(sent[2].payload.after_message_id, "m100");
  assert.equal(api.handleSessionHistoryMessage(frame(sent[2], { messages: [stored("m101")], count: 1 })), true);
  await tick();
  assert.equal(args.historySessionIDRef.current, "session-a");
  assert.equal(args.pendingHistorySessionIDRef.current, "");
  assert.equal(merged.at(-1).messages.length, 102);
  assert.equal(writes[0].messages[101].id, "m101");
});

test("a missing history watermark triggers full sync and old connection replies never apply", async (t) => {
  const { api, args, sent, merged, frame } = historyFixture(t, async () => ({
    messages: [stored("m0")], meta: { local_last_message_id: "m0", local_message_count: 1 },
  }));
  api.requestHistoryWithCache("session-a");
  await tick();
  api.handleSessionHistoryMessage(frame(sent[0], { can_delta: true, message_count: 2, last_message_id: "m1" }));
  api.handleSessionHistoryMessage(frame(sent[1], { messages: [stored("other")], count: 1 }));
  assert.equal(sent[2].type, "session_history");
  const old = frame(sent[2], { messages: [stored("old")], count: 1 });
  args.relayScopeRef.current = { pairingID: "pairing-a", connectionID: "connection-b" };
  api.resetHistorySync();
  api.requestSessionHistoryFull("session-a");
  assert.equal(api.handleSessionHistoryMessage(old), false);
  assert.equal(api.handleSessionHistoryMessage(frame(sent[3], { messages: [stored("new")], count: 1 })), true);
  assert.deepEqual(merged.map((entry) => entry.messages.map((message) => message.id)), [["m0"], ["new"]]);
  await tick();
});

test("asset list accepts only the latest request for the selected session", (t) => {
  const { api, args, sent, frame } = historyFixture(t, undefined, useRemoteRequests);
  assert.equal(api.requestAssets("session-a"), true);
  args.sessionIDRef.current = "session-b";
  assert.equal(api.requestAssets("session-b"), true);
  args.sessionIDRef.current = "session-a";
  assert.equal(api.requestAssets("session-a"), true);
  assert.equal(api.acceptAssetList(frame(sent[0], { assets: [] })), false);
  assert.equal(api.acceptAssetList(frame(sent[1], { assets: [] })), false);
  assert.equal(api.acceptAssetList(frame(sent[2], { assets: [] })), true);
  assert.equal(api.acceptAssetList(frame(sent[2], { assets: [] })), false);
});

const mcpServers = [
  { name: "files", state: "loaded", inventory_tool_count: 3, exposed_tool_count: 2 },
  { name: "notes", state: "disabled", inventory_tool_count: 0, exposed_tool_count: 0 },
  { name: "search", state: "unavailable", inventory_tool_count: 0, exposed_tool_count: 0 },
];
const mcpList = (request, servers = mcpServers, catalog_revision) => ({
  type: "mcp_list_result", request_id: request.request_id,
  payload: { servers, ...(catalog_revision === undefined ? {} : { catalog_revision }) },
});
const mcpReload = (request, reloaded = true) => ({
  type: "mcp_reload_result", request_id: request.request_id, payload: { reloaded, message: "agent detail" },
});
const mcpError = (request, payload) => ({ type: "error", request_id: request.request_id, session_id: request.session_id, payload });

function mcpFixture(t, { send = () => true, useTransport = false, sessionID = "session-a", onReloaded } = {}) {
  const scope = { current: { pairingID: pairing.pairingID, connectionID: "connection-a" } };
  const session = { current: sessionID };
  const sent = [], states = [], chatErrors = [];
  const sendFromRelay = useTransport ? useRelaySender({
    socketRef: { current: { pairing, connectionID: "connection-a", send } },
    currentPairingRef: { current: pairing }, activeRequestIDRef: { current: "turn-a" }, sessionIDRef: { current: "session-a" },
    onEnqueueFailure: (message) => api.handleMessage(message), addErrorMessage: (message) => chatErrors.push(message),
  }) : send;
  const api = createMCPRequests({
    getScope: () => scope.current,
    getSessionID: () => session.current,
    sendEnvelope: (type, frame) => { sent.push({ type, ...frame }); return sendFromRelay(type, frame); },
    onChange: (state) => states.push(state),
    onReloaded,
  });
  t.after(api.clear);
  return { api, scope, session, sent, states, chatErrors, get state() { return states.at(-1); } };
}

test("MCP inventory and reload own separate global requests and accept explicit empty inventory", (t) => {
  const h = mcpFixture(t);
  assert.equal(h.api.refresh(), true);
  assert.equal(h.api.reload(), true);
  const [list, reload] = h.sent;
  assert.deepEqual(h.sent.map(({ request_id, ...frame }) => frame), [
    { type: "mcp_list", session_id: "", payload: {} },
    { type: "mcp_reload", session_id: "", payload: {} },
  ]);
  assert.notEqual(list.request_id, reload.request_id);
  assert.equal(h.api.refresh(), false);
  assert.equal(h.api.reload(), false);
  h.api.handleMessage(mcpReload(list));
  h.api.handleMessage(mcpList(reload));
  h.api.handleMessage(mcpError(list, { request_type: "mcp_reload" }));
  assert.equal(h.state.reloading, true);
  assert.deepEqual(h.state.inventory, { servers: null, status: "loading" });

  h.api.handleMessage(mcpList(list, []));
  assert.deepEqual(h.state.inventory, { servers: [], status: "ready" });
  assert.equal(h.state.reloading, true);
  h.api.handleMessage(mcpReload(reload, false));
  assert.equal(h.state.reloading, false);
  assert.match(h.state.reloadMessage, /重载失败/);
  assert.equal(h.sent.length, 2);
});

test("only a confirmed MCP reload replaces a pre-reload inventory request", (t) => {
  const h = mcpFixture(t);
  h.api.refresh();
  h.api.reload();
  const [before, reload] = h.sent;
  h.api.handleMessage(mcpReload(reload));
  const after = h.sent[2];
  assert.equal(after.type, "mcp_list");
  assert.notEqual(after.request_id, before.request_id);
  assert.match(h.state.reloadMessage, /重载成功/);
  h.api.handleMessage(mcpList(before));
  h.api.handleMessage(mcpError(before, { message: "unsupported client message type: mcp_list" }));
  assert.deepEqual(h.state.inventory, { servers: null, status: "loading" });
  h.api.handleMessage(mcpList(after));
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "ready" });
  h.api.handleMessage(mcpReload(reload));
  assert.equal(h.sent.length, 3);
});

test("same-pairing reconnect keeps stale MCP inventory and re-pair discards it and reload feedback", (t) => {
  const h = mcpFixture(t);
  h.api.refresh();
  h.api.handleMessage(mcpList(h.sent[0]));
  h.api.refresh();
  h.api.reload();
  const [, oldList, oldReload] = h.sent;
  h.scope.current = { ...h.scope.current, connectionID: "" };
  h.api.handleDisconnect();
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "idle" });
  assert.equal(h.state.reloading, false);
  assert.match(h.state.reloadMessage, /重载结果未确认/);
  assert.equal(h.api.refresh(), false);
  h.scope.current = { ...h.scope.current, connectionID: "connection-b" };
  h.api.refresh();
  h.api.handleMessage(mcpList(oldList, []));
  h.api.handleMessage(mcpReload(oldReload));
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "loading" });
  assert.equal(h.sent.length, 4);

  h.scope.current = { pairingID: "pairing-b", connectionID: "connection-b" };
  h.api.handleMessage(mcpList(h.sent[3], []));
  assert.deepEqual(h.state.inventory, { servers: null, status: "idle" });
  assert.equal(h.state.reloadMessage, "");
  h.api.refresh();
  h.api.handleMessage(mcpList(h.sent[4], []));
  assert.deepEqual(h.state.inventory, { servers: [], status: "ready" });
});

test("legacy MCP rejection, malformed inventory, and silent timeout keep the previous snapshot", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "Date"] });
  const h = mcpFixture(t);
  h.api.refresh();
  h.api.handleMessage(mcpList(h.sent[0]));
  for (const requestType of ["mcp_list", undefined]) {
    h.api.refresh();
    h.api.handleMessage(mcpError(h.sent.at(-1), { request_type: requestType, message: "unsupported client message type: mcp_list" }));
    assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "unsupported" });
  }
  h.api.refresh();
  h.api.handleMessage({ ...mcpList(h.sent.at(-1)), payload: {} });
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "error" });
  h.api.refresh();
  h.api.reload();
  t.mock.timers.tick(15_000);
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "timeout" });
  assert.equal(h.state.reloading, true);
  h.api.handleMessage(mcpList(h.sent.at(-2), []));
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "timeout" });
  t.mock.timers.tick(45_000);
  assert.equal(h.state.reloading, false);
  assert.match(h.state.reloadMessage, /重载确认超时/);
  h.api.handleMessage(mcpReload(h.sent.at(-1)));
  assert.equal(h.sent.length, 6);
});

test("MCP transport failures stay in settings and cleanup ignores late results", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const frames = [];
  const h = mcpFixture(t, { useTransport: true, send: (raw) => {
    const frame = JSON.parse(raw);
    frames.push(frame);
    if (frame.type === "mcp_list") throw new Error("private transport detail");
    return Promise.resolve(false);
  } });
  assert.equal(h.api.refresh(), false);
  assert.equal(h.state.inventory.status, "error");
  assert.equal(h.api.reload(), true);
  await tick();
  assert.equal(h.state.reloading, false);
  assert.match(h.state.reloadMessage, /重载失败/);
  assert.deepEqual(h.chatErrors, []);
  assert.ok(frames.every((frame) => frame.session_id === "" && frame.request_id !== "turn-a"));

  const next = mcpFixture(t);
  next.api.refresh();
  next.api.reload();
  next.api.clear();
  const updates = next.states.length;
  t.mock.timers.tick(60_000);
  next.api.handleMessage(mcpList(next.sent[0]));
  next.api.handleMessage(mcpReload(next.sent[1]));
  assert.equal(next.states.length, updates);
  assert.equal(next.sent.length, 2);
});

const summaryWithReasons = [mcpServers[0], mcpServers[1], { ...mcpServers[2], reason: "pending_trust" }];
const tool = (id, props = {}) => ({
  id, name: id, host_only: false, eligibility: "available",
  input: { state: "empty" }, output: { state: "undeclared" }, ...props,
});
const mcpPage = (request, props = {}) => ({
  type: "mcp_detail_result", request_id: request.request_id, session_id: request.session_id || undefined,
  payload: {
    server: summaryWithReasons.find((row) => row.name === request.payload.server_name),
    catalog_revision: 7, session_evaluated: Boolean(request.session_id), tools: [], ...props,
  },
});

function loadSummary(h, servers = summaryWithReasons, revision = 7) {
  h.api.refresh();
  h.api.handleMessage(mcpList(h.sent.at(-1), servers, revision));
}

test("MCP detail owns server, session, request and socket; only matching pages append", (t) => {
  const h = mcpFixture(t);
  loadSummary(h);
  assert.equal(h.state.inventory.catalogRevision, 7);
  assert.equal(h.state.inventory.servers.at(-1).reason, "pending_trust");
  assert.equal(h.api.openDetail("not-in-the-list"), false);
  h.api.openDetail("notes");
  const replaced = h.sent.at(-1);
  h.api.openDetail("files");
  const first = h.sent.at(-1);
  assert.equal(h.api.openDetail("files"), false);
  assert.deepEqual(first.payload, { server_name: "files" });
  assert.equal(first.type, "mcp_detail");
  assert.equal(first.session_id, "session-a");
  assert.match(first.request_id, /^mcp-detail-/);
  h.api.handleMessage(mcpPage(replaced));
  h.api.handleMessage({ ...mcpPage(first), session_id: "session-b" });
  h.api.handleMessage({ ...mcpPage(first), session_id: undefined });
  h.api.handleMessage(mcpPage(first, { server: mcpServers[1] }));
  h.api.handleMessage(mcpList(first, []));
  h.api.handleMessage(mcpError(first, { request_type: "mcp_reload" }));
  h.api.handleMessage(mcpError(first, { request_type: "mcp_detail", server_name: "notes", code: "mcp_catalog_changed" }));
  assert.equal(h.state.detail.status, "loading");
  h.api.handleMessage(mcpPage(first, { tools: [tool("01")], next_cursor: "page-2" }));
  assert.equal(h.state.detail.status, "ready");
  assert.equal(h.api.loadMoreDetail(), true);
  assert.equal(h.api.loadMoreDetail(), false);
  const second = h.sent.at(-1);
  assert.deepEqual(second.payload, { server_name: "files", cursor: "page-2" });
  h.api.openTool("01");
  h.api.handleMessage(mcpPage(second, { tools: [tool("02")] }));
  assert.deepEqual(h.state.detail.result.tools.map(({ id }) => id), ["01", "02"]);
  assert.equal(h.state.detail.selectedToolID, "01");
  assert.equal(h.api.loadMoreDetail(), false);
  h.api.backFromDetail();
  assert.equal(h.state.detail.selectedToolID, undefined);
  h.api.backFromDetail();
  assert.equal(h.state.detail, null);
});

test("MCP detail doesn't mix catalog revisions or evaluated and unevaluated pages", (t) => {
  const scenarios = [
    (request) => mcpError(request, { code: "mcp_catalog_changed", request_type: "mcp_detail" }),
    (request) => mcpPage(request, { catalog_revision: 8, tools: [tool("02")] }),
    (request) => mcpPage(request, { session_evaluated: false, tools: [tool("02", { eligibility: "not_evaluated" })] }),
  ];
  for (const reply of scenarios) {
    const h = mcpFixture(t);
    loadSummary(h);
    h.api.openDetail("files");
    h.api.handleMessage(mcpPage(h.sent.at(-1), { tools: [tool("01")], next_cursor: "page-2" }));
    h.api.loadMoreDetail();
    const page2 = h.sent.at(-1);
    h.api.handleMessage(reply(page2));
    assert.equal(h.state.detail.status, "changed");
    assert.equal(h.state.detail.result, null);
    assert.equal(h.state.inventory.status, "idle");
    assert.equal(h.api.retryDetail(), true);
    const restart = h.sent.at(-1);
    assert.deepEqual(restart.payload, { server_name: "files" });
    h.api.handleMessage(mcpPage(page2, { tools: [tool("02")] }));
    assert.equal(h.state.detail.status, "loading");
    h.api.handleMessage(mcpPage(restart, { catalog_revision: 8, tools: [tool("03")] }));
    assert.deepEqual(h.state.detail.result.tools.map(({ id }) => id), ["03"]);
  }
});

test("MCP detail invalidates immediately on session, policy, connection and pairing changes", (t) => {
  const h = mcpFixture(t);
  loadSummary(h);
  const chat = { id: "session-a", agent_mode: "chat", permission_mode: "ask" };
  h.api.observeSessionPolicy(chat, "session-a");
  h.api.openDetail("files");
  const oldMode = h.sent.at(-1);
  h.api.observeSessionPolicy({ ...chat, title: "A different title" });
  assert.equal(h.state.detail.status, "loading");
  h.api.observeSessionPolicy({ ...chat, permission_mode: "readonly" });
  h.api.handleMessage(mcpPage(oldMode));
  assert.equal(h.state.detail, null);
  h.api.openDetail("files");
  h.api.handleMessage(mcpPage(h.sent.at(-1), { tools: [tool("01")] }));
  h.api.openTool("01");
  h.api.observeSessionPolicy(undefined, "session-remote");
  assert.equal(h.state.detail, null);

  h.api.openDetail("files");
  const oldSession = h.sent.at(-1);
  h.session.current = "session-b";
  h.api.handleMessage(mcpPage(oldSession));
  assert.equal(h.state.detail, null);
  h.api.openDetail("files");
  const oldSocket = h.sent.at(-1);
  assert.equal(oldSocket.session_id, "session-b");
  h.scope.current = { ...h.scope.current, connectionID: "" };
  h.api.handleDisconnect();
  assert.equal(h.state.inventory.status, "idle");
  assert.equal(h.state.detail, null);
  assert.equal(h.api.openDetail("files"), false);
  h.scope.current = { ...h.scope.current, connectionID: "connection-b" };
  h.api.openDetail("files");
  h.api.handleMessage(mcpPage(oldSocket));
  assert.equal(h.state.detail.status, "loading");
  h.scope.current = { pairingID: "pairing-b", connectionID: "connection-b" };
  h.api.handleMessage(mcpPage(h.sent.at(-1)));
  assert.equal(h.state.detail, null);
  assert.equal(h.state.inventory.servers, null);
});

test("MCP detail retains the published snapshot on failed reload but drops it on confirmed reload or a new list revision", (t) => {
  const h = mcpFixture(t);
  loadSummary(h);
  h.api.openDetail("files");
  h.api.handleMessage(mcpPage(h.sent.at(-1), { tools: [tool("old")], next_cursor: "page-2" }));
  h.api.reload();
  h.api.handleMessage(mcpReload(h.sent.at(-1), false));
  assert.equal(h.state.detail.result.tools[0].id, "old");
  h.api.loadMoreDetail();
  const stalePage = h.sent.at(-1);
  h.api.reload();
  h.api.handleMessage(mcpReload(h.sent.at(-1), true));
  assert.equal(h.state.detail, null);
  h.api.handleMessage(mcpPage(stalePage));
  assert.equal(h.state.detail, null);
  const list = h.sent.at(-1);
  assert.equal(list.type, "mcp_list");
  h.api.handleMessage(mcpList(list, summaryWithReasons, 8));
  h.api.openDetail("files");
  const olderFirstPage = h.sent.at(-1);
  loadSummary(h, summaryWithReasons, 9);
  h.api.handleMessage(mcpPage(olderFirstPage, { catalog_revision: 8 }));
  assert.equal(h.state.detail.status, "changed");
  assert.equal(h.state.detail.result, null);
  h.api.retryDetail();
  h.api.handleMessage(mcpPage(h.sent.at(-1), { catalog_revision: 9 }));
  loadSummary(h, summaryWithReasons, 10);
  assert.equal(h.state.detail.status, "changed");
  assert.equal(h.state.detail.result, null);
  loadSummary(h, [mcpServers[1]], 11);
  assert.equal(h.state.detail, null);
});

test("MCP detail supports old summary-only agents and handles silent timeout and transport failure locally", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "Date"] });
  const frames = [];
  const h = mcpFixture(t, { useTransport: true, sessionID: "", send: (raw) => {
    const frame = JSON.parse(raw);
    frames.push(frame);
    if (frame.type === "mcp_detail" && frames.length === 4) return Promise.resolve(false);
  } });
  h.api.refresh();
  h.api.handleMessage(mcpList(h.sent.at(-1)));
  assert.equal(h.state.inventory.status, "ready");
  assert.equal(h.state.inventory.catalogRevision, undefined);
  h.api.openDetail("files");
  assert.equal(Object.hasOwn(frames.at(-1), "session_id"), false);
  h.api.handleMessage(mcpError(h.sent.at(-1), { message: "unsupported client message type: mcp_detail" }));
  assert.equal(h.state.detail.status, "unsupported");
  h.api.openDetail("files");
  const expired = h.sent.at(-1);
  t.mock.timers.tick(15_000);
  assert.equal(h.state.detail.status, "timeout");
  h.api.handleMessage(mcpPage(expired));
  assert.equal(h.state.detail.status, "timeout");
  h.api.retryDetail();
  await tick();
  assert.equal(h.state.detail.status, "error");
  h.api.retryDetail();
  h.api.handleMessage(mcpPage(h.sent.at(-1), { tools: [tool("01", { eligibility: "not_evaluated" })] }));
  assert.equal(h.state.detail.status, "ready");
  assert.equal(h.state.detail.result.session_evaluated, false);
  assert.equal(h.state.inventory.status, "ready");
  assert.deepEqual(h.chatErrors, []);
});

test("MCP detail retains loaded pages after a recoverable pagination failure", (t) => {
  const h = mcpFixture(t);
  loadSummary(h);
  h.api.openDetail("files");
  h.api.handleMessage(mcpPage(h.sent.at(-1), { tools: [tool("01")], next_cursor: "page-2" }));
  h.api.loadMoreDetail();
  h.api.handleMessage(mcpPage(h.sent.at(-1), { tools: [tool("02", { input: { state: "bad" } })] }));
  assert.equal(h.state.detail.status, "error");
  assert.deepEqual(h.state.detail.result.tools.map(({ id }) => id), ["01"]);
  h.api.retryDetail();
  h.api.handleMessage(mcpPage(h.sent.at(-1), { tools: [tool("02", { host_only: true })] }));
  assert.equal(h.state.detail.status, "error");
  assert.deepEqual(h.state.detail.result.tools.map(({ id }) => id), ["01"]);
  h.api.retryDetail();
  assert.equal(h.sent.at(-1).payload.cursor, "page-2");
  h.api.handleMessage(mcpPage(h.sent.at(-1), { tools: [tool("02")] }));
  assert.deepEqual(h.state.detail.result.tools.map(({ id }) => id), ["01", "02"]);
});

const trustService = {
  name: "example", endpoint: "https://mcp.example.invalid:8443/tools/v1", permission: "execute",
  authentication: "headers_configured", trusted: false, runtime: "older",
};

function trustFixture(t, initialConnection = "socket-a") {
  let scope = { pairingID: "pairing-a", connectionID: initialConnection };
  let state;
  const sent = [];
  const api = createMCPTrustRequests({
    getScope: () => scope,
    sendEnvelope: (type, request) => { sent.push({ type, ...request }); return true; },
    onChange: (next) => { state = next; },
  });
  t.after(api.clear);
  return {
    api, sent, get state() { return state; },
    changeScope(connectionID, pairingID = scope.pairingID) { scope = { pairingID, connectionID }; api.syncScope(); },
    reply(payload, request = sent.at(-1)) {
      api.handleMessage({ type: "mcp_trust_result", request_id: request.request_id, payload: { action: request.payload.action, ...payload } });
    },
  };
}

function readyTrust(h) {
  h.api.setActive(true);
  h.reply({ services: [trustService] });
}

function reviewTrust(h) {
  assert.equal(h.api.requestReview(trustService.name), true);
  h.reply({ server_name: trustService.name, review_id: `review-${h.sent.length}`, services: [trustService] });
  return h.state.review.id;
}

test("MCP trust requires a valid capability response on the current connection", (t) => {
  const h = trustFixture(t, "");
  h.api.setActive(true);
  assert.equal(h.sent.length, 0);
  h.changeScope("socket-a");
  assert.deepEqual(h.sent[0].payload, { action: "list" });
  for (const endpoint of ["https://u:secret@host.example/mcp", "https://host.example/mcp?secret", "https://host.example/mcp%2fescaped", "file:///tmp/mcp"]) {
    h.reply({ services: [{ ...trustService, endpoint }] });
    assert.equal(h.state.services, null);
    h.api.refresh();
  }
  h.reply({ code: "mcp_trust_denied" });
  assert.equal(h.state.services, null);
  assert.equal(h.api.requestReview(trustService.name), false);
  h.api.refresh();
  h.reply({}); // Go omits an empty slice; this is still positive capability.
  assert.deepEqual(h.state.services, []);
  h.api.refresh();
  const old = h.sent.at(-1);
  h.changeScope("socket-b");
  h.reply({ services: [trustService] }, old);
  assert.equal(h.state.services, null);
  h.reply({ services: [trustService] });
  assert.deepEqual(h.state.services, [trustService]);
});

test("MCP trust approval uses an explicit matching review and never reloads", (t) => {
  const h = trustFixture(t);
  readyTrust(h);
  assert.equal(h.api.confirm("unreviewed"), false);
  let reviewed = reviewTrust(h);
  h.api.cancelReview();
  assert.equal(h.api.confirm(reviewed), false);
  reviewed = reviewTrust(h);
  assert.equal(h.api.confirm(reviewed), true);
  assert.equal(h.api.confirm(reviewed), false);
  assert.deepEqual(h.sent.at(-1).payload, { action: "approve", server_name: trustService.name, review_id: reviewed });
  h.reply({ services: [{ ...trustService, name: "not-reviewed", trusted: true }] });
  assert.equal(h.state.pendingAction, "approve");
  h.reply({ services: [{ ...trustService, trusted: true }] });
  assert.equal(h.state.services[0].trusted, true);
  assert.equal(h.state.services[0].runtime, "older");
  assert.equal(h.sent.at(-1).payload.action, "approve");
  const runtime = mcpFixture(t, { onReloaded: h.api.afterReload });
  runtime.api.reload();
  runtime.api.handleMessage(mcpReload(runtime.sent.at(-1), false));
  assert.equal(h.sent.at(-1).payload.action, "approve");
  runtime.api.reload();
  runtime.api.handleMessage(mcpReload(runtime.sent.at(-1)));
  assert.deepEqual(h.sent.at(-1).payload, { action: "list" });
  h.reply({ services: [{ ...trustService, trusted: true, runtime: "loaded" }] });
  assert.equal(h.state.services[0].runtime, "loaded");
});

test("MCP trust discards review on settings exit, reload, and socket replacement", (t) => {
  const h = trustFixture(t);
  readyTrust(h);
  let reviewed = reviewTrust(h);
  h.api.setActive(false);
  assert.equal(h.api.confirm(reviewed), false);
  h.api.setActive(true);
  h.reply({ services: [trustService] });
  reviewed = reviewTrust(h);
  h.api.afterReload();
  assert.equal(h.api.confirm(reviewed), false);
  h.reply({ services: [trustService] });
  reviewed = reviewTrust(h);
  h.changeScope("socket-b");
  assert.equal(h.state.review, null);
  assert.equal(h.api.confirm(reviewed), false);
  assert.ok(h.sent.every(({ payload }) => payload.action !== "approve"));
});

test("MCP trust timeout or uncertain reply triggers a fresh list, never another write", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "Date"] });
  const h = trustFixture(t);
  readyTrust(h);
  for (const reason of ["timeout", "mcp_trust_unavailable", "unexpected", "transport"]) {
    assert.equal(h.api.confirm(reviewTrust(h)), true);
    const approve = h.sent.at(-1);
    if (reason === "timeout") t.mock.timers.tick(15_000);
    else if (reason === "transport") h.api.handleMessage({ type: "error", request_id: approve.request_id, payload: { request_type: "mcp_trust", transport_error: true } });
    else h.reply({ code: reason });
    assert.equal(h.state.unclear, true);
    assert.equal(h.state.services, null);
    assert.equal(h.sent.at(-1).payload.action, "list");
    h.reply({ services: [{ ...trustService, trusted: true }] }, approve);
    assert.equal(h.state.pendingAction, "list");
    h.reply({ services: [trustService] });
    assert.equal(h.state.services[0].trusted, false);
    assert.equal(h.state.unclear, true);
  }
  assert.deepEqual(h.sent.map(({ payload }) => payload.action), ["list", ...Array(4).fill(["review", "approve", "list"]).flat()]);
});

test("MCP trust denial and re-review are known non-writes", (t) => {
  const h = trustFixture(t);
  readyTrust(h);
  h.api.confirm(reviewTrust(h));
  h.reply({ code: "mcp_trust_review_required" });
  assert.equal(h.state.unclear, false);
  assert.match(h.state.notice, /重新查看/);
  assert.equal(h.sent.at(-1).payload.action, "approve");
  h.api.confirm(reviewTrust(h));
  h.reply({ code: "mcp_trust_denied" });
  assert.equal(h.state.unclear, false);
  assert.equal(h.state.services, null);
  assert.equal(h.sent.at(-1).payload.action, "list");
  h.reply({ code: "mcp_trust_denied" });
  assert.equal(h.state.services, null);
});

test("MCP trust disconnect waits for a new socket to reconcile and re-pair drops the old outcome", (t) => {
  const h = trustFixture(t);
  readyTrust(h);
  h.api.confirm(reviewTrust(h));
  const approve = h.sent.at(-1);
  h.changeScope("");
  assert.equal(h.state.unclear, true);
  h.api.setActive(false);
  assert.equal(h.sent.at(-1), approve);
  h.changeScope("socket-b");
  assert.equal(h.sent.at(-1).payload.action, "list");
  h.reply({ services: [{ ...trustService, trusted: true }] }, approve);
  assert.equal(h.state.services, null);
  h.reply({ services: [{ ...trustService, trusted: true }] });
  assert.equal(h.state.services[0].trusted, true);
  h.changeScope("socket-c", "pairing-b");
  assert.equal(h.state.unclear, false);
  assert.equal(h.state.services, null);
  assert.deepEqual(h.sent.map(({ payload }) => payload.action), ["list", "review", "approve", "list"]);
});

test("narrow MCP settings interaction opens plain-text tool details, goes back, and paginates", (t) => {
  let width = 320;
  const jsx = (type, props) => ({ type, props });
  const stubs = {
    "react/jsx-runtime": { jsx, jsxs: jsx, Fragment: "Fragment" },
    "react-native": {
      Pressable: "Pressable", Text: "Text", View: "View", useWindowDimensions: () => ({ width }),
      StyleSheet: { create: (styles) => styles }, Platform: { select: (choices) => choices.web ?? choices.default },
    },
    "@expo/vector-icons": { Feather: "Feather" },
    "../../common/ButtonContent": { ButtonContent: "ButtonContent" },
    "../../common/ConfirmModal": { ConfirmModal: "ConfirmModal" },
    "../../../theme/ThemeProvider": { useAppTheme: () => ({ palette: {} }), useThemedStyles: (create) => create({ palette: {} }) },
  };
  let MCPCatalogSection;
  try {
    Module._load = (name, ...args) => stubs[name] ?? originalLoad.call(Module, name, ...args);
    ({ MCPCatalogSection } = require("../src/components/settings/sections/MCPCatalogSection.tsx"));
  } finally {
    Module._load = originalLoad;
  }
  function nodes(value) {
    if (Array.isArray(value)) return value.flatMap(nodes);
    if (!value || typeof value !== "object") return [];
    if (typeof value.type === "function") return nodes(value.type(value.props));
    return [value, ...nodes(value.props.children)];
  }
  function text(value) {
    if (Array.isArray(value)) return value.map(text).join(" ");
    if (typeof value === "string" || typeof value === "number") return String(value);
    if (!value || typeof value !== "object") return "";
    if (typeof value.type === "function") return text(value.type(value.props));
    return value.type === "ButtonContent" ? value.props.text : text(value.props.children);
  }
  const h = mcpFixture(t);
  const trust = trustFixture(t);
  loadSummary(h);
  const render = (connected = true) => MCPCatalogSection({ ...h.api, ...h.state, trust: { ...trust.api, ...trust.state }, connected, buttonFeedback: (style) => style });
  const button = (tree, label) => nodes(tree).find((node) => node.props.accessibilityRole === "button" &&
    (node.props.accessibilityLabel === label || text(node).includes(label)));

  assert.match(text(render()), /待信任审批/);
  assert.match(text(render(false)), /显示上次获取的 MCP 列表/);
  assert.equal(button(render(false), "files"), undefined);
  button(render(), "search").props.onPress();
  h.api.handleMessage(mcpPage(h.sent.at(-1), { session_evaluated: false }));
  assert.match(text(render()), /此服务没有已发现的工具/);
  assert.match(text(render()), /未评估会话资格/);
  h.api.backFromDetail();

  button(render(), "files").props.onPress();
  const name = "long_tool_name_".repeat(10);
  const field = "param_name_".repeat(8);
  h.api.handleMessage(mcpPage(h.sent.at(-1), {
    tools: [tool("01", {
      name, title: "Tool A", alias: "mcp_files_tool", description: "<b>Plain description</b>", metadata_truncated: true,
      eligibility: "filtered", reason: "permission_mode",
      input: { state: "fields", fields: [{ name: field, type: "string", required: true }], truncated: true },
      output: { state: "unsupported" },
    })], next_cursor: "page-2",
  }));
  const narrowRow = button(render(), name);
  assert.equal(narrowRow.props.style({ pressed: false }).some((s) => s?.flexBasis === "48%"), false);
  width = 900;
  assert.equal(button(render(), name).props.style({ pressed: false }).some((s) => s?.flexBasis === "48%"), true);
  width = 320;
  narrowRow.props.onPress();
  const detail = render();
  assert.ok(text(detail).includes(name));
  assert.ok(text(detail).includes(field));
  assert.match(text(detail), /必填/);
  assert.match(text(detail), /模型别名：mcp_files_tool/);
  assert.match(text(detail), /会话权限模式限制/);
  assert.match(text(detail), /<b>Plain description<\/b>/);
  assert.match(text(detail), /结构无法简要展示/);
  assert.match(text(detail), /内容已截断|信息已截断/);
  assert.equal(button(detail, "加载更多工具"), undefined);
  assert.equal(nodes(detail).some((node) => node.type === "Text" && node.props.numberOfLines && text(node).includes(name)), false);
  h.api.backFromDetail();
  button(render(), "加载更多工具").props.onPress();
  assert.deepEqual(button(render(), "加载更多工具").props.accessibilityState, { busy: true, disabled: true });
  h.api.handleMessage(mcpPage(h.sent.at(-1), {
    tools: [tool("02", { name: "Internal", host_only: true, eligibility: "host_only" })],
  }));
  assert.match(text(render()), /仅 Agent 内部使用/);
  assert.ok(button(render(), name));
  h.api.backFromDetail();
  loadSummary(h, []);
  assert.match(text(render()), /没有已配置的 MCP 服务/);
  assert.doesNotMatch(text(render()), /HTTP MCP 信任审批/);
  readyTrust(trust);
  assert.match(text(render()), /未信任 · 运行时：已加载旧配置/);
  assert.ok(text(render()).includes(trustService.endpoint));
  assert.doesNotMatch(text(render(false)), /HTTP MCP 信任审批/);
  button(render(), "查看并审批：example").props.onPress();
  trust.reply({ services: [trustService], review_id: "review-ui", server_name: trustService.name });
  const modal = nodes(render()).find(({ type }) => type === "ConfirmModal");
  assert.ok(modal.props.message.includes(trustService.endpoint));
  assert.equal(trust.sent.at(-1).payload.action, "review");
  modal.props.onConfirm();
  assert.deepEqual(trust.sent.at(-1).payload, { action: "approve", server_name: trustService.name, review_id: "review-ui" });
});
