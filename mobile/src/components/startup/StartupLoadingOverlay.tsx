import { useEffect, useRef, useState } from "react";
import { Animated, Easing, StyleSheet, Text, View } from "react-native";

import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { CharacterHalo, ShimmerProgressBar, useReduceMotion } from "../common/MotionEffects";

const loadingStatuses = ["正在启动...", "加载本地外观...", "连接中继通道..."];

type Props = {
  durationMs?: number;
  onDone: () => void;
  visible: boolean;
};

export function StartupLoadingOverlay({ durationMs = 2000, onDone, visible }: Props) {
  const styles = useThemedStyles(makeStyles);
  const reduceMotion = useReduceMotion();
  const [statusIndex, setStatusIndex] = useState(0);
  const [progressPct, setProgressPct] = useState(18);
  const opacity = useRef(new Animated.Value(0)).current;
  const translateY = useRef(new Animated.Value(14)).current;
  const statusOpacity = useRef(new Animated.Value(1)).current;
  const auroraDrift = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    if (!visible) {
      return;
    }

    setStatusIndex(0);
    setProgressPct(22);
    opacity.setValue(0);
    translateY.setValue(reduceMotion ? 0 : 14);
    statusOpacity.setValue(1);
    auroraDrift.setValue(0);

    let auroraLoop: Animated.CompositeAnimation | null = null;
    if (!reduceMotion) {
      auroraLoop = Animated.loop(
        Animated.sequence([
          Animated.timing(auroraDrift, {
            duration: 2400,
            easing: Easing.inOut(Easing.sin),
            toValue: 1,
            useNativeDriver: true,
          }),
          Animated.timing(auroraDrift, {
            duration: 2400,
            easing: Easing.inOut(Easing.sin),
            toValue: 0,
            useNativeDriver: true,
          }),
        ]),
      );
      auroraLoop.start();
    }

    const stepInterval = Math.max(420, Math.floor(durationMs / 3));
    const stepTimer = setInterval(() => {
      if (!reduceMotion) {
        Animated.sequence([
          Animated.timing(statusOpacity, { duration: 110, toValue: 0.25, useNativeDriver: true }),
          Animated.timing(statusOpacity, { duration: 170, toValue: 1, useNativeDriver: true }),
        ]).start();
      }
      setStatusIndex((current) => {
        const next = Math.min(loadingStatuses.length - 1, current + 1);
        setProgressPct(next === 1 ? 64 : 100);
        return next;
      });
    }, stepInterval);

    const finishTimer = setTimeout(() => {
      Animated.parallel([
        Animated.timing(opacity, { duration: 220, toValue: 0, useNativeDriver: true }),
        Animated.timing(translateY, { duration: 220, toValue: reduceMotion ? 0 : -8, useNativeDriver: true }),
      ]).start(({ finished }) => {
        if (finished) {
          onDone();
        }
      });
    }, durationMs);

    Animated.parallel([
      Animated.timing(opacity, { duration: 260, toValue: 1, useNativeDriver: true }),
      Animated.spring(translateY, { bounciness: 5, speed: 16, toValue: 0, useNativeDriver: true }),
    ]).start();

    return () => {
      clearInterval(stepTimer);
      clearTimeout(finishTimer);
      auroraLoop?.stop();
      opacity.stopAnimation();
      translateY.stopAnimation();
      statusOpacity.stopAnimation();
    };
  }, [auroraDrift, durationMs, onDone, opacity, reduceMotion, statusOpacity, translateY, visible]);

  if (!visible) {
    return null;
  }

  const orbScale = auroraDrift.interpolate({
    inputRange: [0, 1],
    outputRange: [0.96, 1.12],
  });

  return (
    <View pointerEvents="auto" style={styles.overlay}>
      <Animated.View
        pointerEvents="none"
        style={[
          styles.auroraPrimary,
          {
            transform: [{ scale: orbScale }],
          },
        ]}
      />
      <View pointerEvents="none" style={styles.auroraSecondary} />

      <Animated.View style={[styles.content, { opacity, transform: [{ translateY }] }]}>
        <View style={styles.characterWrap}>
          <CharacterHalo ringSize={154}>
            <CharacterAvatar accessibilityLabel="MYAI 正在启动" height={300} size={156} slot="startup" variant="full" />
          </CharacterHalo>
        </View>
        <Text style={styles.title}>MYAI</Text>
        <Animated.Text style={[styles.status, { opacity: statusOpacity }]}>
          {loadingStatuses[statusIndex]}
        </Animated.Text>
        <View style={styles.progressWrap}>
          <ShimmerProgressBar active height={6} progress={progressPct} />
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
      overflow: "hidden",
      position: "absolute",
      right: 0,
      top: 0,
      zIndex: 100,
    },
    auroraPrimary: {
      backgroundColor: palette.ambientGlow,
      borderRadius: 160,
      height: 290,
      position: "absolute",
      top: "14%",
      width: 290,
    },
    auroraSecondary: {
      backgroundColor: palette.brandWash,
      borderRadius: 140,
      bottom: "12%",
      height: 240,
      opacity: 0.7,
      position: "absolute",
      right: -50,
      width: 240,
    },
    content: {
      alignItems: "center",
      justifyContent: "center",
      width: "100%",
    },
    characterWrap: {
      alignItems: "center",
      justifyContent: "center",
      marginBottom: 6,
    },
    title: {
      color: palette.ink1,
      fontSize: font.display + 2,
      fontWeight: weight.bold,
      letterSpacing: 4,
    },
    status: {
      color: palette.ink2,
      fontSize: font.label,
      fontWeight: weight.medium,
      marginTop: 6,
    },
    progressWrap: {
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      marginTop: 18,
      overflow: "hidden",
      width: 186,
    },
  });
