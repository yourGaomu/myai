import type { ComponentProps } from "react";
import { Image, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, palette, radius, weight } from "../../constants/theme";

type Props = {
  connected: boolean;
  deviceID: string;
  name: string;
  onPickSuggestion: (text: string) => void;
};

type FeatherName = ComponentProps<typeof Feather>["name"];

// 建议只填进输入框、不直接发送：这些指令会在电脑上真实执行，发送前给用户改的机会。
const suggestions: { icon: FeatherName; label: string; text: string }[] = [
  { icon: "git-pull-request", label: "总结今天未提交的改动", text: "总结一下今天未提交的改动" },
  { icon: "check-circle", label: "跑一遍测试，修掉失败的用例", text: "跑一遍测试，修掉失败的用例" },
  { icon: "folder", label: "讲讲这个项目的目录结构", text: "讲讲这个项目的目录结构和主要模块" },
];

function greeting() {
  const hour = new Date().getHours();
  if (hour < 5) return "夜深了";
  if (hour < 11) return "早上好";
  if (hour < 13) return "中午好";
  if (hour < 18) return "下午好";
  return "晚上好";
}

export function ChatEmptyState({ connected, deviceID, name, onPickSuggestion }: Props) {
  const device = deviceID.trim() || "电脑";
  return (
    <View style={styles.wrap}>
      <Image accessibilityIgnoresInvertColors source={require("../../../assets/hero-character.png")} style={styles.hero} />
      <Text style={styles.title}>{name ? `${greeting()}，${name}` : greeting()}</Text>
      <Text style={styles.sub}>{connected ? `${device} 已就绪，说说要做什么` : `${device} 未连接，连上后就可以开始`}</Text>
      <View style={styles.list}>
        {suggestions.map((item) => (
          <Pressable
            accessibilityRole="button"
            key={item.label}
            onPress={() => onPickSuggestion(item.text)}
            style={({ pressed }) => [styles.item, pressed && styles.itemPressed]}
          >
            <Feather color={palette.brandLight} name={item.icon} size={19} />
            <Text style={styles.itemText}>{item.label}</Text>
            <Feather color={palette.ink4} name="arrow-up-left" size={16} />
          </Pressable>
        ))}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    alignItems: "center",
    flex: 1,
    justifyContent: "center",
    paddingBottom: 12,
    paddingTop: 8,
  },
  hero: {
    height: 156,
    width: 156,
  },
  title: {
    color: palette.ink1,
    fontSize: font.display,
    fontWeight: weight.semibold,
    marginTop: 4,
    textAlign: "center",
  },
  sub: {
    color: palette.ink2,
    fontSize: font.label,
    marginTop: 6,
    textAlign: "center",
  },
  list: {
    alignSelf: "stretch",
    gap: 8,
    marginTop: 22,
  },
  item: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.lg,
    borderWidth: 1,
    flexDirection: "row",
    gap: 12,
    minHeight: 52,
    paddingHorizontal: 14,
    paddingVertical: 12,
  },
  itemPressed: {
    backgroundColor: palette.pressed,
  },
  itemText: {
    color: palette.ink1,
    flex: 1,
    fontSize: font.body - 0.5,
    lineHeight: 20,
  },
});
