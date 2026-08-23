import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import type { SessionAgentMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";

type Props = {
  buttonFeedback: ButtonFeedback;
  disabled: boolean;
  mode: SessionAgentMode;
  onChange: (mode: SessionAgentMode) => void;
  pending: boolean;
};

const modes: Array<{ label: string; mode: SessionAgentMode }> = [
  { label: "对话", mode: "chat" },
  { label: "计划", mode: "plan" },
];

export function SessionModeSwitch({ buttonFeedback, disabled, mode, onChange, pending }: Props) {
  return (
    <View style={styles.row}>
      <View style={styles.copy}>
        <Text style={styles.title}>会话模式</Text>
        <Text numberOfLines={1} style={styles.description}>
          {pending ? "正在切换模式..." : mode === "plan" ? "先生成计划，再确认执行" : "直接对话并执行任务"}
        </Text>
      </View>
      <View accessibilityRole="tablist" style={styles.control}>
        {modes.map((item) => {
          const selected = mode === item.mode;
          return (
            <Pressable
              accessibilityRole="tab"
              accessibilityState={{ disabled: disabled || pending, selected }}
              disabled={disabled || pending || selected}
              key={item.mode}
              onPress={() => onChange(item.mode)}
              style={({ pressed }) => buttonFeedback([styles.button, selected && styles.buttonActive], pressed)}
            >
              {pending && selected ? <ActivityIndicator color="#171613" size="small" /> : <Text style={styles.buttonText}>{item.label}</Text>}
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  row: {
    alignItems: "center",
    flexDirection: "row",
    gap: 12,
    justifyContent: "space-between",
  },
  copy: {
    flex: 1,
    minWidth: 0,
  },
  title: {
    color: "#171613",
    fontSize: 15,
    fontWeight: "900",
  },
  description: {
    color: "#777269",
    fontSize: 11,
    fontWeight: "700",
    marginTop: 3,
  },
  control: {
    backgroundColor: "#ded7cc",
    borderColor: "#25231f",
    borderRadius: 999,
    borderWidth: 2,
    flexDirection: "row",
    padding: 3,
  },
  button: {
    alignItems: "center",
    borderRadius: 999,
    height: 34,
    justifyContent: "center",
    minWidth: 64,
    paddingHorizontal: 12,
  },
  buttonActive: {
    backgroundColor: "#fffdf7",
    elevation: 2,
    shadowColor: "#171613",
    shadowOffset: { height: 1, width: 0 },
    shadowOpacity: 0.14,
    shadowRadius: 3,
  },
  buttonText: {
    color: "#171613",
    fontSize: 12,
    fontWeight: "900",
  },
});
