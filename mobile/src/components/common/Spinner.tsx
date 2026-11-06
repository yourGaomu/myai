import { useEffect, useRef } from "react";
import { Animated, Easing, StyleSheet } from "react-native";

import { palette } from "../../constants/theme";

type Props = {
  color?: string;
  size?: number;
};

// 细线旋转圈：比系统 ActivityIndicator 更轻，尺寸可以小到 12px，用在时间线和列表行里。
export function Spinner({ color = palette.brand, size = 14 }: Props) {
  const spin = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    const loop = Animated.loop(
      Animated.timing(spin, { duration: 800, easing: Easing.linear, toValue: 1, useNativeDriver: true }),
    );
    loop.start();
    return () => loop.stop();
  }, [spin]);

  const rotate = spin.interpolate({ inputRange: [0, 1], outputRange: ["0deg", "360deg"] });

  return (
    <Animated.View
      accessibilityLabel="加载中"
      style={[
        styles.ring,
        { borderRadius: size / 2, borderTopColor: color, height: size, transform: [{ rotate }], width: size },
      ]}
    />
  );
}

const styles = StyleSheet.create({
  ring: {
    borderColor: palette.hairlineStrong,
    borderWidth: 2,
  },
});
