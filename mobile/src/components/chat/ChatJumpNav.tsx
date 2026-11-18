import { useEffect, useRef, useState } from "react";
import { Animated, Easing, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import type { ButtonFeedback } from "../../types/ui";
import { radius, weight } from "../../constants/theme";
import { useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { useReduceMotion } from "../common/MotionEffects";

export type ChatJumpAnchor = {
  id: string;
  index: number;
  title: string;
};

type Props = {
  anchors: ChatJumpAnchor[];
  buttonFeedback: ButtonFeedback;
  onJump: (id: string) => void;
  onToggle: () => void;
  open: boolean;
};

const maxRailMarkers = 12;
const waveWidths = [7, 11, 8, 12, 9, 11];

export function ChatJumpNav({ anchors, buttonFeedback, onJump, onToggle, open }: Props) {
  const styles = useThemedStyles(createStyles);
  const reduceMotion = useReduceMotion();
  const [mounted, setMounted] = useState(open);
  const anim = useRef(new Animated.Value(open ? 1 : 0)).current;

  useEffect(() => {
    if (open) {
      setMounted(true);
      if (reduceMotion) {
        anim.setValue(1);
        return;
      }
      Animated.spring(anim, {
        bounciness: 5,
        speed: 20,
        toValue: 1,
        useNativeDriver: true,
      }).start();
      return;
    }
    if (!mounted) return;
    if (reduceMotion) {
      anim.setValue(0);
      setMounted(false);
      return;
    }
    Animated.timing(anim, {
      duration: 150,
      easing: Easing.in(Easing.quad),
      toValue: 0,
      useNativeDriver: true,
    }).start(({ finished }) => {
      if (finished) setMounted(false);
    });
  }, [anim, mounted, open, reduceMotion]);

  if (anchors.length < 2) {
    return null;
  }

  const markers = compactMarkers(anchors);
  const panelScale = anim.interpolate({ inputRange: [0, 1], outputRange: [0.9, 1] });
  const panelTranslateX = anim.interpolate({ inputRange: [0, 1], outputRange: [10, 0] });

  return (
    <View pointerEvents="box-none" style={styles.container}>
      {mounted ? (
        <Animated.View
          onStartShouldSetResponder={() => true}
          style={[
            styles.panel,
            {
              opacity: anim,
              transform: [{ translateX: panelTranslateX }, { scale: panelScale }],
            },
          ]}
        >
          <View style={styles.panelHeader}>
            <Text style={styles.panelTitle}>跳转消息</Text>
            <Pressable onPress={onToggle} style={({ pressed }) => buttonFeedback(styles.closeButton, pressed)}>
              <Text style={styles.closeText}>隐藏</Text>
            </Pressable>
          </View>
          <ScrollView
            bounces={false}
            contentContainerStyle={styles.anchorList}
            keyboardShouldPersistTaps="handled"
            nestedScrollEnabled
            overScrollMode="never"
            showsVerticalScrollIndicator
            style={styles.anchorScroll}
          >
            {anchors.map((anchor) => (
              <Pressable
                key={anchor.id}
                onPress={() => onJump(anchor.id)}
                style={({ pressed }) => buttonFeedback(styles.anchorItem, pressed)}
              >
                <Text style={styles.anchorIndex}>Q{anchor.index}</Text>
                <Text numberOfLines={2} style={styles.anchorTitle}>{anchor.title}</Text>
              </Pressable>
            ))}
          </ScrollView>
        </Animated.View>
      ) : null}

      <Pressable onPress={onToggle} style={({ pressed }) => buttonFeedback([styles.railButton, open && styles.railButtonActive], pressed)}>
        <View style={styles.railTrack}>
          {markers.map((anchor, idx) => (
            <View
              key={anchor.id}
              style={[
                styles.railMarker,
                idx === markers.length - 1 && styles.railMarkerLatest,
                { width: waveWidths[idx % waveWidths.length] },
              ]}
            />
          ))}
        </View>
        <Text style={styles.railText}>{anchors.length}</Text>
      </Pressable>
    </View>
  );
}

function compactMarkers(anchors: ChatJumpAnchor[]) {
  if (anchors.length <= maxRailMarkers) {
    return anchors;
  }

  const step = (anchors.length - 1) / (maxRailMarkers - 1);
  const result: ChatJumpAnchor[] = [];
  for (let index = 0; index < maxRailMarkers; index += 1) {
    result.push(anchors[Math.round(index * step)]);
  }
  return result;
}

const createStyles = ({ font, palette, shadow }: AppTheme) =>
  StyleSheet.create({
    container: {
      alignItems: "flex-end",
      bottom: 12,
      justifyContent: "flex-start",
      position: "absolute",
      right: 8,
      top: 10,
      zIndex: 20,
    },
    railButton: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      gap: 5,
      justifyContent: "center",
      maxHeight: 128,
      overflow: "hidden",
      paddingHorizontal: 5,
      paddingVertical: 9,
      width: 26,
      ...shadow.card,
    },
    railButtonActive: {
      borderColor: palette.brandDeep,
    },
    railTrack: {
      alignItems: "center",
      gap: 3,
      justifyContent: "center",
      maxHeight: 88,
      overflow: "hidden",
    },
    railMarker: {
      backgroundColor: palette.hairlineStrong,
      borderRadius: radius.xxs,
      height: 2,
      width: 10,
    },
    railMarkerLatest: {
      backgroundColor: palette.brand,
    },
    railText: {
      color: palette.ink2,
      fontSize: font.micro,
      fontWeight: weight.semibold,
    },
    panel: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.lg,
      borderWidth: 1,
      gap: 6,
      maxHeight: 280,
      overflow: "hidden",
      padding: 8,
      position: "absolute",
      right: 34,
      ...shadow.float,
      top: 0,
      width: 248,
      zIndex: 30,
    },
    panelHeader: {
      alignItems: "center",
      flexDirection: "row",
      gap: 8,
      justifyContent: "space-between",
      paddingLeft: 6,
    },
    panelTitle: {
      color: palette.ink1,
      fontSize: font.label + 0.5,
      fontWeight: weight.semibold,
    },
    closeButton: {
      alignItems: "center",
      backgroundColor: palette.surface1,
      borderRadius: radius.pill,
      justifyContent: "center",
      minHeight: 26,
      paddingHorizontal: 9,
    },
    closeText: {
      color: palette.ink2,
      fontSize: font.micro,
      fontWeight: weight.semibold,
    },
    anchorScroll: {
      maxHeight: 232,
    },
    anchorList: {
      gap: 4,
    },
    anchorItem: {
      alignItems: "flex-start",
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.sm,
      borderWidth: 1,
      flexDirection: "row",
      gap: 6,
      paddingHorizontal: 8,
      paddingVertical: 6,
    },
    anchorIndex: {
      color: palette.brandLight,
      fontSize: font.micro,
      fontWeight: weight.semibold,
      marginTop: 1,
      minWidth: 22,
    },
    anchorTitle: {
      color: palette.ink1,
      flex: 1,
      fontSize: font.caption,
      lineHeight: 16,
    },
  });
