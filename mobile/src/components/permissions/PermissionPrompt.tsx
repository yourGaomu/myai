import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import type { PermissionState } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";

type Props = {
  buttonFeedback: ButtonFeedback;
  onAllow: () => void;
  onDeny: () => void;
  permission: PermissionState;
};

export function PermissionPrompt({ buttonFeedback, onAllow, onDeny, permission }: Props) {
  return (
    <View style={styles.permissionBox}>
      <Text style={styles.permissionTitle}>
        {permission.name} 需要权限：{permission.permission}
      </Text>
      {permission.arguments ? (
        <ScrollView nestedScrollEnabled style={styles.argsScroll}>
          <Text selectable style={styles.permissionArgs}>{permission.arguments}</Text>
        </ScrollView>
      ) : null}
      <View style={styles.row}>
        <Pressable onPress={onDeny} style={({ pressed }) => buttonFeedback([styles.secondaryButton, styles.flex], pressed)}>
          <Text style={styles.secondaryButtonText}>拒绝</Text>
        </Pressable>
        <Pressable onPress={onAllow} style={({ pressed }) => buttonFeedback([styles.primaryButton, styles.flex], pressed)}>
          <Text style={styles.primaryButtonText}>允许</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  permissionBox: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    gap: 10,
    padding: 12,
  },
  permissionTitle: {
    color: "#12100e",
    fontWeight: "900",
  },
  argsScroll: {
    backgroundColor: "#fff8db",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 1.5,
    maxHeight: 160,
    paddingHorizontal: 8,
    paddingVertical: 6,
  },
  permissionArgs: {
    color: "#12100e",
    lineHeight: 19,
  },
  row: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  primaryButton: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    justifyContent: "center",
    minHeight: 44,
    paddingHorizontal: 16,
  },
  primaryButtonText: {
    color: "#12100e",
    fontWeight: "900",
  },
  secondaryButton: {
    alignItems: "center",
    backgroundColor: "#b9e9b0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    justifyContent: "center",
    minHeight: 44,
    paddingHorizontal: 16,
  },
  secondaryButtonText: {
    color: "#12100e",
    fontWeight: "900",
  },
});
