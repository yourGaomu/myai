import { Pressable, StyleSheet, Text, TextInput, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { FileReadResultPayload } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { formatBytes } from "../../utils/format";

type Props = {
  attachedFiles: FileReadResultPayload[];
  buttonFeedback: ButtonFeedback;
  messageInput: string;
  onChangeMessage: (value: string) => void;
  onRemoveAttachedFile: (path: string) => void;
  onSend: () => void;
  pendingSend: boolean;
};

export function Composer({
  attachedFiles,
  buttonFeedback,
  messageInput,
  onChangeMessage,
  onRemoveAttachedFile,
  onSend,
  pendingSend,
}: Props) {
  return (
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
              <Pressable onPress={() => onRemoveAttachedFile(file.path)} style={({ pressed }) => buttonFeedback(styles.attachmentRemove, pressed)}>
                <Text style={styles.attachmentRemoveText}>Remove</Text>
              </Pressable>
            </View>
          ))}
        </View>
      ) : null}
      <View style={styles.composerRow}>
        <TextInput
          multiline
          onChangeText={onChangeMessage}
          placeholder="Message, @files, /commands"
          placeholderTextColor="#776f66"
          style={styles.messageInput}
          value={messageInput}
        />
        <Pressable
          disabled={pendingSend}
          onPress={onSend}
          style={({ pressed }) => buttonFeedback([styles.sendButton, pendingSend && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingSend} text={pendingSend ? "发送中" : "Send"} />
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  composer: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    elevation: 2,
    gap: 10,
    padding: 8,
    shadowColor: "#12100e",
    shadowOffset: { width: 4, height: 4 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
  },
  attachmentTray: {
    gap: 8,
  },
  attachmentChip: {
    alignItems: "center",
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  attachmentTitle: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  attachmentMeta: {
    color: "#6c665f",
    fontSize: 11,
    marginTop: 2,
  },
  attachmentRemove: {
    backgroundColor: "#ff7f68",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    paddingHorizontal: 9,
    paddingVertical: 6,
  },
  attachmentRemoveText: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  composerRow: {
    alignItems: "flex-end",
    flexDirection: "row",
    gap: 10,
  },
  messageInput: {
    backgroundColor: "#fdf7ea",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    color: "#12100e",
    flex: 1,
    includeFontPadding: false,
    lineHeight: 22,
    maxHeight: 120,
    minHeight: 50,
    paddingBottom: 10,
    paddingHorizontal: 12,
    paddingTop: 10,
    textAlignVertical: "top",
  },
  sendButton: {
    alignItems: "center",
    backgroundColor: "#ff7f68",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    justifyContent: "center",
    minHeight: 44,
    minWidth: 72,
    paddingHorizontal: 14,
  },
  disabledButton: {
    opacity: 0.45,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
});
