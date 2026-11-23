const assert = require("node:assert/strict");
const { test } = require("node:test");
const fs = require("node:fs");
const ts = require("typescript");

// Use the project's existing TypeScript compiler to run pure utility modules.
// No Expo runtime, network service, test framework or additional dependency.
require.extensions[".ts"] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    fileName: filename,
  });
  module._compile(outputText, filename);
};
const {
  createAssetUploadClient, AssetUploadCanceledError, AssetUploadDisconnectedError,
  assetUploadChunkSize: chunkSize, assetUploadMaxFileBytes: maxBytes,
  assetUploadCompleteTimeoutMs: completeTimeoutMs,
} = require("../src/utils/assetUpload.ts");
const { messageWithAttachedFiles, parseAttachedFiles, attachmentKey } = require("../src/utils/attachments.ts");
const { normalizeUploadedAsset, parseSharedAssets, assetSummaryToSharedAsset } = require("../src/utils/toolAssets.ts");

const metadata = (size) => ({ asset_id: "asset-1", file_name: 'report & "draft".txt', content_type: "text/plain", size });
const tick = () => new Promise((resolve) => setImmediate(resolve));
function harness(onSend, { requestTimeoutMs = 50, completeTimeoutMs: completion = 50 } = {}) {
  const sent = [];
  const owners = [];
  const reads = [];
  let scope = { pairingID: "pairing-1", connectionID: "connection-1" };
  let closed = 0;
  const client = createAssetUploadClient({
    getScope: () => scope,
    requestTimeoutMs,
    completeTimeoutMs: completion,
    sendEnvelope(type, envelope) {
      const message = { type, ...envelope };
      sent.push(message);
      owners.push(scope.pairingID);
      onSend?.(message, reply, client);
      return true;
    },
  });
  function reply(message, changes = {}) {
    const operation = message.type.replace("asset_upload_", "");
    const offset = operation === "chunk" ? message.payload.offset + Buffer.from(message.payload.data, "base64").length : 0;
    client.handleMessage({ type: "asset_upload_result", request_id: message.request_id, payload: {
      operation, upload_id: message.payload.upload_id || "upload-1", offset,
      chunk_size: chunkSize, max_file_bytes: maxBytes, ...changes,
    } });
  }
  function source(size) {
    return {
      fileName: metadata(size).file_name, contentType: "text/plain", size,
      async read(offset, length) {
        reads.push([offset, length]);
        return Uint8Array.from({ length }, (_, index) => (offset + index) % 251);
      },
      close() { closed++; },
    };
  }
  return {
    client, sent, owners, reads, reply, source,
    upload(size, sessionID, signal) { return client.upload({ source: source(size), scope: { ...scope }, sessionID, signal }); },
    setScope(pairingID, connectionID) { scope = { pairingID, connectionID }; },
    get closed() { return closed; },
  };
}

test("256 KiB chunks preserve bytes, offsets, envelope session and independent request IDs", async () => {
  const size = chunkSize * 2 + 7;
  const h = harness((message, reply) => {
    reply(message, message.type === "asset_upload_complete" ? { offset: size, asset: metadata(size) } : {});
  });
  const result = await h.upload(size, "session-1");
  assert.equal(result.asset_id, "asset-1");
  assert.equal(result.short_url, undefined);
  assert.deepEqual(h.reads, [[0, chunkSize], [chunkSize, chunkSize], [chunkSize * 2, 7]]);
  assert.equal(h.closed, 1);
  assert.equal(new Set(h.sent.map((message) => message.request_id)).size, h.sent.length);
  assert.ok(h.sent.every((message) => message.session_id === "session-1"));
  assert.deepEqual(Object.keys(h.sent[0].payload).sort(), ["content_type", "file_name", "idempotency_key", "size"]);
  for (const message of h.sent.filter((message) => message.type === "asset_upload_chunk")) {
    assert.deepEqual(Object.keys(message.payload).sort(), ["data", "offset", "upload_id"]);
    const bytes = Buffer.from(message.payload.data, "base64");
    assert.ok(bytes.length <= chunkSize);
    for (let index = 0; index < bytes.length; index++) assert.equal(bytes[index], (message.payload.offset + index) % 251);
  }
});

test("readiness errors stop before reading bytes or sending chunks and use local guidance", async () => {
  for (const [code, expected] of [
    ["upload_service_unavailable", /检查 Agent 的 short_links 服务/],
    ["upload_missing_tool", /缺少附件上传工具/],
  ]) {
    const h = harness((message, _reply, client) => {
      client.handleMessage({
        type: "error", request_id: message.request_id, session_id: message.session_id,
        payload: { request_type: message.type, code, message: "upstream details" },
      });
    });
    await assert.rejects(h.upload(1, "s"), expected);
    assert.deepEqual(h.sent.map(({ type }) => type), ["asset_upload_begin"]);
    assert.deepEqual(h.reads, []);
    assert.equal(h.closed, 1);
  }
});

