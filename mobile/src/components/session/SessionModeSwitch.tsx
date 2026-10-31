import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import type { SessionAgentMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

type Props = {
  buttonFeedback: ButtonFeedback;
  disabled: boolean;
  mode: SessionAgentMode;
  onChange: (mode: SessionAgentMode) => void;
  pending: boolean;
};

const modes: Array<{ label: string; mode: SessionAgentMode }> = [
  { label: "对话模式", mode: "chat" },
  { label: "规划模式", mode: "plan" },
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
                <ActivityIndicator color={palette.ink1} size="small" />
              ) : (
                <Text
                  style={[
                    styles.buttonText,
                    selected && styles.buttonTextActive,
                  ]}
                >
                  {item.label}
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
    backgroundColor: palette.surface2,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.pill,
    borderWidth: 1,
    flexDirection: "row",
    gap: 2,
    padding: 3,
  },
  button: {
    alignItems: "center",
    borderRadius: radius.pill,
    height: 32,
    justifyContent: "center",
    paddingHorizontal: 16,
  },
  buttonActive: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderWidth: 1,
    ...shadow.card,
  },
  buttonText: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  buttonTextActive: {
    color: palette.ink1,
  },
});
