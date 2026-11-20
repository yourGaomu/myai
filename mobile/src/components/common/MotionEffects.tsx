import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  AccessibilityInfo,
  Animated,
  Easing,
  StyleSheet,
  View,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { LinearGradient } from "expo-linear-gradient";

import { radius } from "../../constants/theme";
import { useAppTheme } from "../../theme/ThemeProvider";

export function useReduceMotion(): boolean {
  const [reduceMotion, setReduceMotion] = useState(false);

  useEffect(() => {
    let mounted = true;
    void AccessibilityInfo.isReduceMotionEnabled().then((enabled) => {
      if (mounted) setReduceMotion(Boolean(enabled));
    });
    const sub = AccessibilityInfo.addEventListener("reduceMotionChanged", (enabled) => {
      if (mounted) setReduceMotion(Boolean(enabled));
    });
    return () => {
      mounted = false;
      sub.remove();
    };
  }, []);

  return reduceMotion;
}

type PulseDotProps = {
  color: string;
  pulse?: boolean;
  size?: number;
};

// 状态呼吸点：核心圆点 + 外层扩散脉冲环（开启减少动态效果或非激活态时自动静止）。
export function PulseDot({ color, pulse = true, size = 7 }: PulseDotProps) {
  const reduceMotion = useReduceMotion();
  const anim = useRef(new Animated.Value(0)).current;
  const active = pulse && !reduceMotion;

  useEffect(() => {
    anim.stopAnimation();
    anim.setValue(0);
    if (!active) return;

    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(anim, {
          duration: 1650,
          easing: Easing.out(Easing.cubic),
          toValue: 1,
          useNativeDriver: true,
        }),
        Animated.delay(250),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [active, anim]);

  const scale = anim.interpolate({
    inputRange: [0, 1],
    outputRange: [0.95, 2.25],
  });
  const opacity = anim.interpolate({
    inputRange: [0, 0.75, 1],
    outputRange: [0.55, 0.12, 0],
  });

  return (
    <View style={[styles.dotWrap, { height: size, width: size }]}>
      {active ? (
        <Animated.View
          pointerEvents="none"
          style={[
            styles.dotRing,
            {
              backgroundColor: color,
              borderRadius: size,
              height: size,
              opacity,
              transform: [{ scale }],
              width: size,
            },
          ]}
        />
      ) : null}
      <View
        style={{
          backgroundColor: color,
          borderRadius: size / 2,
          height: size,
          width: size,
        }}
      />
    </View>
  );
}

type CharacterHaloProps = {
  bob?: boolean;
  children: ReactNode;
  ringSize?: number;
};

// 角色背景双层呼吸光环 + 轻微悬浮动画。
export function CharacterHalo({ bob = true, children, ringSize = 132 }: CharacterHaloProps) {
  const { palette } = useAppTheme();
  const reduceMotion = useReduceMotion();
  const ripple = useRef(new Animated.Value(0)).current;
  const floatY = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    ripple.stopAnimation();
    floatY.stopAnimation();
    ripple.setValue(0);
    floatY.setValue(0);
    if (reduceMotion) return;

    const rippleLoop = Animated.loop(
      Animated.timing(ripple, {
        duration: 2800,
        easing: Easing.out(Easing.cubic),
        toValue: 1,
        useNativeDriver: true,
      }),
    );
    rippleLoop.start();

    let bobLoop: Animated.CompositeAnimation | null = null;
    if (bob) {
      bobLoop = Animated.loop(
        Animated.sequence([
          Animated.timing(floatY, {
            duration: 1850,
            easing: Easing.inOut(Easing.sin),
            toValue: -4.5,
            useNativeDriver: true,
          }),
          Animated.timing(floatY, {
            duration: 1850,
            easing: Easing.inOut(Easing.sin),
            toValue: 0,
            useNativeDriver: true,
          }),
        ]),
      );
      bobLoop.start();
    }

    return () => {
      rippleLoop.stop();
      bobLoop?.stop();
    };
  }, [bob, floatY, reduceMotion, ripple]);

  const rippleScale = ripple.interpolate({
    inputRange: [0, 1],
    outputRange: [0.9, 1.24],
  });
  const rippleOpacity = ripple.interpolate({
    inputRange: [0, 0.7, 1],
    outputRange: [0.42, 0.14, 0],
  });

  return (
    <View style={styles.haloContainer}>
      <View
        pointerEvents="none"
        style={[
          styles.haloStaticRing,
          {
            backgroundColor: palette.brandWash,
            borderColor: palette.brandDeep,
            borderRadius: ringSize / 2,
            height: ringSize,
            width: ringSize,
          },
        ]}
      />
      {!reduceMotion ? (
        <Animated.View
          pointerEvents="none"
          style={[
            styles.haloPulseRing,
            {
              borderColor: palette.brand,
              borderRadius: ringSize / 2,
              height: ringSize,
              opacity: rippleOpacity,
              transform: [{ scale: rippleScale }],
              width: ringSize,
            },
          ]}
        />
      ) : null}
      <Animated.View style={{ transform: [{ translateY: floatY }] }}>{children}</Animated.View>
    </View>
  );
}

