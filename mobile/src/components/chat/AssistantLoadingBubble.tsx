import { Image, StyleSheet, View } from "react-native";

import { palette, radius } from "../../constants/theme";
import { Spinner } from "../common/Spinner";
import { ShimmerLabel } from "./ShimmerLabel";
import { styles } from "./styles";

type Props = {
  label: string;
};

// 回答还没开始输出时的占位：头像外圈转动 + 闪烁的状态文字。
export function AssistantLoadingBubble({ label }: Props) {
  return (
    <View
      accessibilityLabel={`AI ${label}`}
      accessibilityLiveRegion="polite"
      accessible
      style={styles.assistantLoadingMessage}
    >
      <View style={local.avatar}>
        <Image source={require("../../../assets/avatar.png")} style={local.avatarImage} />
        <View style={local.ring}>
          <Spinner size={30} />
        </View>
      </View>
      <ShimmerLabel label={label} />
    </View>
  );
}

const local = StyleSheet.create({
  avatar: { height: 24, marginRight: 4, width: 24 },
  avatarImage: { backgroundColor: palette.bubble, borderRadius: radius.pill, height: 24, width: 24 },
  ring: { left: -3, position: "absolute", top: -3 },
});
