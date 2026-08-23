import { StyleSheet, View } from "react-native";

import type { ViewMode } from "../../types/app";
import type { ChatAttachment } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { Composer } from "../chat/Composer";
import { BottomTabs } from "../navigation/BottomTabs";

type Props = {
  attachedFiles: ChatAttachment[];
  bottomPadding: number;
  buttonFeedback: ButtonFeedback;
  changesActive: boolean;
  messageInput: string;
  onChangeMessage: (value: string) => void;
  onChangesPress: () => void;
  onChatPress: () => void;
  onFilesPress: () => void;
  onPause: () => void;
  onRemoveAttachedFile: (key: string) => void;
  onSend: () => void;
  onKnowledgePress: () => void;
  onSettingsPress: () => void;
  onUploadFile: () => void;
  pendingPause: boolean;
  pendingSend: boolean;
  pendingUpload: boolean;
  viewMode: ViewMode;
};

export function BottomDock({
  attachedFiles,
  bottomPadding,
  buttonFeedback,
  changesActive,
  messageInput,
  onChangeMessage,
  onChangesPress,
  onChatPress,
  onFilesPress,
  onPause,
  onRemoveAttachedFile,
  onSend,
  onKnowledgePress,
  onSettingsPress,
  onUploadFile,
  pendingPause,
  pendingSend,
  pendingUpload,
  viewMode,
}: Props) {
  return (
    <View style={[styles.bottomDock, { paddingBottom: bottomPadding }]}>
      {viewMode !== "settings" && viewMode !== "changeDetail" && viewMode !== "plan" && viewMode !== "knowledge" ? (
        <Composer
          attachedFiles={attachedFiles}
          buttonFeedback={buttonFeedback}
          canPause={pendingSend}
          messageInput={messageInput}
          onChangeMessage={onChangeMessage}
          onPause={onPause}
          onRemoveAttachedFile={onRemoveAttachedFile}
          onSend={onSend}
          onUploadFile={onUploadFile}
          pendingPause={pendingPause}
          pendingSend={pendingSend}
          pendingUpload={pendingUpload}
        />
      ) : null}
      <BottomTabs
        buttonFeedback={buttonFeedback}
        changesActive={changesActive}
        onChangesPress={onChangesPress}
        onChatPress={onChatPress}
        onFilesPress={onFilesPress}
        onKnowledgePress={onKnowledgePress}
        onSettingsPress={onSettingsPress}
        viewMode={viewMode}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  bottomDock: {
    backgroundColor: "#f4f5f7",
    gap: 7,
    paddingHorizontal: 14,
    paddingTop: 9,
  },
});
