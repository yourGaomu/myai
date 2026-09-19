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

const modes: Array<{ icon: string; label: string; mode: SessionAgentMode }> = [
  { icon: "💬", label: "对话模式", mode: "chat" },
  { icon: "🎯", label: "规划模式", mode: "plan" },
];

export function SessionModeSwitch({ buttonFeedback, disabled, mode, onChange, pending }: Props) {
  return (
    <View style={styles.wrapper}>
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
              style={({ pressed }) =>
                buttonFeedback(
                  [styles.button, selected && styles.buttonActive],
                  pressed,
                )
              }
            >
              {pending && selected ? (
                <ActivityIndicator color="#12100e" size="small" />
              ) : (
                <Text
                  style={[
                    styles.buttonText,
                    selected && styles.buttonTextActive,
                  ]}
                >
                  {`${item.icon} ${item.label}`}
                </Text>
              )}
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  wrapper: {
    alignItems: "center",
    justifyContent: "center",
    paddingVertical: 2,
  },
  control: {
    backgroundColor: "#ded7cc",
    borderColor: "#25231f",
    borderRadius: 999,
    borderWidth: 2,
    flexDirection: "row",
    gap: 2,
    padding: 3,
  },
  button: {
    alignItems: "center",
    borderRadius: 999,
    height: 32,
    justifyContent: "center",
    paddingHorizontal: 16,
  },
  buttonActive: {
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderWidth: 1.5,
    elevation: 2,
    shadowColor: "#12100e",
    shadowOffset: { height: 1, width: 0 },
    shadowOpacity: 0.16,
    shadowRadius: 2,
  },
  buttonText: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "900",
  },
  buttonTextActive: {
    color: "#12100e",
  },
});
