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
