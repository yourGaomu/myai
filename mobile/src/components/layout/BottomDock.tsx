import { StyleSheet, View } from "react-native";

import type { TokenUsage, FileReadResultPayload } from "../../protocol";
import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { Composer } from "../chat/Composer";
import { BottomTabs } from "../navigation/BottomTabs";
import { TokenDock } from "../status/TokenDock";

type Props = {
  attachedFiles: FileReadResultPayload[];
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
  onRemoveAttachedFile: (path: string) => void;
  onSend: () => void;
  onSessionsPress: () => void;
  onSettingsPress: () => void;
  pendingSend: boolean;
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
  onRemoveAttachedFile,
  onSend,
  onSessionsPress,
  onSettingsPress,
  pendingSend,
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
          messageInput={messageInput}
          onChangeMessage={onChangeMessage}
          onRemoveAttachedFile={onRemoveAttachedFile}
          onSend={onSend}
          pendingSend={pendingSend}
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
