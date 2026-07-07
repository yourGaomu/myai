import AsyncStorage from "@react-native-async-storage/async-storage";
import { Platform } from "react-native";

import type { SessionHistoryMessage, SessionHistoryMetaPayload } from "../protocol";

type SQLiteModule = typeof import("expo-sqlite");
type SQLiteDatabase = Awaited<ReturnType<SQLiteModule["openDatabaseAsync"]>>;

type MessageRow = {
  content: string | null;
  created_at: string | null;
  message_id: string;
  reasoning: string | null;
  role: string;
  tool_arguments: string | null;
  tool_call_id: string | null;
  tool_error: string | null;
  tool_name: string | null;
  usage_json: string | null;
};

type MetaRow = {
  message_count: number;
};

const webHistoryCachePrefix = "myai:session_history:";

let dbPromise: Promise<SQLiteDatabase> | null = null;
let sqlitePromise: Promise<SQLiteModule> | null = null;

export async function loadCachedSessionHistory(sessionID: string) {
  const id = sessionID.trim();
  if (!id) {
    return { messages: [], meta: emptyMeta("") };
  }

  if (useWebStorageCache()) {
    const messages = await loadWebCachedMessages(id);
    return {
      messages,
      meta: metaFromMessages(id, messages),
    };
  }

  const db = await database();
  const rows = await db.getAllAsync<MessageRow>(
    `SELECT message_id, role, content, reasoning, tool_call_id, tool_name, tool_arguments, tool_error, usage_json, created_at
     FROM session_messages
     WHERE session_id = ?
     ORDER BY created_at ASC, rowid ASC`,
    id,
  );
  const messages = rows.map(rowToMessage);
  return {
    messages,
    meta: metaFromMessages(id, messages),
  };
}

export async function replaceCachedSessionHistory(sessionID: string, messages: SessionHistoryMessage[]) {
  const id = sessionID.trim();
  if (!id) {
    return;
  }

  if (useWebStorageCache()) {
    await AsyncStorage.setItem(webHistoryCacheKey(id), JSON.stringify(messages));
    return;
  }

  const db = await database();
  await db.withTransactionAsync(async () => {
    await db.runAsync("DELETE FROM session_messages WHERE session_id = ?", id);
    for (const message of messages) {
      await upsertMessage(db, id, message);
    }
  });
}

export async function appendCachedSessionHistory(sessionID: string, messages: SessionHistoryMessage[]) {
  const id = sessionID.trim();
  if (!id || messages.length === 0) {
    return;
  }

  if (useWebStorageCache()) {
    const current = await loadWebCachedMessages(id);
    await AsyncStorage.setItem(webHistoryCacheKey(id), JSON.stringify(mergeHistoryMessages(current, messages)));
    return;
  }

  const db = await database();
  await db.withTransactionAsync(async () => {
    for (const message of messages) {
      await upsertMessage(db, id, message);
    }
  });
}

export async function getCachedSessionHistoryMeta(sessionID: string): Promise<SessionHistoryMetaPayload> {
  const id = sessionID.trim();
  if (!id) {
    return emptyMeta("");
  }

  if (useWebStorageCache()) {
    return metaFromMessages(id, await loadWebCachedMessages(id));
  }

  const db = await database();
  const countRows = await db.getAllAsync<MetaRow>(
    "SELECT COUNT(*) as message_count FROM session_messages WHERE session_id = ?",
    id,
  );
  const lastRows = await db.getAllAsync<MessageRow>(
    `SELECT message_id, role, content, reasoning, tool_call_id, tool_name, tool_arguments, tool_error, usage_json, created_at
     FROM session_messages
     WHERE session_id = ?
     ORDER BY created_at DESC, rowid DESC
     LIMIT 1`,
    id,
  );
  const messageCount = countRows[0]?.message_count || 0;
  const last = lastRows[0];
  const meta: SessionHistoryMetaPayload = {
    session_id: id,
    local_message_count: messageCount,
    local_last_message_id: last?.message_id || "",
    local_history_version: messageCount,
  };
  if (last?.created_at) {
    meta.local_last_message_created_at = last.created_at;
  }
  return meta;
}

