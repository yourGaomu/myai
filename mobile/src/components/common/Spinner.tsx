import { useEffect, useRef } from "react";
import { Animated, Easing, StyleSheet } from "react-native";

import { useAppTheme } from "../../theme/ThemeProvider";

type Props = {
  color?: string;
  size?: number;
};

// 细线旋转圈：自适应深浅主题色。
export function Spinner({ color, size = 14 }: Props) {
  const { palette } = useAppTheme();
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
        {
          borderColor: palette.hairlineStrong,
          borderRadius: size / 2,
          borderTopColor: color ?? palette.brand,
          height: size,
          transform: [{ rotate }],
          width: size,
        },
      ]}
    />
  );
}

const styles = StyleSheet.create({
  ring: {
    borderWidth: 2,
  },
});