test("a coded upstream failure stays on the upload and does not display its original message", async () => {
  const h = harness((message, reply, client) => {
    if (message.type !== "asset_upload_complete") return reply(message);
    client.handleMessage({
      type: "error", request_id: message.request_id, session_id: message.session_id,
      payload: { request_type: message.type, code: "upload_upstream_failure", message: "upstream details" },
    });
  });
  await assert.rejects(h.upload(1, "s"), /short_links 服务未完成附件上传/);
  assert.equal(h.sent.filter(({ type }) => type === "asset_upload_complete").length, 1);
});

test("boolean send and heartbeat cannot acknowledge upload; only matching result advances", async () => {
  const h = harness();
  const upload = h.upload(1, "s");
  assert.equal(h.client.handleMessage({ type: "heartbeat", request_id: h.sent[0].request_id }), false);
  h.reply({ ...h.sent[0], request_id: "another-request" });
  await tick();
  assert.equal(h.sent.length, 1);
  h.reply(h.sent[0]);
  await tick();
  assert.equal(h.sent[1].type, "asset_upload_chunk");
  h.reply(h.sent[1]);
  await tick();
  assert.equal(h.sent[2].type, "asset_upload_complete");
  h.reply(h.sent[2], { offset: 1, asset: metadata(1) });
  assert.equal((await upload).size, 1);
});

test("concurrent uploads resolve by request ID even when replies arrive out of order", async () => {
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_chunk") reply(message);
  });
  const a = h.upload(1, "a");
  const b = h.upload(1, "b");
  h.reply(h.sent[1], { upload_id: "upload-b" });
  h.reply(h.sent[0], { upload_id: "upload-a" });
  await tick();
  for (const message of h.sent.filter((message) => message.type === "asset_upload_complete")) {
    h.reply(message, { offset: 1, asset: { ...metadata(1), asset_id: `asset-${message.session_id}` } });
  }
  assert.equal((await a).asset_id, "asset-a");
  assert.equal((await b).asset_id, "asset-b");
});

test("empty and oversize files reject locally before any send or read and close the source", async () => {
  for (const [size, expected] of [[0, /非空文件/], [maxBytes + 1, /50 MiB/]]) {
    const h = harness();
    await assert.rejects(h.upload(size, "s"), expected);
    assert.deepEqual(h.sent, []);
    assert.deepEqual(h.reads, []);
    assert.equal(h.closed, 1);
  }
});

test("upload errors are consumed by correlation; unrelated errors fall through", async () => {
  const h = harness();
  const failed = assert.rejects(h.upload(1, "s"), /rejected/);
  assert.equal(h.client.handleMessage({ type: "error", request_id: "chat-request", payload: { message: "chat error" } }), false);
  assert.equal(h.client.handleMessage({ type: "error", request_id: h.sent[0].request_id, payload: { message: "rejected" } }), true);
  await failed;
  assert.equal(h.client.handleMessage({ type: "error", request_id: h.sent[0].request_id, payload: { message: "late" } }), true);
});

test("mismatched operation/upload ID is rejected and known upload is canceled once", async () => {
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_begin" || message.type === "asset_upload_cancel") reply(message);
  });
  const failed = assert.rejects(h.upload(1, "s"), /无效/);
  await tick();
  h.reply(h.sent.find((message) => message.type === "asset_upload_chunk"), { upload_id: "wrong-upload" });
  await failed;
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_cancel").length, 1);
});

test("cancel while waiting for begin rejects promptly and cancels the late upload result", async () => {
  const controller = new AbortController();
  const h = harness((message, reply) => { if (message.type === "asset_upload_cancel") reply(message); });
  const failed = assert.rejects(h.upload(1, "s", controller.signal), AssetUploadCanceledError);
  controller.abort();
  await failed;
  h.reply(h.sent[0]);
  await tick();
  const cancels = h.sent.filter((message) => message.type === "asset_upload_cancel");
  assert.equal(cancels.length, 1);
  assert.deepEqual(cancels[0].payload, { upload_id: "upload-1" });
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_complete").length, 0);
});