async function database() {
  if (useWebStorageCache()) {
    throw new Error("SQLite history cache is disabled on web");
  }
  if (!dbPromise) {
    dbPromise = loadSQLite().then(async (SQLite) => {
      const db = await SQLite.openDatabaseAsync("myai_session_history.db");
      await db.execAsync(`
        PRAGMA journal_mode = WAL;
        CREATE TABLE IF NOT EXISTS session_messages (
          session_id TEXT NOT NULL,
          message_id TEXT NOT NULL,
          role TEXT NOT NULL,
          content TEXT,
          reasoning TEXT,
          tool_call_id TEXT,
          tool_name TEXT,
          tool_arguments TEXT,
          tool_error TEXT,
          usage_json TEXT,
          created_at TEXT,
          PRIMARY KEY (session_id, message_id)
        );
        CREATE INDEX IF NOT EXISTS idx_session_messages_order
          ON session_messages(session_id, created_at, message_id);
      `);
      return db;
    });
  }
  return dbPromise;
}

function loadSQLite() {
  if (!sqlitePromise) {
    sqlitePromise = import("expo-sqlite");
  }
  return sqlitePromise;
}

async function upsertMessage(db: SQLiteDatabase, sessionID: string, message: SessionHistoryMessage) {
  const messageID = message.id?.trim();
  if (!messageID) {
    return;
  }

  await db.runAsync(
    `INSERT OR REPLACE INTO session_messages (
      session_id, message_id, role, content, reasoning, tool_call_id, tool_name,
      tool_arguments, tool_error, usage_json, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    sessionID,
    messageID,
    message.role || "event",
    message.content || "",
    message.reasoning || "",
    message.tool_call_id || "",
    message.tool_name || "",
    message.tool_arguments || "",
    message.tool_error || "",
    JSON.stringify(message.usage || {}),
    message.created_at || "",
  );
}

function useWebStorageCache() {
  return Platform.OS === "web";
}

function webHistoryCacheKey(sessionID: string) {
  return `${webHistoryCachePrefix}${sessionID}`;
}

async function loadWebCachedMessages(sessionID: string): Promise<SessionHistoryMessage[]> {
  const raw = await AsyncStorage.getItem(webHistoryCacheKey(sessionID));
  return parseCachedMessages(raw);
}

function parseCachedMessages(raw: string | null): SessionHistoryMessage[] {
  if (!raw) {
    return [];
  }
  try {
    const value = JSON.parse(raw);
    if (!Array.isArray(value)) {
      return [];
    }
    return value.filter(isHistoryMessage);
  } catch {
    return [];
  }
}

function isHistoryMessage(value: unknown): value is SessionHistoryMessage {
  return Boolean(
    value &&
      typeof value === "object" &&
      "id" in value &&
      typeof (value as { id?: unknown }).id === "string",
  );
}

function mergeHistoryMessages(current: SessionHistoryMessage[], next: SessionHistoryMessage[]) {
  const merged = [...current];
  const indexByID = new Map(merged.map((message, index) => [message.id, index]));
  for (const message of next) {
    const index = indexByID.get(message.id);
    if (index === undefined) {
      indexByID.set(message.id, merged.length);
      merged.push(message);
      continue;
    }
    merged[index] = message;
  }
  return merged;
}

function rowToMessage(row: MessageRow): SessionHistoryMessage {
  return {
    id: row.message_id,
    role: row.role,
    content: row.content || "",
    reasoning: row.reasoning || "",
    tool_call_id: row.tool_call_id || "",
    tool_name: row.tool_name || "",
    tool_arguments: row.tool_arguments || "",
    tool_error: row.tool_error || "",
    usage: parseUsage(row.usage_json),
    created_at: row.created_at || undefined,
  };
}

function parseUsage(value: string | null) {
  if (!value) {
    return undefined;
  }
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
}

function metaFromMessages(sessionID: string, messages: SessionHistoryMessage[]): SessionHistoryMetaPayload {
  const last = messages[messages.length - 1];
  const meta: SessionHistoryMetaPayload = {
    session_id: sessionID,
    local_message_count: messages.length,
    local_last_message_id: last?.id || "",
    local_history_version: messages.length,
  };
  if (last?.created_at) {
    meta.local_last_message_created_at = last.created_at;
  }
  return meta;
}

function emptyMeta(sessionID: string): SessionHistoryMetaPayload {
  return {
    session_id: sessionID,
    local_message_count: 0,
    local_last_message_id: "",
    local_history_version: 0,
  };
}