type ShimmerProgressBarProps = {
  active?: boolean;
  height?: number;
  progress: number; // 0..100
  style?: StyleProp<ViewStyle>;
  tone?: "brand" | "ok" | "warn";
  trackColor?: string;
};

// 平滑充能 + 流光扫过进度条（用于计划进度、上下文容量、子代理进度）。
export function ShimmerProgressBar({
  active = false,
  height = 5,
  progress,
  style,
  tone = "brand",
  trackColor,
}: ShimmerProgressBarProps) {
  const { palette } = useAppTheme();
  const reduceMotion = useReduceMotion();
  const clamped = Math.max(0, Math.min(100, Number.isFinite(progress) ? progress : 0));
  const widthAnim = useRef(new Animated.Value(clamped)).current;
  const sweepAnim = useRef(new Animated.Value(0)).current;
  const [trackWidth, setTrackWidth] = useState(220);

  useEffect(() => {
    if (reduceMotion) {
      widthAnim.setValue(clamped);
      return;
    }
    Animated.timing(widthAnim, {
      duration: 420,
      easing: Easing.out(Easing.cubic),
      toValue: clamped,
      useNativeDriver: false,
    }).start();
  }, [clamped, reduceMotion, widthAnim]);

  useEffect(() => {
    sweepAnim.stopAnimation();
    sweepAnim.setValue(0);
    if (!active || reduceMotion) return;

    const loop = Animated.loop(
      Animated.timing(sweepAnim, {
        duration: 1350,
        easing: Easing.inOut(Easing.ease),
        toValue: 1,
        useNativeDriver: true,
      }),
    );
    loop.start();
    return () => loop.stop();
  }, [active, reduceMotion, sweepAnim]);

  const colors: [string, string] =
    tone === "ok"
      ? [palette.ok, palette.ok]
      : tone === "warn"
        ? [palette.warn, palette.err]
        : [palette.brandLight, palette.brand];

  const fillWidth = widthAnim.interpolate({
    inputRange: [0, 100],
    outputRange: ["0%", "100%"],
  });

  const translateX = sweepAnim.interpolate({
    inputRange: [0, 1],
    outputRange: [-56, Math.max(120, trackWidth + 56)],
  });

  return (
    <View
      onLayout={(e) => {
        const w = e.nativeEvent.layout.width;
        if (w > 0 && Math.abs(w - trackWidth) > 2) setTrackWidth(w);
      }}
      style={[
        styles.barTrack,
        {
          backgroundColor: trackColor ?? palette.surface3,
          borderRadius: height,
          height,
        },
        style,
      ]}
    >
      <Animated.View style={[styles.barFillWrap, { borderRadius: height, width: fillWidth }]}>
        <LinearGradient
          colors={colors}
          end={{ x: 1, y: 0 }}
          start={{ x: 0, y: 0 }}
          style={StyleSheet.absoluteFill}
        />
        {active && !reduceMotion ? (
          <Animated.View
            pointerEvents="none"
            style={[
              styles.shimmerStreak,
              {
                transform: [{ translateX }],
              },
            ]}
          >
            <LinearGradient
              colors={["rgba(255,255,255,0)", "rgba(255,255,255,0.58)", "rgba(255,255,255,0)"]}
              end={{ x: 1, y: 0 }}
              start={{ x: 0, y: 0 }}
              style={StyleSheet.absoluteFill}
            />
          </Animated.View>
        ) : null}
      </Animated.View>
    </View>
  );
}

