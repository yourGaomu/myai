import { Image, type ImageContentFit } from "expo-image";
import type { StyleProp, ViewStyle } from "react-native";
import { StyleSheet, View } from "react-native";

import { useMotion } from "../../hooks/useMotionConfig";
import type { MotionSlot } from "../../utils/motionRegistry";

type Props = {
  accessibilityLabel?: string;
  autoplay?: boolean;
  circular?: boolean;
  contentFit?: ImageContentFit;
  size: number;
  height?: number;
  slot: MotionSlot;
  style?: StyleProp<ViewStyle>;
  variant?: "full" | "avatar";
};

// 统一角色动效载体：自动根据槽位加载自定义 GIF 或内置透明 WebP，并应用缩放与偏移校准。
export function CharacterAvatar({
  accessibilityLabel,
  autoplay,
  circular = false,
  contentFit = "contain",
  height,
  size,
  slot,
  style,
  variant = "full",
}: Props) {
  const { resolveMedia, settings, systemReduceMotion } = useMotion();
  const resolved = resolveMedia(slot, variant);
  const shouldPlay = autoplay ?? (settings.motionEnabled && !settings.reduceMotion && !systemReduceMotion);
  const boxH = height ?? size;

  return (
    <View
      style={[
        styles.frame,
        {
          borderRadius: circular ? size / 2 : 0,
          height: boxH,
          width: size,
        },
        style,
      ]}
    >
      <Image
        accessibilityLabel={accessibilityLabel}
        autoplay={shouldPlay}
        contentFit={contentFit}
        contentPosition={resolved.cropPosition}
        source={typeof resolved.source === "string" ? { uri: resolved.source } : resolved.source}
        style={[
          styles.image,
          resolved.scale !== 1 || resolved.offsetY !== 0
            ? {
                transform: [{ translateY: resolved.offsetY }, { scale: resolved.scale }],
              }
            : null,
        ]}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  frame: {
    alignItems: "center",
    justifyContent: "center",
    overflow: "hidden",
  },
  image: {
    height: "100%",
    width: "100%",
  },
});
