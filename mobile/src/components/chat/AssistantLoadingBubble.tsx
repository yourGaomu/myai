import { StyleSheet, View } from "react-native";

import type { MotionSlot } from "../../utils/motionRegistry";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { Spinner } from "../common/Spinner";
import { ShimmerLabel } from "./ShimmerLabel";
import { useChatStyles } from "./styles";

type Props = {
  label: string;
  slot?: MotionSlot;
};

// 回答还没开始输出时的占位：动态角色头像 + 外圈转动 + 闪烁状态文字。
export function AssistantLoadingBubble({ label, slot = "thinking" }: Props) {
  const styles = useChatStyles();

  return (
    <View
      accessibilityLabel={`AI ${label}`}
      accessibilityLiveRegion="polite"
      accessible
      style={styles.assistantLoadingMessage}
    >
      <View style={local.avatar}>
        <CharacterAvatar circular size={24} slot={slot} variant="avatar" />
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
  ring: { left: -3, position: "absolute", top: -3 },
});
