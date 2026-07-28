import { ActivityIndicator, StyleSheet, Text, View } from "react-native";

export function ButtonContent({ color = "#12100e", loading, text }: { color?: string; loading?: boolean; text: string }) {
  return (
    <View style={styles.buttonContent}>
      {loading ? <ActivityIndicator color={color} size="small" /> : null}
      <Text style={[styles.buttonContentText, { color }]}>{text}</Text>
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
    fontSize: 12,
    fontWeight: "900",
  },
});
