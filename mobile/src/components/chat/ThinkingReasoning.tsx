import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AccessibilityInfo,
  Animated,
  Easing,
  Pressable,
  Text,
  View,
  type LayoutChangeEvent,
} from "react-native";

import type { ButtonFeedback } from "../../types/ui";
import { ShimmerLabel } from "./ShimmerLabel";
import { styles } from "./styles";

type Props = {
  buttonFeedback: ButtonFeedback;
  finishedAt?: string;
  reasoning: string;
  running: boolean;
  startedAt?: string;
};

export function ThinkingReasoning({
  buttonFeedback,
  finishedAt,
  reasoning,
  running,
  startedAt,
}: Props) {
  const [expanded, setExpanded] = useState(running);
  const [bodyHeight, setBodyHeight] = useState(0);
  const [reduceMotion, setReduceMotion] = useState(false);
  const [observedFinishedAt, setObservedFinishedAt] = useState<string>();
  const expansion = useRef(new Animated.Value(running ? 1 : 0)).current;
  const previousRunning = useRef(running);

  useEffect(() => {
    void AccessibilityInfo.isReduceMotionEnabled().then(setReduceMotion);
    const subscription = AccessibilityInfo.addEventListener("reduceMotionChanged", setReduceMotion);
    return () => subscription.remove();
  }, []);

  useEffect(() => {
    const wasRunning = previousRunning.current;
    if (running && !wasRunning) {
      setExpanded(true);
      setObservedFinishedAt(undefined);
    } else if (!running && wasRunning) {
      setObservedFinishedAt(new Date().toISOString());
      setExpanded(false);
    }
    previousRunning.current = running;
  }, [running]);

  useEffect(() => {
    const target = expanded ? 1 : 0;
    expansion.stopAnimation();
    if (reduceMotion) {
      expansion.setValue(target);
      return;
    }
    Animated.timing(expansion, {
      duration: 240,
      easing: Easing.out(Easing.cubic),
      toValue: target,
      useNativeDriver: false,
    }).start();
  }, [expanded, expansion, reduceMotion]);

  const summary = useMemo(
    () => thoughtSummary(startedAt, finishedAt || observedFinishedAt),
    [finishedAt, observedFinishedAt, startedAt],
  );
  const rememberBodyHeight = useCallback((event: LayoutChangeEvent) => {
    const height = Math.ceil(event.nativeEvent.layout.height);
    setBodyHeight((current) => (current === height ? current : height));
  }, []);
  const animatedHeight = expansion.interpolate({
    inputRange: [0, 1],
    outputRange: [0, bodyHeight],
  });
  const chevronRotation = expansion.interpolate({
    inputRange: [0, 1],
    outputRange: ["0deg", "180deg"],
  });

  return (
    <View style={styles.reasoningBlock}>
      <Pressable
        accessibilityLabel={running ? "正在思考" : summary}
        accessibilityRole="button"
        accessibilityState={{ expanded }}
        onPress={() => setExpanded((value) => !value)}
        style={({ pressed }) => buttonFeedback(styles.reasoningHeader, pressed)}
      >
        {running ? (
          <ShimmerLabel containerStyle={styles.reasoningShimmerLabel} label="正在思考" />
        ) : (
          <Text style={styles.reasoningSummaryText}>{summary}</Text>
        )}
        <Animated.Text
          accessible={false}
          style={[styles.reasoningChevron, { transform: [{ rotate: chevronRotation }] }]}
        >
          ⌄
        </Animated.Text>
      </Pressable>

      <Animated.View
        accessibilityElementsHidden={!expanded}
        importantForAccessibility={expanded ? "auto" : "no-hide-descendants"}
        style={[
          styles.reasoningBodyClip,
          {
            height: animatedHeight,
            opacity: expansion,
            pointerEvents: expanded ? "auto" : "none",
          },
        ]}
      >
        <View style={styles.reasoningBody}>
          <Text style={styles.reasoningText}>{reasoning}</Text>
        </View>
      </Animated.View>

      <View
        accessibilityElementsHidden
        importantForAccessibility="no-hide-descendants"
        onLayout={rememberBodyHeight}
        style={[styles.reasoningBody, styles.reasoningBodyMeasure]}
      >
        <Text style={styles.reasoningText}>{reasoning}</Text>
      </View>
    </View>
  );
}

function thoughtSummary(startedAt?: string, finishedAt?: string) {
  if (!startedAt || !finishedAt) {
    return "思考过程";
  }
  const started = Date.parse(startedAt);
  const finished = Date.parse(finishedAt);
  if (!Number.isFinite(started) || !Number.isFinite(finished) || finished < started) {
    return "思考过程";
  }
  return `思考了 ${formatDuration(finished - started)}`;
}

function formatDuration(durationMs: number) {
  const seconds = Math.max(1, Math.round(durationMs / 1000));
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return remainingSeconds ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
}
