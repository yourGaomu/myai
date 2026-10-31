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
    backgroundColor: palette.glassTint,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 5,
    justifyContent: "center",
    maxHeight: 128,
    overflow: "hidden",
    paddingHorizontal: 5,
    paddingVertical: 7,
    width: 28,
  },
  railTrack: {
    alignItems: "center",
    gap: 3,
    justifyContent: "center",
    maxHeight: 88,
    overflow: "hidden",
  },
  railMarker: {
    backgroundColor: palette.surface2,
    borderRadius: radius.xxs,
    height: 3,
    width: 12,
  },
  railText: {
    color: palette.ink1,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  panel: {
    backgroundColor: palette.glassTintStrong,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 8,
    maxHeight: 256,
    overflow: "hidden",
    padding: 8,
    position: "absolute",
    right: 34,
    ...shadow.card,
    top: 0,
    width: 236,
    zIndex: 30,
  },
  panelHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  panelTitle: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  closeButton: {
    backgroundColor: palette.brandWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    paddingHorizontal: 8,
    paddingVertical: 5,
  },
  closeText: {
    color: palette.ink1,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  anchorScroll: {
    flexGrow: 0,
    maxHeight: 196,
  },
  anchorList: {
    gap: 6,
    paddingBottom: 4,
  },
  anchorItem: {
    alignItems: "flex-start",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 8,
    paddingVertical: 7,
  },
  anchorIndex: {
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.xs,
    borderWidth: 1,
    color: palette.inkInverse,
    fontSize: font.micro,
    fontWeight: weight.bold,
    overflow: "hidden",
    paddingHorizontal: 5,
    paddingVertical: 3,
  },
  anchorTitle: {
    color: palette.ink1,
    flex: 1,
    fontSize: font.label,
    fontWeight: weight.bold,
    lineHeight: 16,
  },
});
