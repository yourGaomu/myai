import { Pressable, StyleSheet, Text, View } from "react-native";

import { font, palette, radius, shadow, weight } from "../../constants/theme";

type Option<T extends string> = {
  key: T;
  label: string;
};

type Props<T extends string> = {
  accessibilityLabel?: string;
  disabled?: boolean;
  onChange: (key: T) => void;
  options: Option<T>[];
  value: T;
};

// 分段控件：用于两到三个互斥选项（工作区/会话资源、对话/规划、只读/询问/完全开放）。
export function Segmented<T extends string>({ accessibilityLabel, disabled, onChange, options, value }: Props<T>) {
  return (
    <View accessibilityLabel={accessibilityLabel} accessibilityRole="radiogroup" style={[styles.track, disabled && styles.disabled]}>
      {options.map((option) => {
        const active = option.key === value;
        return (
          <Pressable
            accessibilityRole="radio"
            accessibilityState={{ checked: active, disabled }}
            disabled={disabled}
            key={option.key}
            onPress={() => {
              if (!active) {
                onChange(option.key);
              }
            }}
            style={({ pressed }) => [styles.item, active && styles.itemActive, pressed && !active && styles.itemPressed]}
          >
            <Text numberOfLines={1} style={[styles.label, active && styles.labelActive]}>
              {option.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  track: {
    backgroundColor: palette.well,
    borderRadius: radius.md - 2,
    flexDirection: "row",
    gap: 3,
    padding: 3,
  },
  item: {
    alignItems: "center",
    borderRadius: radius.sm - 1,
    flex: 1,
    justifyContent: "center",
    minHeight: 38,
    paddingHorizontal: 8,
  },
  itemActive: {
    backgroundColor: palette.surface4,
    ...shadow.card,
  },
  disabled: {
    opacity: 0.5,
  },
  itemPressed: {
    backgroundColor: palette.pressed,
  },
  label: {
    color: palette.ink2,
    fontSize: font.label + 0.5,
    fontWeight: weight.regular,
  },
  labelActive: {
    color: palette.ink1,
    fontWeight: weight.semibold,
  },
});
