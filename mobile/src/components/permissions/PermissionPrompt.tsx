import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import type { PermissionState } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { palette, radius, weight } from "../../constants/theme";

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
    backgroundColor: palette.surface2,
    borderColor: palette.brandDeep,
    borderRadius: radius.md,
    borderWidth: 1,
    gap: 10,
    padding: 12,
  },
  permissionTitle: {
    color: palette.ink1,
    fontWeight: weight.bold,
  },
  argsScroll: {
    backgroundColor: palette.recessed,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    maxHeight: 160,
    paddingHorizontal: 8,
    paddingVertical: 6,
  },
  permissionArgs: {
    color: palette.ink2,
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
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 44,
    paddingHorizontal: 16,
  },
  primaryButtonText: {
    color: palette.inkInverse,
    fontWeight: weight.bold,
  },
  secondaryButton: {
    alignItems: "center",
    backgroundColor: palette.surface3,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 44,
    paddingHorizontal: 16,
  },
  secondaryButtonText: {
    color: palette.ink1,
    fontWeight: weight.bold,
  },
});
