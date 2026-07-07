import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import type { ButtonFeedback } from "../../types/ui";

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

const maxRailMarkers = 24;

export function ChatJumpNav({ anchors, buttonFeedback, onJump, onToggle, open }: Props) {
  if (anchors.length < 2) {
    return null;
  }

  const markers = compactMarkers(anchors);

  return (
    <View pointerEvents="box-none" style={styles.container}>
      {open ? (
        <View style={styles.panel}>
          <View style={styles.panelHeader}>
            <Text style={styles.panelTitle}>Jump</Text>
            <Pressable onPress={onToggle} style={({ pressed }) => buttonFeedback(styles.closeButton, pressed)}>
              <Text style={styles.closeText}>Hide</Text>
            </Pressable>
          </View>
          <ScrollView contentContainerStyle={styles.anchorList} nestedScrollEnabled showsVerticalScrollIndicator={false}>
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
    position: "absolute",
    right: 8,
    top: 52,
    zIndex: 20,
  },
  railButton: {
    alignItems: "center",
    backgroundColor: "rgba(255, 250, 240, 0.88)",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    gap: 6,
    justifyContent: "center",
    minHeight: 126,
    paddingHorizontal: 5,
    paddingVertical: 8,
    width: 28,
  },
  railTrack: {
    alignItems: "center",
    flex: 1,
    gap: 3,
    justifyContent: "center",
    minHeight: 76,
  },
  railMarker: {
    backgroundColor: "#6c665f",
    borderRadius: 3,
    height: 3,
    width: 12,
  },
  railText: {
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
  },
  panel: {
    backgroundColor: "rgba(255, 250, 240, 0.96)",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    elevation: 4,
    gap: 8,
    maxHeight: 260,
    padding: 8,
    position: "absolute",
    right: 34,
    shadowColor: "#12100e",
    shadowOffset: { width: 4, height: 4 },
    shadowOpacity: 0.16,
    shadowRadius: 0,
    top: 0,
    width: 236,
  },
  panelHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  panelTitle: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  closeButton: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 2,
    paddingHorizontal: 8,
    paddingVertical: 5,
  },
  closeText: {
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
  },
  anchorList: {
    gap: 6,
    paddingBottom: 2,
  },
  anchorItem: {
    alignItems: "flex-start",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 7,
    borderWidth: 2,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 8,
    paddingVertical: 7,
  },
  anchorIndex: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 5,
    borderWidth: 1,
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
    overflow: "hidden",
    paddingHorizontal: 5,
    paddingVertical: 3,
  },
  anchorTitle: {
    color: "#12100e",
    flex: 1,
    fontSize: 12,
    fontWeight: "800",
    lineHeight: 16,
  },
});