test("disconnect rejects pending uploads and reconnect sends best-effort cancel without restarting", async () => {
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_begin" || message.type === "asset_upload_cancel") reply(message);
  });
  const failed = assert.rejects(h.upload(2, "s"), AssetUploadDisconnectedError);
  await tick();
  h.setScope("pairing-1", "");
  h.client.disconnect();
  await failed;
  h.setScope("pairing-1", "connection-2");
  h.client.reconnect();
  await tick();
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_cancel").length, 1);
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_begin").length, 1);
});

test("begin timeout retries the same idempotency key with a new request ID", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let begins = 0;
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_begin" && ++begins === 1) return;
    reply(message, message.type === "asset_upload_complete" ? { offset: 1, asset: metadata(1) } : {});
  });
  const upload = h.upload(1, "s");
  t.mock.timers.tick(50);
  await upload;
  const beginMessages = h.sent.filter((message) => message.type === "asset_upload_begin");
  assert.equal(beginMessages.length, 2);
  assert.equal(beginMessages[0].payload.idempotency_key, beginMessages[1].payload.idempotency_key);
  assert.notEqual(beginMessages[0].request_id, beginMessages[1].request_id);
});

test("chunk timeout asks status and does not resend bytes already accepted", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const size = 3;
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_chunk") return;
    if (message.type === "asset_upload_status") reply(message, { offset: size });
    else reply(message, message.type === "asset_upload_complete" ? { offset: size, asset: metadata(size) } : {});
  });
  const upload = h.upload(size, "s");
  await tick();
  t.mock.timers.tick(50);
  await upload;
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_chunk").length, 1);
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_status").length, 1);
});

test("uncertain complete recovers asset via status without a second backend create", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const size = 2;
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_complete") return;
    reply(message, message.type === "asset_upload_status" ? { offset: size, asset: metadata(size) } : {});
  });
  const upload = h.upload(size, "s");
  await tick();
  t.mock.timers.tick(50);
  await upload;
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_complete").length, 1);
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_begin").length, 1);
});

test("timeout exhausts bounded recovery and rejects rather than waiting on heartbeat", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const h = harness();
  const failed = assert.rejects(h.upload(1, "s"), /超时/);
  t.mock.timers.tick(50);
  await tick();
  t.mock.timers.tick(50);
  await failed;
  assert.equal(h.sent.length, 2);
});

test("complete waits through the Agent's five-minute worker window", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const h = harness((message, reply) => {
    if (message.type !== "asset_upload_complete") reply(message);
  }, { completeTimeoutMs });
  const upload = h.upload(1, "s");
  await tick();
  t.mock.timers.tick(300_000);
  await tick();
  assert.deepEqual(h.sent.map(({ type }) => type), ["asset_upload_begin", "asset_upload_chunk", "asset_upload_complete"]);
  h.reply(h.sent[2], { offset: 1, asset: metadata(1) });
  assert.equal((await upload).asset_id, "asset-1");
});

test("completion recovery has a bounded same-upload rejoin, even if replies never finish", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const h = harness((message, reply) => {
    if (message.type !== "asset_upload_complete") {
      reply(message, message.type === "asset_upload_status" ? { offset: 1 } : {});
    }
  });
  const failed = assert.rejects(h.upload(1, "s"), /超时/);
  await tick();
  t.mock.timers.tick(50);
  await tick();
  t.mock.timers.tick(50);
  await failed;
  assert.deepEqual(h.sent.map(({ type }) => type), [
    "asset_upload_begin", "asset_upload_chunk", "asset_upload_complete", "asset_upload_status",
    "asset_upload_complete", "asset_upload_cancel",
  ]);
  assert.ok(h.sent.slice(2).every(({ payload }) => payload.upload_id === "upload-1"));
});

test("disconnect after a completion reply, before await resumes, never delivers the asset", async () => {
  const h = harness((message, reply) => {
    if (message.type !== "asset_upload_complete") reply(message);
  });
  const failed = assert.rejects(h.upload(1, "s"), AssetUploadDisconnectedError);
  await tick();
  h.reply(h.sent[2], { offset: 1, asset: metadata(1) });
  h.setScope("pairing-1", "");
  h.client.disconnect();
  await failed;
  assert.equal(h.closed, 1);
});

test("cancel after a status reply, before await resumes, does not return the asset or send complete", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_begin" || message.type === "asset_upload_cancel") reply(message);
  });
  const controller = new AbortController();
  const failed = assert.rejects(h.upload(1, "s", controller.signal), AssetUploadCanceledError);
  await tick();
  t.mock.timers.tick(50);
  await tick();
  assert.equal(h.sent[2].type, "asset_upload_status");
  h.reply(h.sent[2], { offset: 1, asset: metadata(1) });
  controller.abort();
  await failed;
  assert.deepEqual(h.sent.map(({ type }) => type), [
    "asset_upload_begin", "asset_upload_chunk", "asset_upload_status", "asset_upload_cancel",
  ]);
});

