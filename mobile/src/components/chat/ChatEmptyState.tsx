import { useEffect, useRef, type ComponentProps } from "react";
import { Animated, Easing, Platform, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";
import * as Haptics from "expo-haptics";
import { LinearGradient } from "expo-linear-gradient";

import { font, radius, weight } from "../../constants/theme";
import { useWallpaper } from "../../hooks/useWallpaperConfig";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { CharacterHalo, PulseDot, useReduceMotion } from "../common/MotionEffects";

type Props = {
  changesCount?: number;
  connected: boolean;
  deviceID: string;
  name: string;
  onOpenChanges?: () => void;
  onPickSuggestion: (text: string) => void;
};

type FeatherName = ComponentProps<typeof Feather>["name"];

// 建议点击后填入输入框，发送前保留二次编辑机会（部分指令会在电脑真实执行）。
const suggestions: { icon: FeatherName; subtitle: string; text: string; title: string }[] = [
  {
    icon: "git-pull-request",
    title: "总结当前代码变更",
    subtitle: "读取工作区改动并生成评审摘要",
    text: "总结一下当前未提交的代码改动，并指出潜在风险",
  },
  {
    icon: "check-circle",
    title: "运行测试并定位报错",
    subtitle: "执行单元验证，如有失败直接给出修复建议",
    text: "跑一遍项目测试，定位并修复失败的用例",
  },
  {
    icon: "folder",
    title: "梳理目录与模块架构",
    subtitle: "快速了解核心模块划分、数据流与入口文件",
    text: "讲讲这个项目的目录结构、核心模块和交互链路",
  },
];

function greeting() {
  const hour = new Date().getHours();
  if (hour < 5) return "夜深了";
  if (hour < 11) return "早上好";
  if (hour < 13) return "中午好";
  if (hour < 18) return "下午好";
  return "晚上好";
}

function triggerHaptic() {
  if (Platform.OS !== "web") {
    void Haptics.selectionAsync().catch(() => {});
  }
}

export function ChatEmptyState({
  changesCount = 0,
  connected,
  deviceID,
  name,
  onOpenChanges,
  onPickSuggestion,
}: Props) {
  const { isDark, palette } = useAppTheme();
  const { isWallpaperActive } = useWallpaper();
  const styles = useThemedStyles(makeStyles);
  const reduceMotion = useReduceMotion();
  const device = deviceID.trim() || "电脑";

  const anims = useRef(
    Array.from({ length: 5 }, () => ({
      opacity: new Animated.Value(1),
      translateY: new Animated.Value(0),
    })),
  ).current;

  useEffect(() => {
    if (reduceMotion) {
      anims.forEach((item) => {
        item.opacity.setValue(1);
        item.translateY.setValue(0);
      });
      return;
    }

    anims.forEach((item) => {
      item.opacity.setValue(0);
      item.translateY.setValue(14);
    });

    const composite = Animated.stagger(
      65,
      anims.map((item) =>
        Animated.parallel([
          Animated.timing(item.opacity, {
            duration: 320,
            easing: Easing.out(Easing.cubic),
            toValue: 1,
            useNativeDriver: true,
          }),
          Animated.timing(item.translateY, {
            duration: 340,
            easing: Easing.out(Easing.cubic),
            toValue: 0,
            useNativeDriver: true,
          }),
        ]),
      ),
    );
    composite.start();
    return () => composite.stop();
  }, [anims, reduceMotion]);

  const stageStyle = (idx: number) => ({
    opacity: anims[idx].opacity,
    transform: [{ translateY: anims[idx].translateY }],
  });

  return (
    <View style={styles.wrap}>
      <Animated.View style={[styles.heroWrap, stageStyle(0)]}>
        <CharacterHalo ringSize={140}>
          <CharacterAvatar accessibilityLabel="MyAI 角色" height={158} size={158} slot="idle" variant="full" />
        </CharacterHalo>
      </Animated.View>

      <Animated.View style={[styles.headerBlock, stageStyle(1)]}>
        <Text style={[styles.title, isWallpaperActive && (isDark ? styles.textShadowDark : styles.textShadowLight)]}>
          {name ? `${greeting()}，${name}` : greeting()}
        </Text>
        <Text style={[styles.sub, isWallpaperActive && (isDark ? styles.textShadowDark : styles.textShadowLight)]}>
          {connected ? `${device} 已就绪，说说接下来要做什么` : `${device} 暂未连线，可先浏览本地视图`}
        </Text>

        {changesCount > 0 && onOpenChanges ? (
          <Pressable
            accessibilityRole="button"
            onPress={() => {
              triggerHaptic();
              onOpenChanges();
            }}
            style={({ pressed }) => [styles.changesPill, isWallpaperActive && styles.pillGlass, pressed && styles.pressed]}
          >
            <PulseDot color={palette.brand} size={7} />
            <Text style={styles.changesText}>工作区有 {changesCount} 项未提交变更 · 点击审查</Text>
            <Feather color={palette.brandLight} name="chevron-right" size={14} />
          </Pressable>
        ) : null}
      </Animated.View>

      <View style={styles.list}>
        {suggestions.map((item, idx) => (
          <Animated.View key={item.title} style={stageStyle(Math.min(4, idx + 2))}>
            <Pressable
              accessibilityRole="button"
              onPress={() => {
                triggerHaptic();
                onPickSuggestion(item.text);
              }}
              style={({ pressed }) => [styles.item, isWallpaperActive && styles.itemGlass, pressed && styles.itemPressed]}
            >
              <View pointerEvents="none" style={styles.leftBar}>
                <LinearGradient
                  colors={[palette.brandLight, palette.brandDark]}
                  end={{ x: 0, y: 1 }}
                  start={{ x: 0, y: 0 }}
                  style={StyleSheet.absoluteFill}
                />
              </View>
              <View style={styles.iconBox}>
                <Feather color={palette.brandLight} name={item.icon} size={18} />
              </View>
              <View style={styles.itemBody}>
                <Text numberOfLines={1} style={styles.itemTitle}>{item.title}</Text>
                <Text numberOfLines={1} style={styles.itemSub}>{item.subtitle}</Text>
              </View>
              <Feather color={palette.ink4} name="arrow-up-left" size={16} />
            </Pressable>
          </Animated.View>
        ))}
      </View>
    </View>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    wrap: {
      alignItems: "center",
      flex: 1,
      justifyContent: "center",
      paddingBottom: 16,
      paddingTop: 8,
    },
    heroWrap: {
      alignItems: "center",
      justifyContent: "center",
      marginBottom: 6,
    },
    headerBlock: {
      alignItems: "center",
    },
    title: {
      color: palette.ink1,
      fontSize: font.display,
      fontWeight: weight.semibold,
      marginTop: 2,
      textAlign: "center",
    },
    sub: {
      color: palette.ink2,
      fontSize: font.label,
      marginTop: 6,
      textAlign: "center",
    },
    changesPill: {
      alignItems: "center",
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 7,
      marginTop: 12,
      paddingHorizontal: 13,
      paddingVertical: 6,
    },
    changesText: {
      color: palette.brandLight,
      fontSize: font.caption,
      fontWeight: weight.semibold,
    },
    list: {
      alignSelf: "stretch",
      gap: 9,
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
      minHeight: 58,
      overflow: "hidden",
      paddingHorizontal: 13,
      paddingVertical: 11,
      position: "relative",
    },
    leftBar: {
      borderBottomRightRadius: 3,
      borderTopRightRadius: 3,
      bottom: 9,
      left: 0,
      overflow: "hidden",
      position: "absolute",
      top: 9,
      width: 3,
    },
    iconBox: {
      alignItems: "center",
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
      borderRadius: radius.md,
      borderWidth: 1,
      height: 36,
      justifyContent: "center",
      width: 36,
    },
    itemBody: {
      flex: 1,
      gap: 2,
      minWidth: 0,
    },
    itemTitle: {
      color: palette.ink1,
      fontSize: font.body - 0.5,
      fontWeight: weight.semibold,
    },
    itemSub: {
      color: palette.ink3,
      fontSize: font.caption - 0.5,
    },
    itemPressed: {
      backgroundColor: palette.pressed,
      transform: [{ scale: 0.98 }],
    },
    itemGlass: {
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
    },
    pillGlass: {
      backgroundColor: palette.floatingGlass,
    },
    textShadowDark: {
      textShadowColor: "rgba(0,0,0,0.45)",
      textShadowOffset: { width: 0, height: 1 },
      textShadowRadius: 6,
    },
    textShadowLight: {
      textShadowColor: "rgba(255,255,255,0.7)",
      textShadowOffset: { width: 0, height: 1 },
      textShadowRadius: 6,
    },
    pressed: {
      opacity: 0.78,
      transform: [{ scale: 0.97 }],
    },
  });
