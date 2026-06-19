import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";

type Props = {
  buttonFeedback: ButtonFeedback;
  connected: boolean;
  deviceID: string;
  isBusy: boolean;
  onToggleSettings: () => void;
  status: string;
  userID: string;
  viewMode: ViewMode;
};

export function AppHeader({
  buttonFeedback,
  connected,
  deviceID,
  isBusy,
  onToggleSettings,
  status,
  userID,
  viewMode,
}: Props) {
  return (
    <View style={styles.header}>
      <View style={styles.brand}>
        <View style={styles.brandMark}>
          <Text style={styles.brandMarkText}>M</Text>
        </View>
        <View style={styles.headerText}>
          <Text style={styles.title}>MYAI</Text>
          <Text style={styles.subtitle}>
            {connected ? `手机控制电脑 / ${userID.trim() || "local"} / ${deviceID.trim() || "pc-local"}` : status}
          </Text>
        </View>
      </View>
      <View style={styles.headerActions}>
        <Pressable
          onPress={onToggleSettings}
          style={({ pressed }) => buttonFeedback([styles.ghostButton, viewMode === "settings" && styles.ghostButtonActive], pressed)}
        >
          <Text style={styles.ghostButtonText}>⚙</Text>
        </Pressable>
        <View style={[styles.statusPill, connected ? styles.statusPillOnline : styles.statusPillOffline]}>
          {isBusy ? (
            <ActivityIndicator color="#12100e" size="small" />
          ) : (
            <View style={styles.statusDot} />
          )}
          <Text style={styles.statusPillText}>{isBusy ? "处理中" : connected ? "在线" : "离线"}</Text>
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  header: {
    display: "none",
  },
  brand: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 10,
    minWidth: 0,
  },
  brandMark: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    height: 46,
    justifyContent: "center",
    width: 46,
  },
  brandMarkText: {
    color: "#12100e",
    fontSize: 20,
    fontWeight: "900",
  },
  headerText: {
    flex: 1,
    minWidth: 0,
  },
  headerActions: {
    alignItems: "center",
    flexDirection: "row",
    flexShrink: 0,
    gap: 8,
  },
  title: {
    color: "#12100e",
    fontSize: 22,
    fontWeight: "900",
    lineHeight: 24,
  },
  subtitle: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "700",
    marginTop: 2,
  },
  ghostButton: {
    alignItems: "center",
    backgroundColor: "#4fd7ee",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    height: 44,
    justifyContent: "center",
    minWidth: 44,
    paddingHorizontal: 10,
  },
  ghostButtonActive: {
    backgroundColor: "#ffd84f",
  },
  ghostButtonText: {
    color: "#12100e",
    fontSize: 18,
    fontWeight: "900",
  },
  statusPill: {
    alignItems: "center",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 3,
    flexDirection: "row",
    gap: 6,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  statusPillOnline: {
    backgroundColor: "#b9e9b0",
  },
  statusPillOffline: {
    backgroundColor: "#ff7f68",
  },
  statusDot: {
    backgroundColor: "#12100e",
    borderRadius: 4,
    height: 8,
    width: 8,
  },
  statusPillText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
});
