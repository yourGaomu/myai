import { ActivityIndicator, Text, View } from "react-native";

import { styles } from "./styles";

export function AssistantLoadingBubble() {
  return (
    <View style={[styles.message, styles.assistantMessage, styles.assistantLoadingMessage]}>
      <ActivityIndicator color="#12100e" size="small" />
      <Text style={styles.assistantLoadingText}>AI 正在回复...</Text>
    </View>
  );
}
