import { useEffect, useRef, useState } from "react";
import { Animated, Easing, StyleSheet, Text, View } from "react-native";
import { LinearGradient } from "expo-linear-gradient";

import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { CharacterAvatar } from "../common/CharacterAvatar";

const loadingStatuses = ["正在启动...", "加载本地外观...", "连接中继通道..."];

type Props = {
  durationMs?: number;
  onDone: () => void;
  visible: boolean;
};

export function StartupLoadingOverlay({ durationMs = 2000, onDone, visible }: Props) {
  const theme = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const { palette } = theme;
  const [statusIndex, setStatusIndex] = useState(0);
  const opacity = useRef(new Animated.Value(0)).current;
  const translateY = useRef(new Animated.Value(12)).current;
  const progress = useRef(new Animated.Value(0.12)).current;

  useEffect(() => {
    if (!visible) {
      return;
    }

    setStatusIndex(0);
    opacity.setValue(0);
    translateY.setValue(12);
    progress.setValue(0.12);

    const stepTimer = setInterval(() => {
      setStatusIndex((current) => Math.min(loadingStatuses.length - 1, current + 1));
    }, Math.max(400, Math.floor(durationMs / 3)));

    const finishTimer = setTimeout(() => {
      Animated.parallel([
        Animated.timing(opacity, { duration: 220, toValue: 0, useNativeDriver: true }),
        Animated.timing(translateY, { duration: 220, toValue: -8, useNativeDriver: true }),
      ]).start(({ finished }) => {
        if (finished) {
          onDone();
        }
      });
    }, durationMs);

    Animated.parallel([
      Animated.timing(opacity, { duration: 260, toValue: 1, useNativeDriver: true }),
      Animated.spring(translateY, { bounciness: 5, speed: 16, toValue: 0, useNativeDriver: true }),
      Animated.timing(progress, {
        duration: Math.max(500, durationMs - 150),
        easing: Easing.out(Easing.cubic),
        toValue: 1,
        useNativeDriver: false,
      }),
    ]).start();

    return () => {
      clearInterval(stepTimer);
      clearTimeout(finishTimer);
      opacity.stopAnimation();
      translateY.stopAnimation();
      progress.stopAnimation();
    };
  }, [durationMs, onDone, opacity, progress, translateY, visible]);

  if (!visible) {
    return null;
  }

  const barWidth = progress.interpolate({
    inputRange: [0, 1],
    outputRange: ["12%", "100%"],
  });

  return (
    <View pointerEvents="auto" style={styles.overlay}>
      <Animated.View style={[styles.content, { opacity, transform: [{ translateY }] }]}>
        <View style={styles.characterWrap}>
          <CharacterAvatar accessibilityLabel="MYAI 正在启动" height={310} size={156} slot="startup" variant="full" />
        </View>
        <Text style={styles.title}>MYAI</Text>
        <Text style={styles.status}>{loadingStatuses[statusIndex]}</Text>
        <View style={styles.progressTrack}>
          <Animated.View style={[styles.progressBarWrap, { width: barWidth }]}>
            <LinearGradient
              colors={[palette.brandLight, palette.brand]}
              end={{ x: 1, y: 0 }}
              start={{ x: 0, y: 0 }}
              style={styles.progressFill}
            />
          </Animated.View>
        </View>
      </Animated.View>
    </View>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    overlay: {
      alignItems: "center",
      backgroundColor: palette.canvas,
      bottom: 0,
      justifyContent: "center",
      left: 0,
      position: "absolute",
      right: 0,
      top: 0,
      zIndex: 100,
    },
    content: {
      alignItems: "center",
      justifyContent: "center",
      width: "100%",
    },
    characterWrap: {
      alignItems: "center",
      justifyContent: "center",
      marginBottom: 4,
    },
    title: {
      color: palette.ink1,
      fontSize: font.display,
      fontWeight: weight.semibold,
      letterSpacing: 1.5,
    },
    status: {
      color: palette.ink2,
      fontSize: font.label,
      fontWeight: weight.medium,
      marginTop: 4,
    },
    progressTrack: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      height: 6,
      marginTop: 14,
      overflow: "hidden",
      width: 156,
    },
    progressBarWrap: {
      height: "100%",
      overflow: "hidden",
    },
    progressFill: {
      height: "100%",
      width: "100%",
    },
  });
