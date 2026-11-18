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
  createAssetUploadClient, AssetUploadCanceledError,
  assetUploadChunkSize: chunkSize, assetUploadMaxFileBytes: maxBytes,
} = require("../src/utils/assetUpload.ts");
const { messageWithAttachedFiles, parseAttachedFiles, attachmentKey } = require("../src/utils/attachments.ts");
const { normalizeUploadedAsset, parseSharedAssets, assetSummaryToSharedAsset } = require("../src/utils/toolAssets.ts");

const metadata = (size) => ({ asset_id: "asset-1", file_name: 'report & "draft".txt', content_type: "text/plain", size });
const tick = () => new Promise((resolve) => setImmediate(resolve));
function harness(onSend, timeout = 50) {
  const sent = [];
  const reads = [];
  let connected = true;
  let closed = 0;
  const client = createAssetUploadClient({
    isConnected: () => connected,
    requestTimeoutMs: timeout,
    sendEnvelope(type, envelope) {
      const message = { type, ...envelope };
      sent.push(message);
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
  return { client, sent, reads, reply, source, setConnected(value) { connected = value; }, get closed() { return closed; } };
}

test("256 KiB chunks preserve bytes, offsets, envelope session and independent request IDs", async () => {
  const size = chunkSize * 2 + 7;
  const h = harness((message, reply) => {
    reply(message, message.type === "asset_upload_complete" ? { offset: size, asset: metadata(size) } : {});
  });
  const result = await h.client.upload({ source: h.source(size), sessionID: "session-1" });
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

test("boolean send and heartbeat cannot acknowledge upload; only matching result advances", async () => {
  const h = harness();
  const upload = h.client.upload({ source: h.source(1), sessionID: "s" });
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
  const a = h.client.upload({ source: h.source(1), sessionID: "a" });
  const b = h.client.upload({ source: h.source(1), sessionID: "b" });
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
    await assert.rejects(h.client.upload({ source: h.source(size), sessionID: "s" }), expected);
    assert.deepEqual(h.sent, []);
    assert.deepEqual(h.reads, []);
    assert.equal(h.closed, 1);
  }
});

test("upload errors are consumed by correlation; unrelated errors fall through", async () => {
  const h = harness();
  const failed = assert.rejects(h.client.upload({ source: h.source(1), sessionID: "s" }), /rejected/);
  assert.equal(h.client.handleMessage({ type: "error", request_id: "chat-request", payload: { message: "chat error" } }), false);
  assert.equal(h.client.handleMessage({ type: "error", request_id: h.sent[0].request_id, payload: { message: "rejected" } }), true);
  await failed;
  assert.equal(h.client.handleMessage({ type: "error", request_id: h.sent[0].request_id, payload: { message: "late" } }), true);
});

test("mismatched operation/upload ID is rejected and known upload is canceled once", async () => {
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_begin" || message.type === "asset_upload_cancel") reply(message);
  });
  const failed = assert.rejects(h.client.upload({ source: h.source(1), sessionID: "s" }), /无效/);
  await tick();
  h.reply(h.sent.find((message) => message.type === "asset_upload_chunk"), { upload_id: "wrong-upload" });
  await failed;
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_cancel").length, 1);
});

test("cancel while waiting for begin rejects promptly and cancels the late upload result", async () => {
  const controller = new AbortController();
  const h = harness((message, reply) => { if (message.type === "asset_upload_cancel") reply(message); });
  const failed = assert.rejects(h.client.upload({ source: h.source(1), sessionID: "s", signal: controller.signal }), AssetUploadCanceledError);
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
  const failed = assert.rejects(h.client.upload({ source: h.source(2), sessionID: "s" }), /断开/);
  await tick();
  h.setConnected(false);
  h.client.disconnect();
  await failed;
  h.setConnected(true);
  h.client.reconnect();
  await tick();
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_cancel").length, 1);
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_begin").length, 1);
});

test("begin timeout retries the same idempotency key with a new request ID", async () => {
  let begins = 0;
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_begin" && ++begins === 1) return;
    reply(message, message.type === "asset_upload_complete" ? { offset: 1, asset: metadata(1) } : {});
  }, 15);
  await h.client.upload({ source: h.source(1), sessionID: "s" });
  const beginMessages = h.sent.filter((message) => message.type === "asset_upload_begin");
  assert.equal(beginMessages.length, 2);
  assert.equal(beginMessages[0].payload.idempotency_key, beginMessages[1].payload.idempotency_key);
  assert.notEqual(beginMessages[0].request_id, beginMessages[1].request_id);
});

test("chunk timeout asks status and does not resend bytes already accepted", async () => {
  const size = 3;
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_chunk") return;
    if (message.type === "asset_upload_status") reply(message, { offset: size });
    else reply(message, message.type === "asset_upload_complete" ? { offset: size, asset: metadata(size) } : {});
  }, 15);
  await h.client.upload({ source: h.source(size), sessionID: "s" });
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_chunk").length, 1);
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_status").length, 1);
});

test("uncertain complete recovers asset via status without a second backend create", async () => {
  const size = 2;
  const h = harness((message, reply) => {
    if (message.type === "asset_upload_complete") return;
    reply(message, message.type === "asset_upload_status" ? { offset: size, asset: metadata(size) } : {});
  }, 15);
  await h.client.upload({ source: h.source(size), sessionID: "s" });
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_complete").length, 1);
  assert.equal(h.sent.filter((message) => message.type === "asset_upload_begin").length, 1);
});

test("timeout exhausts bounded recovery and rejects rather than waiting on heartbeat", async () => {
  const h = harness(undefined, 15);
  await assert.rejects(h.client.upload({ source: h.source(1), sessionID: "s" }), /超时/);
  assert.equal(h.sent.length, 2);
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
