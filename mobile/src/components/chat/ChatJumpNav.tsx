import { useEffect, useRef } from "react";
import { Animated, Platform, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

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
  topClearance: number;
  viewportHeight: number;
};

const railSize = 44;
const railInset = 8;
export const chatJumpRailClearance = railSize + railInset + 6;
const maxRailMarkers = 12;
const waveWidths = [7, 11, 8, 12, 9, 11];

export function ChatJumpNav({ anchors, buttonFeedback, onJump, onToggle, open, topClearance, viewportHeight }: Props) {
  const styles = useThemedStyles(createStyles);
  const reduceMotion = useReduceMotion();
  const anim = useRef(new Animated.Value(0)).current;
  const railRef = useRef<View | null>(null);

  useEffect(() => {
    if (!open || reduceMotion) {
      anim.setValue(open ? 1 : 0);
      return;
    }
    const animation = Animated.spring(anim, {
      bounciness: 5,
      speed: 20,
      toValue: 1,
      useNativeDriver: true,
    });
    animation.start();
    return () => animation.stop();
  }, [anim, open, reduceMotion]);

  if (anchors.length < 2) return null;

  const close = () => {
    onToggle();
    if (Platform.OS === "web") railRef.current?.focus();
  };
  const webKeyboardHandlers = Platform.OS === "web" ? {
    onKeyDown: (event: { key: string; stopPropagation: () => void }) => {
      if (open && event.key === "Escape") {
        event.stopPropagation();
        close();
      }
    },
  } : {};
  const markers = compactMarkers(anchors);
  const panelTranslateX = anim.interpolate({ inputRange: [0, 1], outputRange: [10, 0] });
  // Leave room for the close button and the return-to-bottom action when the keyboard is open.
  const bottomSpace = 56;
  const top = viewportHeight > 0 ? Math.min(topClearance, Math.max(0, viewportHeight - bottomSpace - 96)) : topClearance;
  const maxHeight = viewportHeight > 0 ? Math.min(280, Math.max(0, viewportHeight - top - bottomSpace)) : 280;

  return (
    <View {...webKeyboardHandlers} pointerEvents="box-none" style={[styles.container, { top, height: open ? Math.max(railSize, maxHeight) : railSize }]}>
      <Pressable
        accessibilityLabel={`跳转消息，共 ${anchors.length} 条`}
        accessibilityRole="button"
        accessibilityState={{ expanded: open }}
        onPress={onToggle}
        ref={railRef}
        style={({ pressed }) => buttonFeedback([styles.railButton, open && styles.railButtonActive], pressed)}
      >
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

      {open ? (
        <View pointerEvents="box-none" style={styles.panelSlot}>
          <Animated.View
            style={[
              styles.panel,
              { maxHeight, opacity: anim, transform: [{ translateX: panelTranslateX }] },
            ]}
          >
            <View style={styles.panelHeader}>
              <Text accessibilityRole="header" numberOfLines={1} style={styles.panelTitle}>跳转消息</Text>
              <Pressable
                accessibilityLabel="关闭消息跳转"
                accessibilityRole="button"
                onPress={close}
                style={({ pressed }) => buttonFeedback(styles.closeButton, pressed)}
              >
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
                  accessibilityLabel={`Q${anchor.index}，${anchor.title}`}
                  accessibilityRole="button"
                  key={anchor.id}
                  onPress={() => {
                    onJump(anchor.id);
                    if (Platform.OS === "web") railRef.current?.focus();
                  }}
                  style={({ pressed }) => buttonFeedback(styles.anchorItem, pressed)}
                >
                  <Text style={styles.anchorIndex}>Q{anchor.index}</Text>
                  <Text numberOfLines={2} style={styles.anchorTitle}>{anchor.title}</Text>
                </Pressable>
              ))}
            </ScrollView>
          </Animated.View>
        </View>
      ) : null}
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
      left: railInset,
      position: "absolute",
      right: railInset,
      zIndex: 20,
    },
    railButton: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 4,
      justifyContent: "center",
      minHeight: railSize,
      width: railSize,
      ...shadow.card,
    },
    railButtonActive: {
      borderColor: palette.brandDeep,
    },
    railTrack: {
      alignItems: "center",
      gap: 1,
      justifyContent: "center",
      maxHeight: 35,
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
    panelSlot: {
      left: 0,
      position: "absolute",
      right: railSize + 8,
      top: 0,
    },
    panel: {
      alignSelf: "flex-end",
      backgroundColor: palette.surface2,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.lg,
      borderWidth: 1,
      gap: 6,
      maxWidth: 248,
      overflow: "hidden",
      padding: 8,
      width: "100%",
      ...shadow.float,
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
      minHeight: 44,
      paddingHorizontal: 9,
    },
    closeText: {
      color: palette.ink2,
      fontSize: font.micro,
      fontWeight: weight.semibold,
    },
    anchorScroll: {
      flexShrink: 1,
      minHeight: 0,
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
      justifyContent: "center",
      minHeight: 44,
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
