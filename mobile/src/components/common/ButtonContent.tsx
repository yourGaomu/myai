import { ActivityIndicator, StyleSheet, Text, View } from "react-native";
import { font, weight } from "../../constants/theme";
import { useAppTheme } from "../../theme/ThemeProvider";

export function ButtonContent({ color, loading, text }: { color?: string; loading?: boolean; text: string }) {
  const { palette } = useAppTheme();
  const resolved = color ?? palette.inkInverse;
  return (
    <View style={styles.buttonContent}>
      {loading ? <ActivityIndicator color={resolved} size="small" /> : null}
      <Text style={[styles.buttonContentText, { color: resolved }]}>{text}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  buttonContent: {
    alignItems: "center",
    flexDirection: "row",
    gap: 7,
    justifyContent: "center",
  },
  buttonContentText: {
    fontSize: font.label,
    fontWeight: weight.semibold,
  },
});