// 运行中卡片左侧垂直流光轨
export function RunningEdgeRail({ active }: { active: boolean }) {
  const { palette } = useAppTheme();
  const reduceMotion = useReduceMotion();
  const pulse = useRef(new Animated.Value(0.55)).current;

  useEffect(() => {
    pulse.stopAnimation();
    if (!active || reduceMotion) {
      pulse.setValue(0.85);
      return;
    }
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, {
          duration: 850,
          easing: Easing.inOut(Easing.sin),
          toValue: 1,
          useNativeDriver: true,
        }),
        Animated.timing(pulse, {
          duration: 850,
          easing: Easing.inOut(Easing.sin),
          toValue: 0.35,
          useNativeDriver: true,
        }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [active, pulse, reduceMotion]);

  if (!active) return null;

  return (
    <Animated.View pointerEvents="none" style={[styles.edgeRail, { opacity: pulse }]}>
      <LinearGradient
        colors={[palette.brandLight, palette.brand, palette.brandDark]}
        end={{ x: 0, y: 1 }}
        start={{ x: 0, y: 0 }}
        style={StyleSheet.absoluteFill}
      />
    </Animated.View>
  );
}

// 列表加载流光骨架屏
export function SkeletonRows({ count = 4 }: { count?: number }) {
  const { palette } = useAppTheme();
  const reduceMotion = useReduceMotion();
  const pulse = useRef(new Animated.Value(0.45)).current;

  useEffect(() => {
    pulse.stopAnimation();
    if (reduceMotion) {
      pulse.setValue(0.6);
      return;
    }
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, { duration: 750, toValue: 0.9, useNativeDriver: true }),
        Animated.timing(pulse, { duration: 750, toValue: 0.42, useNativeDriver: true }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [pulse, reduceMotion]);

  const widths: Array<`${number}%`> = ["64%", "48%", "76%", "54%"];

  return (
    <View style={styles.skelList}>
      {Array.from({ length: count }).map((_, index) => (
        <Animated.View
          key={`skel-${index}`}
          style={[
            styles.skelRow,
            {
              backgroundColor: palette.surface1,
              borderColor: palette.hairline,
              opacity: pulse,
            },
          ]}
        >
          <View style={[styles.skelIcon, { backgroundColor: palette.surface3 }]} />
          <View style={styles.skelBody}>
            <View
              style={[
                styles.skelLine,
                { backgroundColor: palette.surface3, width: widths[index % widths.length] },
              ]}
            />
            <View style={[styles.skelSub, { backgroundColor: palette.surface2, width: "36%" }]} />
          </View>
        </Animated.View>
      ))}
    </View>
  );
}

// 二级视图/题目切换时的轻量滑入淡入包装器
export function FadeSlideView({
  children,
  direction = "right",
  keyProp,
  style,
}: {
  children: ReactNode;
  direction?: "right" | "up";
  keyProp: string;
  style?: StyleProp<ViewStyle>;
}) {
  const reduceMotion = useReduceMotion();
  const opacity = useRef(new Animated.Value(1)).current;
  const offset = useRef(new Animated.Value(0)).current;
  const firstMount = useRef(true);

  useEffect(() => {
    if (firstMount.current) {
      firstMount.current = false;
      return;
    }
    if (reduceMotion) {
      opacity.setValue(1);
      offset.setValue(0);
      return;
    }
    opacity.setValue(0);
    offset.setValue(direction === "right" ? 16 : 10);
    Animated.parallel([
      Animated.timing(opacity, {
        duration: 210,
        easing: Easing.out(Easing.cubic),
        toValue: 1,
        useNativeDriver: true,
      }),
      Animated.timing(offset, {
        duration: 230,
        easing: Easing.out(Easing.cubic),
        toValue: 0,
        useNativeDriver: true,
      }),
    ]).start();
  }, [direction, keyProp, offset, opacity, reduceMotion]);

  const transform =
    direction === "right" ? [{ translateX: offset }] : [{ translateY: offset }];

  return <Animated.View style={[style, { opacity, transform }]}>{children}</Animated.View>;
}

const styles = StyleSheet.create({
  dotWrap: {
    alignItems: "center",
    justifyContent: "center",
  },
  dotRing: {
    position: "absolute",
  },
  haloContainer: {
    alignItems: "center",
    justifyContent: "center",
  },
  haloStaticRing: {
    borderWidth: 1.5,
    opacity: 0.72,
    position: "absolute",
  },
  haloPulseRing: {
    borderWidth: 1,
    position: "absolute",
  },
  barTrack: {
    overflow: "hidden",
    width: "100%",
  },
  barFillWrap: {
    height: "100%",
    overflow: "hidden",
    position: "relative",
  },
  shimmerStreak: {
    bottom: 0,
    left: 0,
    position: "absolute",
    top: 0,
    width: 48,
  },
  edgeRail: {
    borderBottomRightRadius: 3,
    borderTopRightRadius: 3,
    bottom: 8,
    left: 0,
    overflow: "hidden",
    position: "absolute",
    top: 8,
    width: 3,
  },
  skelList: {
    gap: 8,
    paddingVertical: 6,
  },
  skelRow: {
    alignItems: "center",
    borderRadius: radius.md,
    borderWidth: 1,
    flexDirection: "row",
    gap: 11,
    minHeight: 50,
    paddingHorizontal: 12,
    paddingVertical: 9,
  },
  skelIcon: {
    borderRadius: radius.sm,
    height: 26,
    width: 26,
  },
  skelBody: {
    flex: 1,
    gap: 6,
  },
  skelLine: {
    borderRadius: 4,
    height: 11,
  },
  skelSub: {
    borderRadius: 4,
    height: 8,
  },
});
