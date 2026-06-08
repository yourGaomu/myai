import { StatusBar } from "expo-status-bar";
import { useCallback, useMemo, useRef, useState } from "react";
import {
  Alert,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import type {
  AssistantDeltaPayload,
  ChangeDiffResultPayload,
  ChangeEntry,
  ChangesListResultPayload,
  ErrorPayload,
  FileEntry,
  FileListResultPayload,
  FileReadResultPayload,
  PairResponse,
  PermissionAskPayload,
  PermissionResultPayload,
  RelayMessage,
  SessionChangedPayload,
  SessionListResultPayload,
  SessionSummary,
  ToolCallPayload,
} from "./src/protocol";

type ChatItem = {
  id: string;
  role: "user" | "assistant" | "event" | "error";
  text: string;
};

type PermissionState = {
  requestID: string;
  sessionID: string;
  name: string;
  permission: string;
  arguments: string;
};

type ViewMode = "chat" | "files" | "changes";

const defaultRelayURL = "http://127.0.0.1:18080";

export default function App() {
  const [relayURL, setRelayURL] = useState(defaultRelayURL);
  const [bindCode, setBindCode] = useState("");
  const [userID, setUserID] = useState("local");
  const [deviceID, setDeviceID] = useState("pc-local");
  const [sessionID, setSessionID] = useState("");
  const [clientToken, setClientToken] = useState("");
  const [connected, setConnected] = useState(false);
  const [status, setStatus] = useState("Not paired");
  const [showSetup, setShowSetup] = useState(false);
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [viewMode, setViewMode] = useState<ViewMode>("chat");
  const [filePath, setFilePath] = useState(".");
  const [fileEntries, setFileEntries] = useState<FileEntry[]>([]);
  const [fileParent, setFileParent] = useState("");
  const [filePreview, setFilePreview] = useState<FileReadResultPayload | null>(null);
  const [changes, setChanges] = useState<ChangeEntry[]>([]);
  const [changesMessage, setChangesMessage] = useState("");
  const [changesClean, setChangesClean] = useState(false);
  const [selectedChange, setSelectedChange] = useState("");
  const [changeDiff, setChangeDiff] = useState<ChangeDiffResultPayload | null>(null);
  const [messageInput, setMessageInput] = useState("");
  const [messages, setMessages] = useState<ChatItem[]>([]);
  const [pendingPermission, setPendingPermission] = useState<PermissionState | null>(null);

  const socketRef = useRef<WebSocket | null>(null);
  const activeRequestIDRef = useRef("");
  const activeAssistantIDRef = useRef("");

  const normalizedRelayURL = useMemo(() => relayURL.trim().replace(/\/+$/, ""), [relayURL]);
  const setupVisible = !clientToken || !connected || showSetup;
  const activeSession = useMemo(
    () => sessions.find((session) => session.id === sessionID),
    [sessionID, sessions],
  );
  const selectedChangeEntry = useMemo(
    () => changes.find((entry) => entry.path === selectedChange),
    [changes, selectedChange],
  );

  const addMessage = useCallback((role: ChatItem["role"], text: string) => {
    setMessages((current) => [...current, { id: newRequestID(), role, text }]);
  }, []);

  const appendAssistant = useCallback((text: string) => {
    if (!text) {
      return;
    }

    setMessages((current) => {
      const assistantID = activeAssistantIDRef.current;
      if (!assistantID) {
        const id = newRequestID();
        activeAssistantIDRef.current = id;
        return [...current, { id, role: "assistant", text }];
      }

      return current.map((item) => (item.id === assistantID ? { ...item, text: item.text + text } : item));
    });
  }, []);

  const sendEnvelope = useCallback(
    (type: RelayMessage["type"], overrides: Partial<RelayMessage> = {}) => {
      const socket = socketRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        addMessage("error", "WebSocket is not connected");
        return;
      }

      const envelope: RelayMessage = {
        type,
        request_id: overrides.request_id || activeRequestIDRef.current || newRequestID(),
        user_id: userID.trim(),
        device_id: deviceID.trim(),
        session_id: overrides.session_id || sessionID.trim(),
        client_token: clientToken,
        payload: overrides.payload || {},
      };
      socket.send(JSON.stringify(envelope));
    },
    [addMessage, clientToken, deviceID, sessionID, userID],
  );

  const requestSessions = useCallback(() => {
    if (!clientToken) {
      setSessions([]);
      return;
    }
    sendEnvelope("session_list", { request_id: newRequestID() });
  }, [clientToken, sendEnvelope]);

  const requestFiles = useCallback(
    (path = filePath) => {
      if (!clientToken) {
        setFileEntries([]);
        return;
      }
      sendEnvelope("file_list", {
        request_id: newRequestID(),
        payload: { path, limit: 200 },
      });
    },
    [clientToken, filePath, sendEnvelope],
  );

  const requestChanges = useCallback(() => {
    if (!clientToken) {
      setChanges([]);
      setChangesMessage("");
      setChangesClean(false);
      setChangeDiff(null);
      return;
    }

    sendEnvelope("changes_list", {
      request_id: newRequestID(),
      payload: { limit: 200 },
    });
  }, [clientToken, sendEnvelope]);

  const connect = useCallback(() => {
    if (!clientToken) {
      addMessage("error", "Pair this phone before connecting");
      return;
    }

    socketRef.current?.close();

    const socket = new WebSocket(websocketURL(normalizedRelayURL));
    socketRef.current = socket;
    setStatus("Connecting");

    socket.onopen = () => {
      setConnected(true);
      setStatus("Connected");
      requestSessions();
      requestFiles(".");
      requestChanges();
    };
    socket.onclose = () => {
      setConnected(false);
      setStatus("Disconnected");
    };
    socket.onerror = () => {
      setConnected(false);
      setStatus("WebSocket error");
      addMessage("error", "WebSocket connection error");
    };
    socket.onmessage = (event) => {
      try {
        handleRemoteMessage(JSON.parse(event.data) as RelayMessage);
      } catch (error) {
        addMessage("error", `Invalid relay message: ${messageFromError(error)}`);
      }
    };
  }, [addMessage, clientToken, normalizedRelayURL, requestChanges, requestFiles, requestSessions]);

  const pairDevice = useCallback(async () => {
    const code = bindCode.trim();
    if (!code) {
      Alert.alert("Bind code required", "Enter the code printed by the PC Agent.");
      return;
    }

    setStatus("Pairing");
    try {
      const response = await fetch(`${normalizedRelayURL}/pair`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ bind_code: code, client_name: clientName() }),
      });

      if (!response.ok) {
        throw new Error((await response.text()).trim() || response.statusText);
      }

      const data = (await response.json()) as PairResponse;
      setUserID(data.user_id || "local");
      setDeviceID(data.device_id || "pc-local");
      setClientToken(data.client_token || "");
      setBindCode("");
      setStatus(`Paired ${data.user_id}/${data.device_id}`);
    } catch (error) {
      setStatus("Pair failed");
      addMessage("error", messageFromError(error));
    }
  }, [addMessage, bindCode, normalizedRelayURL]);

  const sendUserMessage = useCallback(() => {
    const content = messageInput.trim();
    if (!content) {
      return;
    }

    const requestID = newRequestID();
    activeRequestIDRef.current = requestID;
    activeAssistantIDRef.current = "";
    setPendingPermission(null);
    addMessage("user", content);
    setMessageInput("");

    sendEnvelope("user_message", {
      request_id: requestID,
      payload: { content },
    });
  }, [addMessage, messageInput, sendEnvelope]);

  const newSession = useCallback(() => {
    sendEnvelope("session_new", { request_id: newRequestID() });
  }, [sendEnvelope]);

  const loadSession = useCallback(
    (nextSessionID: string) => {
      sendEnvelope("session_load", {
        request_id: newRequestID(),
        session_id: nextSessionID,
        payload: { session_id: nextSessionID },
      });
    },
    [sendEnvelope],
  );

  const readFilePath = useCallback(
    (path: string) => {
      sendEnvelope("file_read", {
        request_id: newRequestID(),
        payload: { path },
      });
    },
    [sendEnvelope],
  );

  const openFileEntry = useCallback(
    (entry: FileEntry) => {
      if (entry.type === "dir") {
        setFilePreview(null);
        requestFiles(entry.path);
        return;
      }
      readFilePath(entry.path);
    },
    [readFilePath, requestFiles],
  );

  const openSelectedChangeFile = useCallback(() => {
    const path = changeDiff?.path || selectedChange;
    if (!path) {
      return;
    }

    requestFiles(parentPathOf(path));
    readFilePath(path);
  }, [changeDiff?.path, readFilePath, requestFiles, selectedChange]);

  const canOpenSelectedChangeFile = Boolean(selectedChange) && !selectedChangeEntry?.deleted;

  const openChangeEntry = useCallback(
    (entry: ChangeEntry) => {
      setSelectedChange(entry.path);
      setChangeDiff(null);
      sendEnvelope("change_diff", {
        request_id: newRequestID(),
        payload: { path: entry.path },
      });
    },
    [sendEnvelope],
  );

  const goToParent = useCallback(() => {
    if (!fileParent) {
      return;
    }
    setFilePreview(null);
    requestFiles(fileParent);
  }, [fileParent, requestFiles]);

  const sendPermissionResult = useCallback(
    (allowed: boolean) => {
      if (!pendingPermission) {
        return;
      }

      const payload: PermissionResultPayload = { allowed };
      sendEnvelope("permission_result", {
        request_id: pendingPermission.requestID,
        session_id: pendingPermission.sessionID,
        payload,
      });
      addMessage("event", `${allowed ? "Allowed" : "Denied"} ${pendingPermission.name}`);
      setPendingPermission(null);
    },
    [addMessage, pendingPermission, sendEnvelope],
  );

  const handleRemoteMessage = useCallback(
    (message: RelayMessage) => {
      switch (message.type) {
        case "heartbeat":
          setStatus(message.request_id ? `Ack ${shortID(message.request_id)}` : "Connected");
          break;
        case "assistant_delta":
          appendAssistant((message.payload as AssistantDeltaPayload | undefined)?.content || "");
          break;
        case "assistant_done":
          if (message.session_id) {
            setSessionID(message.session_id);
          }
          setStatus("Done");
          setPendingPermission(null);
          requestSessions();
          requestFiles(filePath);
          requestChanges();
          break;
        case "tool_call": {
          const payload = (message.payload || {}) as ToolCallPayload;
          addMessage("event", `Tool call: ${payload.name || ""}\n${payload.arguments || ""}`);
          break;
        }
        case "permission_ask": {
          const payload = (message.payload || {}) as PermissionAskPayload;
          setPendingPermission({
            requestID: message.request_id || "",
            sessionID: message.session_id || "",
            name: payload.name || "tool",
            permission: payload.permission || "permission",
            arguments: payload.arguments || "",
          });
          break;
        }
        case "session_list_result":
          applySessionList(message.payload as SessionListResultPayload | undefined);
          break;
        case "session_changed":
          applySessionChanged(message.payload as SessionChangedPayload | undefined);
          addMessage("event", "Session changed");
          break;
        case "file_list_result":
          applyFileList(message.payload as FileListResultPayload | undefined);
          break;
        case "file_read_result":
          applyFileRead(message.payload as FileReadResultPayload | undefined);
          break;
        case "changes_list_result":
          applyChangesList(message.payload as ChangesListResultPayload | undefined);
          break;
        case "change_diff_result":
          applyChangeDiff(message.payload as ChangeDiffResultPayload | undefined);
          break;
        case "error": {
          const payload = (message.payload || {}) as ErrorPayload;
          addMessage("error", payload.message || "Remote error");
          setPendingPermission(null);
          break;
        }
        default:
          addMessage("event", `Message: ${message.type}`);
      }
    },
    [addMessage, appendAssistant, filePath, requestChanges, requestFiles, requestSessions],
  );

  const applySessionList = (payload?: SessionListResultPayload) => {
    const nextSessions = payload?.sessions || [];
    setSessions(nextSessions);
    if (payload?.current_session_id) {
      setSessionID(payload.current_session_id);
    }
  };

  const applySessionChanged = (payload?: SessionChangedPayload) => {
    const nextSessions = payload?.sessions || [];
    setSessions(nextSessions);
    if (payload?.current_session_id) {
      setSessionID(payload.current_session_id);
    } else if (payload?.session?.id) {
      setSessionID(payload.session.id);
    }
  };

  const applyFileList = (payload?: FileListResultPayload) => {
    setFilePath(payload?.path || ".");
    setFileParent(payload?.parent || "");
    setFileEntries(payload?.entries || []);
  };

  const applyFileRead = (payload?: FileReadResultPayload) => {
    if (!payload) {
      return;
    }
    setFilePreview(payload);
    setViewMode("files");
  };

  const applyChangesList = (payload?: ChangesListResultPayload) => {
    const nextChanges = payload?.entries || [];
    setChanges(nextChanges);
    setChangesClean(Boolean(payload?.clean));
    setChangesMessage(payload?.message || "");
    if (selectedChange && !nextChanges.some((entry) => entry.path === selectedChange)) {
      setSelectedChange("");
      setChangeDiff(null);
    }
  };

  const applyChangeDiff = (payload?: ChangeDiffResultPayload) => {
    if (!payload) {
      return;
    }
    setSelectedChange(payload.path || "");
    setChangeDiff(payload);
    setViewMode("changes");
  };

  return (
    <SafeAreaView style={styles.safe}>
      <StatusBar style="light" />
      <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={styles.keyboard}>
        <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
          <View style={styles.header}>
            <View style={styles.headerText}>
              <Text style={styles.title}>MyAI</Text>
              <Text style={styles.subtitle}>
                {connected ? `${userID.trim() || "local"} / ${deviceID.trim() || "pc-local"}` : status}
              </Text>
            </View>
            <View style={styles.headerActions}>
              <Pressable onPress={() => setShowSetup((value) => !value)} style={styles.ghostButton}>
                <Text style={styles.ghostButtonText}>{setupVisible ? "Hide" : "Setup"}</Text>
              </Pressable>
              <View style={[styles.statusPill, connected ? styles.statusPillOnline : styles.statusPillOffline]}>
                <View style={[styles.statusDot, connected ? styles.statusDotOnline : styles.statusDotOffline]} />
                <Text style={styles.statusPillText}>{connected ? "Online" : "Offline"}</Text>
              </View>
            </View>
          </View>

          {setupVisible ? (
          <View style={styles.panel}>
            <View style={styles.panelHeader}>
              <View>
                <Text style={styles.panelTitle}>Relay</Text>
                <Text style={styles.pathText}>Pair the phone with your PC Agent</Text>
              </View>
              <Pressable onPress={connect} style={styles.primaryButton}>
                <Text style={styles.primaryButtonText}>{connected ? "Reconnect" : "Connect"}</Text>
              </Pressable>
            </View>
            <TextInput
              autoCapitalize="none"
              autoCorrect={false}
              onChangeText={setRelayURL}
              placeholder="http://server:18080"
              placeholderTextColor="#6f7280"
              style={styles.input}
              value={relayURL}
            />
            <View style={styles.row}>
              <TextInput
                keyboardType="number-pad"
                maxLength={6}
                onChangeText={setBindCode}
                placeholder="Bind code"
                placeholderTextColor="#6f7280"
                style={[styles.input, styles.flex]}
                value={bindCode}
              />
              <Pressable onPress={pairDevice} style={styles.secondaryButton}>
                <Text style={styles.secondaryButtonText}>Pair</Text>
              </Pressable>
            </View>
            <View style={styles.row}>
              <TextInput
                onChangeText={setUserID}
                placeholder="User"
                placeholderTextColor="#6f7280"
                style={[styles.input, styles.flex]}
                value={userID}
              />
              <TextInput
                onChangeText={setDeviceID}
                placeholder="Device"
                placeholderTextColor="#6f7280"
                style={[styles.input, styles.flex]}
                value={deviceID}
              />
            </View>
          </View>
          ) : null}

          <View style={styles.panel}>
            <View style={styles.panelHeader}>
              <View style={styles.flex}>
                <Text style={styles.panelTitle}>Sessions</Text>
                <Text style={styles.pathText}>{activeSession?.title || (sessionID ? shortID(sessionID) : "No active session")}</Text>
              </View>
              <View style={styles.rowCompact}>
                <Pressable onPress={newSession} style={styles.smallButton}>
                  <Text style={styles.smallButtonText}>New</Text>
                </Pressable>
                <Pressable onPress={requestSessions} style={styles.smallButton}>
                  <Text style={styles.smallButtonText}>Refresh</Text>
                </Pressable>
              </View>
            </View>
            <TextInput
              autoCapitalize="none"
              autoCorrect={false}
              onChangeText={setSessionID}
              placeholder="Current session"
              placeholderTextColor="#6f7280"
              style={styles.input}
              value={sessionID}
            />
            <ScrollView horizontal showsHorizontalScrollIndicator={false}>
              <View style={styles.sessionRow}>
                {sessions.length === 0 ? (
                  <Text style={styles.emptyText}>{clientToken ? "No sessions loaded" : "Pair first"}</Text>
                ) : (
                  sessions.map((session) => (
                    <Pressable
                      key={session.id}
                      onPress={() => loadSession(session.id)}
                      style={[styles.sessionChip, session.id === sessionID && styles.sessionChipActive]}
                    >
                      <Text style={styles.sessionTitle}>{session.title || "New chat"}</Text>
                      <Text style={styles.sessionMeta}>{shortID(session.id)}</Text>
                    </Pressable>
                  ))
                )}
              </View>
            </ScrollView>
          </View>

          <View style={styles.segmented}>
            <Pressable onPress={() => setViewMode("chat")} style={[styles.segment, viewMode === "chat" && styles.segmentActive]}>
              <Text style={[styles.segmentText, viewMode === "chat" && styles.segmentTextActive]}>Chat</Text>
            </Pressable>
            <Pressable
              onPress={() => {
                setViewMode("files");
                if (fileEntries.length === 0) {
                  requestFiles(filePath);
                }
              }}
              style={[styles.segment, viewMode === "files" && styles.segmentActive]}
            >
              <Text style={[styles.segmentText, viewMode === "files" && styles.segmentTextActive]}>Files</Text>
            </Pressable>
            <Pressable
              onPress={() => {
                setViewMode("changes");
                requestChanges();
              }}
              style={[styles.segment, viewMode === "changes" && styles.segmentActive]}
            >
              <Text style={[styles.segmentText, viewMode === "changes" && styles.segmentTextActive]}>Changes</Text>
            </Pressable>
          </View>

          {viewMode === "chat" ? (
            <View style={[styles.panel, styles.chatPanel]}>
              <Text style={styles.panelTitle}>Chat</Text>
              <View style={styles.messages}>
                {messages.length === 0 ? (
                  <Text style={styles.emptyText}>Messages will appear here.</Text>
                ) : (
                  messages.map((message) => (
                    <View key={message.id} style={[styles.message, styles[`${message.role}Message`]]}>
                      <Text style={styles.messageText}>{message.text}</Text>
                    </View>
                  ))
                )}
              </View>
            </View>
          ) : null}

          {viewMode === "files" ? (
            <View style={[styles.panel, styles.filesPanel]}>
              <View style={styles.panelHeader}>
                <View style={styles.flex}>
                  <Text style={styles.panelTitle}>Files</Text>
                  <Text style={styles.pathText}>{filePath}</Text>
                </View>
                <View style={styles.rowCompact}>
                  <Pressable disabled={!fileParent} onPress={goToParent} style={[styles.smallButton, !fileParent && styles.disabledButton]}>
                    <Text style={styles.smallButtonText}>Up</Text>
                  </Pressable>
                  <Pressable onPress={() => requestFiles(filePath)} style={styles.smallButton}>
                    <Text style={styles.smallButtonText}>Refresh</Text>
                  </Pressable>
                </View>
              </View>

              <View style={styles.fileList}>
                {fileEntries.length === 0 ? (
                  <Text style={styles.emptyText}>{clientToken ? "No files loaded" : "Pair first"}</Text>
                ) : (
                  fileEntries.map((entry) => (
                    <Pressable key={entry.path} onPress={() => openFileEntry(entry)} style={styles.fileRow}>
                      <Text style={styles.fileIcon}>{entry.type === "dir" ? "DIR" : "TXT"}</Text>
                      <View style={styles.flex}>
                        <Text style={styles.fileName}>{entry.name}</Text>
                        <Text style={styles.fileMeta}>{entry.type === "dir" ? entry.path : `${formatBytes(entry.size || 0)} / ${entry.path}`}</Text>
                      </View>
                    </Pressable>
                  ))
                )}
              </View>

              {filePreview ? (
                <View style={styles.previewBox}>
                  <Text style={styles.previewTitle}>
                    {filePreview.name} / {filePreview.language} / {formatBytes(filePreview.size)}
                  </Text>
                  {filePreview.binary ? (
                    <Text style={styles.emptyText}>Binary file preview is not available.</Text>
                  ) : (
                    <ScrollView horizontal>
                      <Text style={styles.codeText}>
                        {filePreview.content || ""}
                        {filePreview.truncated ? "\n\n[truncated]" : ""}
                      </Text>
                    </ScrollView>
                  )}
                </View>
              ) : null}
            </View>
          ) : null}

          {viewMode === "changes" ? (
            <View style={[styles.panel, styles.changesPanel]}>
              <View style={styles.panelHeader}>
                <View style={styles.flex}>
                  <Text style={styles.panelTitle}>Changes</Text>
                  <Text style={styles.pathText}>{changesClean ? "Clean workspace" : `${changes.length} changed file(s)`}</Text>
                </View>
                <Pressable onPress={requestChanges} style={styles.smallButton}>
                  <Text style={styles.smallButtonText}>Refresh</Text>
                </Pressable>
              </View>

              <View style={styles.changeList}>
                {changesMessage ? <Text style={styles.emptyText}>{changesMessage}</Text> : null}
                {!changesMessage && changes.length === 0 ? (
                  <Text style={styles.emptyText}>{clientToken ? "No changes loaded" : "Pair first"}</Text>
                ) : (
                  changes.map((entry) => (
                    <Pressable
                      key={`${entry.path}-${entry.index_status || ""}-${entry.worktree_status || ""}`}
                      onPress={() => openChangeEntry(entry)}
                      style={[styles.changeRow, selectedChange === entry.path && styles.changeRowActive]}
                    >
                      <Text style={[styles.changeBadge, changeBadgeStyle(entry)]}>{changeLabel(entry)}</Text>
                      <View style={styles.flex}>
                        <Text style={styles.fileName}>{entry.path}</Text>
                        <Text style={styles.fileMeta}>{changeMeta(entry)}</Text>
                      </View>
                    </Pressable>
                  ))
                )}
              </View>

              {changeDiff ? (
                <View style={styles.previewBox}>
                  <View style={styles.previewHeader}>
                    <Text style={[styles.previewTitle, styles.flex]}>
                      {changeDiff.path}
                      {changeDiff.truncated ? " / truncated" : ""}
                    </Text>
                    <Pressable
                      disabled={!canOpenSelectedChangeFile}
                      onPress={openSelectedChangeFile}
                      style={[styles.previewButton, !canOpenSelectedChangeFile && styles.disabledButton]}
                    >
                      <Text style={styles.previewButtonText}>Open file</Text>
                    </Pressable>
                  </View>
                  {changeDiff.binary ? (
                    <Text style={styles.emptyText}>{changeDiff.message || "Binary diff is not available."}</Text>
                  ) : (
                    <ScrollView horizontal>
                      <Text style={styles.codeText}>{changeDiff.diff || changeDiff.message || "No diff is available."}</Text>
                    </ScrollView>
                  )}
                </View>
              ) : null}
            </View>
          ) : null}

          {pendingPermission ? (
            <View style={styles.permissionBox}>
              <Text style={styles.permissionTitle}>
                {pendingPermission.name} requires {pendingPermission.permission}
              </Text>
              <Text style={styles.permissionArgs}>{pendingPermission.arguments}</Text>
              <View style={styles.row}>
                <Pressable onPress={() => sendPermissionResult(false)} style={[styles.secondaryButton, styles.flex]}>
                  <Text style={styles.secondaryButtonText}>Deny</Text>
                </Pressable>
                <Pressable onPress={() => sendPermissionResult(true)} style={[styles.primaryButton, styles.flex]}>
                  <Text style={styles.primaryButtonText}>Allow</Text>
                </Pressable>
              </View>
            </View>
          ) : null}

          <View style={styles.composer}>
            <TextInput
              multiline
              onChangeText={setMessageInput}
              placeholder="Ask the PC Agent..."
              style={[styles.input, styles.messageInput]}
              value={messageInput}
            />
            <Pressable onPress={sendUserMessage} style={styles.primaryButton}>
              <Text style={styles.primaryButtonText}>Send</Text>
            </Pressable>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

function websocketURL(relayURL: string) {
  if (relayURL.startsWith("https://")) {
    return relayURL.replace(/^https:\/\//, "wss://") + "/ws/client";
  }
  if (relayURL.startsWith("http://")) {
    return relayURL.replace(/^http:\/\//, "ws://") + "/ws/client";
  }
  return `ws://${relayURL}/ws/client`;
}

function clientName() {
  return `Mobile ${Platform.OS}`;
}

function newRequestID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function shortID(value?: string) {
  if (!value) {
    return "-";
  }
  return value.length > 8 ? value.slice(0, 8) : value;
}

function parentPathOf(path: string) {
  const normalized = path.replace(/\\/g, "/");
  const index = normalized.lastIndexOf("/");
  if (index <= 0) {
    return ".";
  }
  return normalized.slice(0, index);
}

function messageFromError(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function formatBytes(size: number) {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

function changeLabel(entry: ChangeEntry) {
  if (entry.untracked) {
    return "?";
  }
  if (entry.deleted) {
    return "D";
  }
  if (entry.renamed) {
    return "R";
  }
  if (entry.status === "added") {
    return "A";
  }
  if (entry.status === "modified") {
    return "M";
  }
  return "C";
}

function changeMeta(entry: ChangeEntry) {
  const parts = [entry.status || "changed"];
  if (entry.staged) {
    parts.push("staged");
  }
  if (entry.unstaged) {
    parts.push("unstaged");
  }
  if (entry.old_path) {
    parts.push(`from ${entry.old_path}`);
  }
  return parts.join(" / ");
}

function changeBadgeStyle(entry: ChangeEntry) {
  if (entry.deleted) {
    return styles.changeBadgeDeleted;
  }
  if (entry.untracked || entry.status === "added") {
    return styles.changeBadgeAdded;
  }
  if (entry.renamed) {
    return styles.changeBadgeRenamed;
  }
  return styles.changeBadgeModified;
}

const styles = StyleSheet.create({
  safe: {
    flex: 1,
    backgroundColor: "#f4f7fb",
  },
  keyboard: {
    flex: 1,
  },
  content: {
    gap: 14,
    padding: 16,
    paddingBottom: 28,
  },
  header: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  title: {
    color: "#17202a",
    fontSize: 26,
    fontWeight: "800",
  },
  subtitle: {
    color: "#667085",
    marginTop: 4,
  },
  statusDot: {
    borderRadius: 8,
    height: 16,
    width: 16,
  },
  statusDotOnline: {
    backgroundColor: "#0f766e",
  },
  statusDotOffline: {
    backgroundColor: "#b42318",
  },
  panel: {
    backgroundColor: "#ffffff",
    borderColor: "#d6dde7",
    borderRadius: 8,
    borderWidth: 1,
    gap: 10,
    padding: 12,
  },
  panelHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  panelTitle: {
    color: "#17202a",
    fontSize: 16,
    fontWeight: "700",
  },
  input: {
    backgroundColor: "#ffffff",
    borderColor: "#d6dde7",
    borderRadius: 8,
    borderWidth: 1,
    color: "#17202a",
    minHeight: 42,
    paddingHorizontal: 12,
    paddingVertical: 9,
  },
  row: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  rowCompact: {
    flexDirection: "row",
    gap: 8,
  },
  flex: {
    flex: 1,
  },
  primaryButton: {
    alignItems: "center",
    backgroundColor: "#0f766e",
    borderRadius: 8,
    justifyContent: "center",
    minHeight: 42,
    paddingHorizontal: 16,
  },
  primaryButtonText: {
    color: "#ffffff",
    fontWeight: "700",
  },
  secondaryButton: {
    alignItems: "center",
    backgroundColor: "#eef3f8",
    borderRadius: 8,
    justifyContent: "center",
    minHeight: 42,
    paddingHorizontal: 16,
  },
  secondaryButtonText: {
    color: "#17202a",
    fontWeight: "700",
  },
  smallButton: {
    backgroundColor: "#eef3f8",
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  smallButtonText: {
    color: "#17202a",
    fontSize: 12,
    fontWeight: "700",
  },
  disabledButton: {
    opacity: 0.45,
  },
  segmented: {
    backgroundColor: "#e9eef5",
    borderRadius: 8,
    flexDirection: "row",
    padding: 4,
  },
  segment: {
    alignItems: "center",
    borderRadius: 6,
    flex: 1,
    paddingVertical: 9,
  },
  segmentActive: {
    backgroundColor: "#ffffff",
  },
  segmentText: {
    color: "#667085",
    fontWeight: "700",
  },
  segmentTextActive: {
    color: "#17202a",
  },
  sessionRow: {
    flexDirection: "row",
    gap: 8,
    paddingVertical: 2,
  },
  sessionChip: {
    borderColor: "#d6dde7",
    borderRadius: 8,
    borderWidth: 1,
    minWidth: 132,
    padding: 10,
  },
  sessionChipActive: {
    backgroundColor: "#edf8f6",
    borderColor: "#0f766e",
  },
  sessionTitle: {
    color: "#17202a",
    fontWeight: "700",
  },
  sessionMeta: {
    color: "#667085",
    fontSize: 12,
    marginTop: 3,
  },
  chatPanel: {
    minHeight: 260,
  },
  filesPanel: {
    minHeight: 280,
  },
  changesPanel: {
    minHeight: 320,
  },
  pathText: {
    color: "#667085",
    fontSize: 12,
    marginTop: 3,
  },
  fileList: {
    gap: 8,
  },
  fileRow: {
    alignItems: "center",
    borderColor: "#d6dde7",
    borderRadius: 8,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  fileIcon: {
    color: "#0f766e",
    fontSize: 11,
    fontWeight: "800",
    width: 28,
  },
  fileName: {
    color: "#17202a",
    fontWeight: "700",
  },
  fileMeta: {
    color: "#667085",
    fontSize: 12,
    marginTop: 2,
  },
  changeList: {
    gap: 8,
  },
  changeRow: {
    alignItems: "center",
    borderColor: "#d6dde7",
    borderRadius: 8,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  changeRowActive: {
    backgroundColor: "#edf8f6",
    borderColor: "#0f766e",
  },
  changeBadge: {
    borderRadius: 6,
    color: "#ffffff",
    fontSize: 12,
    fontWeight: "800",
    overflow: "hidden",
    paddingHorizontal: 8,
    paddingVertical: 4,
    textAlign: "center",
    width: 30,
  },
  changeBadgeModified: {
    backgroundColor: "#0f766e",
  },
  changeBadgeAdded: {
    backgroundColor: "#15803d",
  },
  changeBadgeDeleted: {
    backgroundColor: "#b42318",
  },
  changeBadgeRenamed: {
    backgroundColor: "#6d5bd0",
  },
  previewBox: {
    backgroundColor: "#111827",
    borderRadius: 8,
    gap: 8,
    maxHeight: 360,
    padding: 12,
  },
  previewHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  previewTitle: {
    color: "#d1d5db",
    fontSize: 12,
    fontWeight: "700",
  },
  previewButton: {
    backgroundColor: "#263244",
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  previewButtonText: {
    color: "#f9fafb",
    fontSize: 12,
    fontWeight: "700",
  },
  codeText: {
    color: "#e5e7eb",
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 12,
    lineHeight: 18,
  },
  messages: {
    gap: 8,
  },
  message: {
    borderRadius: 8,
    padding: 10,
  },
  userMessage: {
    alignSelf: "flex-end",
    backgroundColor: "#dff4ef",
    maxWidth: "88%",
  },
  assistantMessage: {
    alignSelf: "flex-start",
    backgroundColor: "#f0f4f8",
    maxWidth: "88%",
  },
  eventMessage: {
    alignSelf: "stretch",
    backgroundColor: "#fff8ed",
    borderColor: "#f1c48b",
    borderWidth: 1,
  },
  errorMessage: {
    alignSelf: "stretch",
    backgroundColor: "#fff1f0",
    borderColor: "#f2b8b5",
    borderWidth: 1,
  },
  messageText: {
    color: "#17202a",
    lineHeight: 20,
  },
  permissionBox: {
    backgroundColor: "#fff8ed",
    borderColor: "#f1c48b",
    borderRadius: 8,
    borderWidth: 1,
    gap: 10,
    padding: 12,
  },
  permissionTitle: {
    color: "#b54708",
    fontWeight: "800",
  },
  permissionArgs: {
    color: "#667085",
    lineHeight: 19,
  },
  composer: {
    backgroundColor: "#ffffff",
    borderColor: "#d6dde7",
    borderRadius: 8,
    borderWidth: 1,
    gap: 10,
    padding: 12,
  },
  messageInput: {
    minHeight: 82,
    textAlignVertical: "top",
  },
  emptyText: {
    color: "#667085",
  },
});
