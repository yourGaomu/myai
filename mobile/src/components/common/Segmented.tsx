import { useEffect, useRef, useState } from "react";
import { Animated, Platform, Pressable, StyleSheet, Text, View } from "react-native";
import * as Haptics from "expo-haptics";

import { font, radius, weight } from "../../constants/theme";
import { useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { useReduceMotion } from "./MotionEffects";

type Option<T extends string> = {
  key: T;
  label: string;
};

type Props<T extends string> = {
  accessibilityLabel?: string;
  disabled?: boolean;
  onChange: (key: T) => void;
  options: Option<T>[];
  value: T;
};

// 分段控件：带弹簧平滑横滑胶囊底块与轻触感反馈。
export function Segmented<T extends string>({ accessibilityLabel, disabled, onChange, options, value }: Props<T>) {
  const styles = useThemedStyles(makeStyles);
  const reduceMotion = useReduceMotion();
  const [trackWidth, setTrackWidth] = useState(0);
  const count = Math.max(1, options.length);
  const activeIndex = Math.max(
    0,
    options.findIndex((opt) => opt.key === value),
  );
  const slideAnim = useRef(new Animated.Value(activeIndex)).current;

  useEffect(() => {
    if (reduceMotion) {
      slideAnim.setValue(activeIndex);
      return;
    }
    Animated.spring(slideAnim, {
      bounciness: 4,
      speed: 20,
      toValue: activeIndex,
      useNativeDriver: true,
    }).start();
  }, [activeIndex, reduceMotion, slideAnim]);

  const innerWidth = Math.max(0, trackWidth - 6);
  const itemWidth = innerWidth / count;
  const translateX = slideAnim.interpolate({
    inputRange: options.map((_, idx) => idx),
    outputRange: options.map((_, idx) => idx * itemWidth),
  });

  return (
    <View
      accessibilityLabel={accessibilityLabel}
      accessibilityRole="radiogroup"
      onLayout={(e) => setTrackWidth(e.nativeEvent.layout.width)}
      style={[styles.track, disabled && styles.disabled]}
    >
      {itemWidth > 0 ? (
        <Animated.View
          pointerEvents="none"
          style={[
            styles.slidingPill,
            {
              transform: [{ translateX }],
              width: itemWidth,
            },
          ]}
        />
      ) : null}
      {options.map((option) => {
        const active = option.key === value;
        return (
          <Pressable
            accessibilityRole="radio"
            accessibilityState={{ checked: active, disabled }}
            disabled={disabled}
            key={option.key}
            onPress={() => {
              if (!active) {
                if (Platform.OS !== "web") {
                  void Haptics.selectionAsync().catch(() => {});
                }
                onChange(option.key);
              }
            }}
            style={({ pressed }) => [
              styles.item,
              itemWidth === 0 && active && styles.itemActiveFallback,
              pressed && !active && styles.itemPressed,
            ]}
          >
            <Text numberOfLines={1} style={[styles.label, active && styles.labelActive]}>
              {option.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const makeStyles = ({ palette, shadow }: AppTheme) =>
  StyleSheet.create({
    track: {
      backgroundColor: palette.well,
      borderRadius: radius.md - 2,
      flexDirection: "row",
      overflow: "hidden",
      padding: 3,
      position: "relative",
    },
    slidingPill: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.sm - 1,
      borderWidth: 1,
      bottom: 3,
      left: 3,
      position: "absolute",
      top: 3,
      ...shadow.card,
    },
    item: {
      alignItems: "center",
      borderRadius: radius.sm - 1,
      flex: 1,
      justifyContent: "center",
      minHeight: 38,
      paddingHorizontal: 6,
      zIndex: 2,
    },
    itemActiveFallback: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderWidth: 1,
    },
    disabled: {
      opacity: 0.5,
    },
    itemPressed: {
      backgroundColor: palette.pressed,
    },
    label: {
      color: palette.ink2,
      fontSize: font.label,
      fontWeight: weight.regular,
    },
    labelActive: {
      color: palette.ink1,
      fontWeight: weight.semibold,
    },
  });