test("an old begin reply can only schedule cleanup on a new connection of the same pairing", async () => {
  const h = harness((message, reply) => { if (message.type === "asset_upload_cancel") reply(message); });
  const failed = assert.rejects(h.upload(1, "origin-session"), AssetUploadDisconnectedError);
  h.setScope("pairing-1", "");
  h.client.disconnect();
  await failed;
  h.setScope("pairing-1", "connection-2");
  h.reply(h.sent[0]);
  assert.equal(h.sent[1].type, "asset_upload_cancel");
  assert.equal(h.sent[1].session_id, "origin-session");
  assert.deepEqual(h.owners, ["pairing-1", "pairing-1"]);
  assert.deepEqual(h.reads, []);
});

test("re-pair before the awaited begin continuation or after disconnect never sends old data or cleanup", async () => {
  const h = harness();
  const failed = assert.rejects(h.upload(1, "origin-session"), AssetUploadCanceledError);
  h.reply(h.sent[0]);
  h.setScope("pairing-2", "connection-2");
  h.client.reconnect();
  await failed;
  assert.deepEqual(h.sent.map(({ type }) => type), ["asset_upload_begin"]);
  assert.deepEqual(h.reads, []);

  const late = harness((message, reply) => { if (message.type === "asset_upload_begin") reply(message); });
  const lateFailure = assert.rejects(late.upload(1, "origin-session"), AssetUploadDisconnectedError);
  await tick();
  late.setScope("pairing-1", "");
  late.client.disconnect();
  await lateFailure;
  late.setScope("pairing-2", "connection-2");
  late.client.reconnect();
  late.reply(late.sent[0]);
  assert.deepEqual(late.owners, ["pairing-1", "pairing-1"]);
});

test("native enqueue failures are distinguished from cancellation and Agent errors", async () => {
  const h = harness();
  const failed = assert.rejects(h.upload(1, "s"), AssetUploadDisconnectedError);
  assert.equal(h.client.handleMessage({ type: "error", request_id: h.sent[0].request_id, payload: {
    request_type: "asset_upload_begin", transport_error: true,
  } }), true);
  await failed;
});

test("attachment metadata roundtrip keeps only asset ID, name, type and size in model text", () => {
  const asset = { ...metadata(0), kind: "uploaded_asset", local_uri: "file:///private/photo.jpg", short_url: "https://files.example/once", code: "one-use-code", checksum: "sha256:test", max_visits: 1 };
  const content = messageWithAttachedFiles("inspect", [asset]);
  assert.match(content, /\[User attachment\]/);
  assert.doesNotMatch(content, /file:\/|https:\/|one-use-code|sha256|local_uri/);
  const [parsed] = parseAttachedFiles(content);
  assert.deepEqual(parsed, { ...metadata(0), kind: "uploaded_asset" });
  assert.equal(attachmentKey(parsed), "asset:asset-1");
  assert.deepEqual(parseAttachedFiles('<uploaded_file name="old" short_url="https://files.example/old">old</uploaded_file>'), []);
});

test("canonical metadata whitelist and source-specific structured asset cards need no public URL", () => {
  const value = { ...metadata(5), business_id: "business-1", checksum: "sha256:abc", max_visits: 1, bucket: "private", object_key: "secret", api_key: "secret" };
  const normalized = normalizeUploadedAsset(value);
  assert.equal(normalized.asset_id, "asset-1");
  assert.equal(normalized.checksum, "sha256:abc");
  assert.equal(normalized.max_visits, 1);
  assert.equal(normalized.bucket, undefined);
  assert.equal(normalized.api_key, undefined);
  assert.equal(parseSharedAssets("share_file", JSON.stringify(value))[0].assetID, "asset-1");
  assert.equal(parseSharedAssets("mcp__short_links__get_asset_info", JSON.stringify(value)).length, 1);
  assert.equal(parseSharedAssets("create_asset_shortlink", JSON.stringify({ files: [value, { ...value, asset_id: "asset-2" }] })).length, 2);
  assert.deepEqual(parseSharedAssets("shell", JSON.stringify(value)), []);
  assert.deepEqual(parseSharedAssets("share_file", "Some URL: https://files.example/once"), []);
  assert.equal(assetSummaryToSharedAsset({ id: "record-1", ...value }).shortURL, undefined);
  assert.equal(normalizeUploadedAsset({ ...value, short_url: "javascript:alert(1)" }).short_url, undefined);
});
