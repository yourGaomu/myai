import { ActivityIndicator, StyleSheet, Text, View } from "react-native";

export function ButtonContent({ loading, text }: { loading?: boolean; text: string }) {
  return (
    <View style={styles.buttonContent}>
      {loading ? <ActivityIndicator color="#12100e" size="small" /> : null}
      <Text style={styles.buttonContentText}>{text}</Text>
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
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
});
