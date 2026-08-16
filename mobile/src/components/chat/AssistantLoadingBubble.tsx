import { View } from "react-native";

import { ShimmerLabel } from "./ShimmerLabel";
import { styles } from "./styles";

type Props = {
  label: string;
};

export function AssistantLoadingBubble({ label }: Props) {
  return (
    <View
      accessibilityLabel={`AI ${label}`}
      accessibilityLiveRegion="polite"
      accessible
      style={[styles.message, styles.assistantMessage, styles.assistantLoadingMessage]}
    >
      <ShimmerLabel label={label} />
    </View>
  );
}
