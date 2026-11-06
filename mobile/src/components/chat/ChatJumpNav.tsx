import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import type { ButtonFeedback } from "../../types/ui";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

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

export function ChatJumpNav({ anchors, buttonFeedback, onJump, onToggle, open }: Props) {
  if (anchors.length < 2) {
    return null;
  }

  const markers = compactMarkers(anchors);

  return (
    <View pointerEvents="box-none" style={styles.container}>
      {open ? (
        <View
          onStartShouldSetResponder={() => true}
          style={styles.panel}
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
        </View>
      ) : null}

      <Pressable onPress={onToggle} style={({ pressed }) => buttonFeedback(styles.railButton, pressed)}>
        <View style={styles.railTrack}>
          {markers.map((anchor) => (
            <View key={anchor.id} style={styles.railMarker} />
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

const styles = StyleSheet.create({
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
    borderRadius: radius.pill,
    gap: 5,
    justifyContent: "center",
    maxHeight: 128,
    overflow: "hidden",
    paddingHorizontal: 5,
    paddingVertical: 9,
    width: 26,
    ...shadow.card,
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
  railText: {
    color: palette.ink2,
    fontSize: font.micro,
    fontWeight: weight.semibold,
  },
  panel: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
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
    borderRadius: radius.sm,
    paddingHorizontal: 8,
    paddingVertical: 6,
  },
  closeText: {
    color: palette.brandLight,
    fontSize: font.caption,
  },
  anchorScroll: {
    flexGrow: 0,
    maxHeight: 196,
  },
  anchorList: {
    gap: 2,
    paddingBottom: 2,
  },
  anchorItem: {
    alignItems: "flex-start",
    borderRadius: radius.sm,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 8,
    paddingVertical: 8,
  },
  anchorIndex: {
    color: palette.ink3,
    fontSize: font.caption,
    fontWeight: weight.semibold,
    minWidth: 24,
  },
  anchorTitle: {
    color: palette.ink1,
    flex: 1,
    fontSize: font.label,
    lineHeight: 19,
  },
});
