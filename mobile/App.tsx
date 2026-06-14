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
  ChangeRevertResultPayload,
  ChangesListResultPayload,
  ErrorPayload,
  FileEntry,
  FileListResultPayload,
  FileReadResultPayload,
  HistoryCheckpoint,
  HistoryDiffResultPayload,
  HistoryListResultPayload,
  HistoryRevertResultPayload,
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
const maxAttachedFiles = 5;
const maxAttachedFileChars = 12000;

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
  const [historyCheckpoints, setHistoryCheckpoints] = useState<HistoryCheckpoint[]>([]);
  const [historyDiff, setHistoryDiff] = useState<HistoryDiffResultPayload | null>(null);
  const [historyMessage, setHistoryMessage] = useState("");
  const [attachedFiles, setAttachedFiles] = useState<FileReadResultPayload[]>([]);
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
  const canRevertSelectedChange = Boolean(changeDiff?.restorable || selectedChangeEntry?.restorable);
  const filePreviewAttached = Boolean(filePreview && attachedFiles.some((file) => file.path === filePreview.path));

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

  const requestHistory = useCallback(() => {
    if (!clientToken) {
      setHistoryCheckpoints([]);
      setHistoryDiff(null);
      setHistoryMessage("");
      return;
    }

    sendEnvelope("history_list", {
      request_id: newRequestID(),
      payload: { limit: 50 },
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
      requestHistory();
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
  }, [addMessage, clientToken, normalizedRelayURL, requestChanges, requestFiles, requestHistory, requestSessions]);

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
    if (!content && attachedFiles.length === 0) {
      return;
    }

    const requestID = newRequestID();
    activeRequestIDRef.current = requestID;
    activeAssistantIDRef.current = "";
    setPendingPermission(null);
    addMessage("user", userMessageEcho(content, attachedFiles));
    setMessageInput("");
    setAttachedFiles([]);

    sendEnvelope("user_message", {
      request_id: requestID,
      payload: { content: messageWithAttachedFiles(content, attachedFiles) },
    });
  }, [addMessage, attachedFiles, messageInput, sendEnvelope]);

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

  const revertSelectedChange = useCallback(() => {
    const path = changeDiff?.path || selectedChange;
    if (!path || !canRevertSelectedChange) {
      return;
    }

    Alert.alert("Revert file change?", `Restore ${path} to the saved baseline.`, [
      { text: "Cancel", style: "cancel" },
      {
        text: "Revert",
        style: "destructive",
        onPress: () => {
          sendEnvelope("change_revert", {
            request_id: newRequestID(),
            payload: { path },
          });
        },
      },
    ]);
  }, [canRevertSelectedChange, changeDiff?.path, selectedChange, sendEnvelope]);

  const revertHistoryCheckpoint = useCallback(
    (checkpointID: string) => {
      if (!checkpointID) {
        return;
      }

      const checkpoint = historyCheckpoints.find((item) => item.id === checkpointID);
      const title = checkpoint?.title || `Checkpoint ${shortID(checkpointID)}`;
      const detail = checkpoint
        ? `${checkpoint.change_count} file(s) from ${formatDateTime(checkpoint.created_at)} will be restored.`
        : "This checkpoint will be restored.";

      Alert.alert("Revert checkpoint?", `${title}\n${detail}`, [
        { text: "Cancel", style: "cancel" },
        {
          text: "Revert",
          style: "destructive",
          onPress: () => {
            sendEnvelope("history_revert", {
              request_id: newRequestID(),
              payload: { checkpoint_id: checkpointID },
            });
          },
        },
      ]);
    },
    [historyCheckpoints, sendEnvelope],
  );

  const previewHistoryCheckpoint = useCallback(
    (checkpointID: string) => {
      if (!checkpointID) {
        return;
      }

      sendEnvelope("history_diff", {
        request_id: newRequestID(),
        payload: { checkpoint_id: checkpointID },
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

  const attachFilePreview = useCallback(() => {
    if (!filePreview) {
      return;
    }
    if (filePreview.binary) {
      addMessage("error", "Binary files cannot be attached to chat yet");
      return;
    }
    if (!filePreview.content) {
      addMessage("error", "File content is empty");
      return;
    }

    setAttachedFiles((current) => {
      if (current.some((file) => file.path === filePreview.path)) {
        return current;
      }
      if (current.length >= maxAttachedFiles) {
        return [...current.slice(1), filePreview];
      }
      return [...current, filePreview];
    });
    setViewMode("chat");
  }, [addMessage, filePreview]);

  const removeAttachedFile = useCallback((path: string) => {
    setAttachedFiles((current) => current.filter((file) => file.path !== path));
  }, []);

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
          requestHistory();
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
        case "change_revert_result":
          applyChangeRevert(message.payload as ChangeRevertResultPayload | undefined);
          break;
        case "history_list_result":
          applyHistoryList(message.payload as HistoryListResultPayload | undefined);
          break;
        case "history_diff_result":
          applyHistoryDiff(message.payload as HistoryDiffResultPayload | undefined);
          break;
        case "history_revert_result":
          applyHistoryRevert(message.payload as HistoryRevertResultPayload | undefined);
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
    [addMessage, appendAssistant, filePath, requestChanges, requestFiles, requestHistory, requestSessions],
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

  const applyChangeRevert = (payload?: ChangeRevertResultPayload) => {
    if (!payload) {
      return;
    }
    addMessage("event", payload.message || `Reverted ${payload.path}`);
    setSelectedChange("");
    setChangeDiff(null);
    requestChanges();
    requestHistory();
    requestFiles(filePath);
  };

  const applyHistoryList = (payload?: HistoryListResultPayload) => {
    const checkpoints = payload?.checkpoints || [];
    setHistoryCheckpoints(checkpoints);
    setHistoryMessage(checkpoints.length === 0 ? "No file history recorded yet" : "");
    if (historyDiff && !checkpoints.some((checkpoint) => checkpoint.id === historyDiff.checkpoint_id)) {
      setHistoryDiff(null);
    }
  };

  const applyHistoryDiff = (payload?: HistoryDiffResultPayload) => {
    if (!payload) {
      return;
    }
    setHistoryDiff(payload);
    setHistoryMessage(payload.message || "");
    setViewMode("changes");
  };

  const applyHistoryRevert = (payload?: HistoryRevertResultPayload) => {
    if (!payload) {
      return;
    }
    addMessage("event", payload.message || `Reverted checkpoint ${shortID(payload.checkpoint_id)}`);
    setSelectedChange("");
    setChangeDiff(null);
    setHistoryDiff(null);
    requestChanges();
    requestHistory();
    requestFiles(filePath);
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
            {sessionID ? (
              <View style={styles.currentSessionBox}>
                <Text style={styles.currentSessionText}>{shortID(sessionID)}</Text>
                <Text style={styles.currentSessionMeta}>
                  {activeSession?.model || "model"} / {activeSession?.permission_mode || "permission"}
                </Text>
              </View>
            ) : null}
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
                  <View style={styles.previewHeader}>
                    <Text style={[styles.previewTitle, styles.flex]}>
                      {filePreview.name} / {filePreview.language} / {formatBytes(filePreview.size)}
                    </Text>
                    <Pressable
                      disabled={filePreview.binary || filePreviewAttached}
                      onPress={attachFilePreview}
                      style={[styles.previewButton, (filePreview.binary || filePreviewAttached) && styles.disabledButton]}
                    >
                      <Text style={styles.previewButtonText}>{filePreviewAttached ? "Attached" : "Attach"}</Text>
                    </Pressable>
                  </View>
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
                <View style={styles.rowCompact}>
                  <Pressable onPress={requestHistory} style={styles.smallButton}>
                    <Text style={styles.smallButtonText}>History</Text>
                  </Pressable>
                  <Pressable onPress={requestChanges} style={styles.smallButton}>
                    <Text style={styles.smallButtonText}>Refresh</Text>
                  </Pressable>
                </View>
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
                      disabled={!canRevertSelectedChange}
                      onPress={revertSelectedChange}
                      style={[styles.previewButton, !canRevertSelectedChange && styles.disabledButton]}
                    >
                      <Text style={styles.previewButtonText}>Revert</Text>
                    </Pressable>
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

              <View style={styles.historyBox}>
                <View style={styles.previewHeader}>
                  <Text style={[styles.previewTitle, styles.flex]}>File History</Text>
                  <Pressable onPress={requestHistory} style={styles.previewButton}>
                    <Text style={styles.previewButtonText}>Refresh</Text>
                  </Pressable>
                </View>
                {historyMessage ? <Text style={styles.emptyText}>{historyMessage}</Text> : null}
                {!historyMessage && historyCheckpoints.length === 0 ? (
                  <Text style={styles.emptyText}>{clientToken ? "No history loaded" : "Pair first"}</Text>
                ) : (
                  historyCheckpoints.map((checkpoint) => (
                    <View key={checkpoint.id} style={styles.historyRow}>
                      <View style={styles.flex}>
                        <Text style={styles.fileName}>{checkpoint.title || `Checkpoint ${shortID(checkpoint.id)}`}</Text>
                        <Text style={styles.fileMeta}>
                          {checkpoint.change_count} file(s) / {formatDateTime(checkpoint.created_at)}
                        </Text>
                      </View>
                      <Pressable onPress={() => previewHistoryCheckpoint(checkpoint.id)} style={styles.previewButton}>
                        <Text style={styles.previewButtonText}>Diff</Text>
                      </Pressable>
                      <Pressable onPress={() => revertHistoryCheckpoint(checkpoint.id)} style={styles.previewButton}>
                        <Text style={styles.previewButtonText}>Revert</Text>
                      </Pressable>
                    </View>
                  ))
                )}
                {historyDiff ? (
                  <View style={styles.historyDiffBox}>
                    <Text style={styles.previewTitle}>
                      Checkpoint {shortID(historyDiff.checkpoint_id)} / {(historyDiff.files || []).length} file(s)
                    </Text>
                    {(historyDiff.files || []).length === 0 ? (
                      <Text style={styles.emptyText}>{historyDiff.message || "No diff is available."}</Text>
                    ) : (
                      (historyDiff.files || []).map((file) => (
                        <View key={`${historyDiff.checkpoint_id}-${file.path}`} style={styles.historyDiffFile}>
                          <Text style={styles.fileName}>
                            {file.path}
                            {file.truncated ? " / truncated" : ""}
                          </Text>
                          <Text style={styles.fileMeta}>{file.change_type || "changed"}</Text>
                          {file.binary ? (
                            <Text style={styles.emptyText}>{file.message || "Binary diff is not available."}</Text>
                          ) : (
                            <ScrollView horizontal>
                              <Text style={styles.codeText}>{file.diff || file.message || "No diff is available."}</Text>
                            </ScrollView>
                          )}
                        </View>
                      ))
                    )}
                  </View>
                ) : null}
              </View>
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
            {attachedFiles.length > 0 ? (
              <View style={styles.attachmentTray}>
                {attachedFiles.map((file) => (
                  <View key={file.path} style={styles.attachmentChip}>
                    <View style={styles.flex}>
                      <Text style={styles.attachmentTitle}>{file.name}</Text>
                      <Text style={styles.attachmentMeta}>
                        {file.path} / {formatBytes(file.size)}
                        {file.truncated ? " / truncated" : ""}
                      </Text>
                    </View>
                    <Pressable onPress={() => removeAttachedFile(file.path)} style={styles.attachmentRemove}>
                      <Text style={styles.attachmentRemoveText}>Remove</Text>
                    </Pressable>
                  </View>
                ))}
              </View>
            ) : null}
            <View style={styles.composerRow}>
              <TextInput
                multiline
                onChangeText={setMessageInput}
                placeholder="Message, @files, /commands"
                placeholderTextColor="#6f7280"
                style={[styles.input, styles.messageInput]}
                value={messageInput}
              />
              <Pressable onPress={sendUserMessage} style={styles.sendButton}>
                <Text style={styles.sendButtonText}>Send</Text>
              </Pressable>
            </View>
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

function messageWithAttachedFiles(content: string, files: FileReadResultPayload[]) {
  if (files.length === 0) {
    return content;
  }

  const fileBlocks = files.map((file) => {
    const body = truncateText(file.content || "", maxAttachedFileChars);
    return [
      `<file path="${file.path}" language="${file.language}" size="${file.size}">`,
      body,
      file.truncated || (file.content || "").length > maxAttachedFileChars ? "\n[content truncated]" : "",
      "</file>",
    ].join("\n");
  });

  const prompt = content || "请阅读我附加的文件内容，并告诉我你看到了什么。";
  return `${prompt}\n\nAttached files:\n${fileBlocks.join("\n\n")}`;
}

function userMessageEcho(content: string, files: FileReadResultPayload[]) {
  const text = content || "Sent attached file context";
  if (files.length === 0) {
    return text;
  }
  const names = files.map((file) => `@${file.path}`).join("\n");
  return `${text}\n\n${names}`;
}

function truncateText(text: string, maxChars: number) {
  if (text.length <= maxChars) {
    return text;
  }
  return text.slice(0, maxChars);
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

function formatDateTime(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
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
    backgroundColor: "#0d1110",
  },
  keyboard: {
    flex: 1,
  },
  content: {
    gap: 10,
    padding: 12,
    paddingBottom: 18,
  },
  header: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 2,
    paddingTop: 4,
  },
  headerText: {
    flex: 1,
    minWidth: 0,
  },
  headerActions: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  title: {
    color: "#f4f0f8",
    fontSize: 22,
    fontWeight: "800",
  },
  subtitle: {
    color: "#8d91a0",
    fontSize: 12,
    marginTop: 4,
  },
  ghostButton: {
    borderColor: "#2b2d35",
    borderRadius: 8,
    borderWidth: 1,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  ghostButtonText: {
    color: "#d8d5df",
    fontSize: 12,
    fontWeight: "700",
  },
  statusPill: {
    alignItems: "center",
    borderRadius: 999,
    flexDirection: "row",
    gap: 6,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  statusPillOnline: {
    backgroundColor: "#123a32",
  },
  statusPillOffline: {
    backgroundColor: "#3a2224",
  },
  statusPillText: {
    color: "#f4f0f8",
    fontSize: 12,
    fontWeight: "700",
  },
  statusDot: {
    borderRadius: 4,
    height: 8,
    width: 8,
  },
  statusDotOnline: {
    backgroundColor: "#35d07f",
  },
  statusDotOffline: {
    backgroundColor: "#ff6961",
  },
  panel: {
    backgroundColor: "#141419",
    borderColor: "#24262f",
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
    color: "#f4f0f8",
    fontSize: 15,
    fontWeight: "700",
  },
  input: {
    backgroundColor: "#1d1d23",
    borderColor: "#2c2d36",
    borderRadius: 8,
    borderWidth: 1,
    color: "#f4f0f8",
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
    backgroundColor: "#f4f0f8",
    borderRadius: 8,
    justifyContent: "center",
    minHeight: 42,
    paddingHorizontal: 16,
  },
  primaryButtonText: {
    color: "#101014",
    fontWeight: "700",
  },
  secondaryButton: {
    alignItems: "center",
    backgroundColor: "#24262f",
    borderRadius: 8,
    justifyContent: "center",
    minHeight: 42,
    paddingHorizontal: 16,
  },
  secondaryButtonText: {
    color: "#f4f0f8",
    fontWeight: "700",
  },
  smallButton: {
    backgroundColor: "#24262f",
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  smallButtonText: {
    color: "#d8d5df",
    fontSize: 12,
    fontWeight: "700",
  },
  disabledButton: {
    opacity: 0.45,
  },
  segmented: {
    backgroundColor: "#141419",
    borderColor: "#24262f",
    borderRadius: 8,
    borderWidth: 1,
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
    backgroundColor: "#24262f",
  },
  segmentText: {
    color: "#8d91a0",
    fontWeight: "700",
  },
  segmentTextActive: {
    color: "#f4f0f8",
  },
  sessionRow: {
    flexDirection: "row",
    gap: 8,
    paddingVertical: 2,
  },
  sessionChip: {
    backgroundColor: "#18181e",
    borderColor: "#282a33",
    borderRadius: 8,
    borderWidth: 1,
    minWidth: 132,
    padding: 10,
  },
  sessionChipActive: {
    backgroundColor: "#20252c",
    borderColor: "#5f8df7",
  },
  sessionTitle: {
    color: "#f4f0f8",
    fontWeight: "700",
  },
  sessionMeta: {
    color: "#8d91a0",
    fontSize: 12,
    marginTop: 3,
  },
  currentSessionBox: {
    backgroundColor: "#101014",
    borderColor: "#24262f",
    borderRadius: 8,
    borderWidth: 1,
    gap: 3,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  currentSessionText: {
    color: "#f4f0f8",
    fontSize: 12,
    fontWeight: "700",
  },
  currentSessionMeta: {
    color: "#8d91a0",
    fontSize: 12,
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
    color: "#8d91a0",
    fontSize: 12,
    marginTop: 3,
  },
  fileList: {
    gap: 8,
  },
  fileRow: {
    alignItems: "center",
    backgroundColor: "#17171c",
    borderColor: "#252731",
    borderRadius: 8,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  fileIcon: {
    color: "#8d91a0",
    fontSize: 11,
    fontWeight: "800",
    width: 28,
  },
  fileName: {
    color: "#f4f0f8",
    fontWeight: "700",
  },
  fileMeta: {
    color: "#8d91a0",
    fontSize: 12,
    marginTop: 2,
  },
  changeList: {
    gap: 8,
  },
  changeRow: {
    alignItems: "center",
    backgroundColor: "#17171c",
    borderColor: "#252731",
    borderRadius: 8,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  changeRowActive: {
    backgroundColor: "#20252c",
    borderColor: "#5f8df7",
  },
  historyBox: {
    backgroundColor: "#101014",
    borderColor: "#24262f",
    borderRadius: 8,
    borderWidth: 1,
    gap: 8,
    padding: 12,
  },
  historyRow: {
    alignItems: "center",
    backgroundColor: "#17171c",
    borderColor: "#252731",
    borderRadius: 8,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  historyDiffBox: {
    backgroundColor: "#0f1117",
    borderColor: "#252731",
    borderRadius: 8,
    borderWidth: 1,
    gap: 8,
    maxHeight: 420,
    padding: 10,
  },
  historyDiffFile: {
    backgroundColor: "#151820",
    borderColor: "#252b36",
    borderRadius: 8,
    borderWidth: 1,
    gap: 6,
    padding: 10,
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
    backgroundColor: "#5f8df7",
  },
  changeBadgeAdded: {
    backgroundColor: "#2ea043",
  },
  changeBadgeDeleted: {
    backgroundColor: "#da3633",
  },
  changeBadgeRenamed: {
    backgroundColor: "#8957e5",
  },
  previewBox: {
    backgroundColor: "#101014",
    borderColor: "#24262f",
    borderWidth: 1,
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
    color: "#d8d5df",
    fontSize: 12,
    fontWeight: "700",
  },
  previewButton: {
    backgroundColor: "#24262f",
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  previewButtonText: {
    color: "#f4f0f8",
    fontSize: 12,
    fontWeight: "700",
  },
  codeText: {
    color: "#d8d5df",
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
    backgroundColor: "#263b33",
    maxWidth: "88%",
  },
  assistantMessage: {
    alignSelf: "flex-start",
    backgroundColor: "#1d1d23",
    maxWidth: "88%",
  },
  eventMessage: {
    alignSelf: "stretch",
    backgroundColor: "#272217",
    borderColor: "#5f4a20",
    borderWidth: 1,
  },
  errorMessage: {
    alignSelf: "stretch",
    backgroundColor: "#2e1f22",
    borderColor: "#6b2d31",
    borderWidth: 1,
  },
  messageText: {
    color: "#f4f0f8",
    lineHeight: 20,
  },
  permissionBox: {
    backgroundColor: "#272217",
    borderColor: "#5f4a20",
    borderRadius: 8,
    borderWidth: 1,
    gap: 10,
    padding: 12,
  },
  permissionTitle: {
    color: "#f0b65a",
    fontWeight: "800",
  },
  permissionArgs: {
    color: "#d8d5df",
    lineHeight: 19,
  },
  composer: {
    backgroundColor: "#141419",
    borderColor: "#24262f",
    borderRadius: 8,
    borderWidth: 1,
    gap: 10,
    padding: 8,
  },
  attachmentTray: {
    gap: 8,
  },
  attachmentChip: {
    alignItems: "center",
    backgroundColor: "#101014",
    borderColor: "#24262f",
    borderRadius: 8,
    borderWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  attachmentTitle: {
    color: "#f4f0f8",
    fontSize: 12,
    fontWeight: "800",
  },
  attachmentMeta: {
    color: "#8d91a0",
    fontSize: 11,
    marginTop: 2,
  },
  attachmentRemove: {
    backgroundColor: "#24262f",
    borderRadius: 8,
    paddingHorizontal: 9,
    paddingVertical: 6,
  },
  attachmentRemoveText: {
    color: "#d8d5df",
    fontSize: 11,
    fontWeight: "700",
  },
  composerRow: {
    alignItems: "flex-end",
    flexDirection: "row",
    gap: 10,
  },
  messageInput: {
    flex: 1,
    maxHeight: 120,
    minHeight: 44,
    textAlignVertical: "top",
  },
  sendButton: {
    alignItems: "center",
    backgroundColor: "#f4f0f8",
    borderRadius: 8,
    justifyContent: "center",
    minHeight: 44,
    paddingHorizontal: 14,
  },
  sendButtonText: {
    color: "#101014",
    fontWeight: "800",
  },
  emptyText: {
    color: "#8d91a0",
  },
});
