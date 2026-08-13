import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AccessibilityInfo,
  Animated,
  Easing,
  Text,
  View,
  type LayoutChangeEvent,
} from "react-native";

import { styles } from "./styles";

type Props = {
  label: string;
};

export function AssistantLoadingBubble({ label }: Props) {
  const progress = useRef(new Animated.Value(0)).current;
  const labelOpacity = useRef(new Animated.Value(1)).current;
  const [reduceMotion, setReduceMotion] = useState(false);
  const [visibleLabel, setVisibleLabel] = useState(label);
  const [labelSize, setLabelSize] = useState({ height: 0, width: 0 });

  useEffect(() => {
    void AccessibilityInfo.isReduceMotionEnabled().then(setReduceMotion);
    const subscription = AccessibilityInfo.addEventListener("reduceMotionChanged", setReduceMotion);
    return () => subscription.remove();
  }, []);

  useEffect(() => {
    progress.stopAnimation();
    progress.setValue(0);
    if (reduceMotion) {
      return;
    }

    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(progress, {
          duration: 1400,
          easing: Easing.inOut(Easing.ease),
          toValue: 1,
          useNativeDriver: true,
        }),
        Animated.delay(300),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [progress, reduceMotion]);

  useEffect(() => {
    if (label === visibleLabel) {
      return;
    }

    labelOpacity.stopAnimation();
    if (reduceMotion) {
      setVisibleLabel(label);
      labelOpacity.setValue(1);
      return;
    }

    Animated.timing(labelOpacity, {
      duration: 100,
      toValue: 0,
      useNativeDriver: true,
    }).start(({ finished }) => {
      if (!finished) {
        return;
      }
      setVisibleLabel(label);
      labelOpacity.setValue(0);
      Animated.timing(labelOpacity, {
        duration: 160,
        toValue: 1,
        useNativeDriver: true,
      }).start();
    });
  }, [label, labelOpacity, reduceMotion, visibleLabel]);

  const sweep = useMemo(
    () =>
      progress.interpolate({
        inputRange: [0, 1],
        outputRange: [-34, labelSize.width + 34],
      }),
    [labelSize.width, progress],
  );
  const highlightSweep = useMemo(() => Animated.add(sweep, 9), [sweep]);
  const rememberLabelSize = useCallback((event: LayoutChangeEvent) => {
    const { height, width } = event.nativeEvent.layout;
    setLabelSize((current) =>
      current.height === height && current.width === width ? current : { height, width },
    );
  }, []);

  return (
    <View
      accessibilityLabel={`AI ${visibleLabel}`}
      accessibilityLiveRegion="polite"
      accessible
      style={[styles.message, styles.assistantMessage, styles.assistantLoadingMessage]}
    >
      <Animated.View style={[styles.assistantLoadingLabel, { opacity: labelOpacity }]}>
        <Text accessible={false} onLayout={rememberLabelSize} style={styles.assistantLoadingText}>
          {visibleLabel}
        </Text>
        {!reduceMotion && labelSize.width > 0 ? (
          <>
            <Animated.View
              style={[
                styles.assistantLoadingSoftWindow,
                {
                  height: labelSize.height,
                  transform: [{ translateX: sweep }],
                },
              ]}
            >
              <Animated.Text
                accessible={false}
                style={[
                  styles.assistantLoadingText,
                  styles.assistantLoadingSoftText,
                  {
                    width: labelSize.width,
                    transform: [{ translateX: Animated.multiply(sweep, -1) }],
                  },
                ]}
              >
                {visibleLabel}
              </Animated.Text>
            </Animated.View>
            <Animated.View
              style={[
                styles.assistantLoadingHighlightWindow,
                {
                  height: labelSize.height,
                  transform: [{ translateX: highlightSweep }],
                },
              ]}
            >
              <Animated.Text
                accessible={false}
                style={[
                  styles.assistantLoadingText,
                  styles.assistantLoadingHighlightText,
                  {
                    width: labelSize.width,
                    transform: [{ translateX: Animated.multiply(highlightSweep, -1) }],
                  },
                ]}
              >
                {visibleLabel}
              </Animated.Text>
            </Animated.View>
          </>
        ) : null}
      </Animated.View>
    </View>
  );
}
