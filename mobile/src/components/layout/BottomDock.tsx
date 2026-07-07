import { StyleSheet, View } from "react-native";

import type { ViewMode } from "../../types/app";
import type { ChatAttachment } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import type { TokenUsage } from "../../protocol";
import { Composer } from "../chat/Composer";
import { BottomTabs } from "../navigation/BottomTabs";
import { TokenDock } from "../status/TokenDock";

type Props = {
  attachedFiles: ChatAttachment[];
  bottomPadding: number;
  buttonFeedback: ButtonFeedback;
  changesActive: boolean;
  connected: boolean;
  isBusy: boolean;
  lastUsage: TokenUsage | null;
  messageInput: string;
  onChangeMessage: (value: string) => void;
  onChangesPress: () => void;
  onChatPress: () => void;
  onFilesPress: () => void;
  onPause: () => void;
  onRemoveAttachedFile: (key: string) => void;
  onSend: () => void;
  onSessionsPress: () => void;
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
  connected,
  isBusy,
  lastUsage,
  messageInput,
  onChangeMessage,
  onChangesPress,
  onChatPress,
  onFilesPress,
  onPause,
  onRemoveAttachedFile,
  onSend,
  onSessionsPress,
  onSettingsPress,
  onUploadFile,
  pendingPause,
  pendingSend,
  pendingUpload,
  viewMode,
}: Props) {
  return (
    <View style={[styles.bottomDock, { paddingBottom: bottomPadding }]}>
      <BottomTabs
        buttonFeedback={buttonFeedback}
        changesActive={changesActive}
        onChangesPress={onChangesPress}
        onChatPress={onChatPress}
        onFilesPress={onFilesPress}
        onSessionsPress={onSessionsPress}
        onSettingsPress={onSettingsPress}
        viewMode={viewMode}
      />
      {viewMode === "chat" ? (
        <TokenDock
          buttonFeedback={buttonFeedback}
          connected={connected}
          isBusy={isBusy}
          lastUsage={lastUsage}
          onSettingsPress={onSettingsPress}
        />
      ) : null}
      {viewMode !== "settings" && viewMode !== "changeDetail" ? (
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
    </View>
  );
}

const styles = StyleSheet.create({
  bottomDock: {
    backgroundColor: "#efe4d2",
    gap: 8,
    paddingHorizontal: 14,
    paddingTop: 8,
  },
});
