import { useEffect, useRef, useState, type ReactNode } from "react";
import { Animated, Easing, Modal, Platform, Pressable, ScrollView, StyleSheet, useWindowDimensions, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { palette, radius, shadow } from "../../constants/theme";

type Props = {
  accessibilityLabel: string;
  children: ReactNode;
  // 点遮罩或按安卓返回键时调用。权限确认用它把弹层收成提示条，而不是直接取消请求。
  onDismiss: () => void;
  visible: boolean;
};

const hiddenOffset = 640;

// 底部弹层：遮罩淡入，面板从底部滑入；关闭时先播放退出动画再卸载。
export function BottomSheet({ accessibilityLabel, children, onDismiss, visible }: Props) {
  const insets = useSafeAreaInsets();
  const { height } = useWindowDimensions();
  const [mounted, setMounted] = useState(visible);
  const fade = useRef(new Animated.Value(0)).current;
  const offset = useRef(new Animated.Value(hiddenOffset)).current;

  useEffect(() => {
    if (visible) {
      setMounted(true);
      fade.setValue(0);
      offset.setValue(hiddenOffset);
      Animated.parallel([
        Animated.timing(fade, { duration: 200, easing: Easing.out(Easing.quad), toValue: 1, useNativeDriver: true }),
        Animated.spring(offset, { bounciness: 0, speed: 16, toValue: 0, useNativeDriver: true }),
      ]).start();
      return;
    }
    if (!mounted) {
      return;
    }
    Animated.parallel([
      Animated.timing(fade, { duration: 160, toValue: 0, useNativeDriver: true }),
      Animated.timing(offset, { duration: 180, easing: Easing.in(Easing.quad), toValue: hiddenOffset, useNativeDriver: true }),
    ]).start(({ finished }) => {
      if (finished) {
        setMounted(false);
      }
    });
    // mounted 只用于判断是否需要播放退出动画，不作为依赖重新触发
  }, [fade, offset, visible]);

  if (!mounted) {
    return null;
  }

  const bottomPad = Math.max(insets.bottom, Platform.OS === "android" ? 16 : 12) + 10;

  return (
    <Modal
      animationType="none"
      navigationBarTranslucent
      onRequestClose={onDismiss}
      statusBarTranslucent
      transparent
      visible
    >
      <View style={styles.root}>
        <Animated.View style={[styles.scrim, { opacity: fade }]}>
          <Pressable accessibilityLabel="收起" onPress={onDismiss} style={StyleSheet.absoluteFill} />
        </Animated.View>
        <Animated.View
          accessibilityLabel={accessibilityLabel}
          accessibilityViewIsModal
          style={[styles.sheet, { maxHeight: height * 0.86, transform: [{ translateY: offset }] }]}
        >
          <View style={styles.grabber} />
          <ScrollView
            bounces={false}
            contentContainerStyle={{ paddingBottom: bottomPad }}
            keyboardShouldPersistTaps="handled"
            showsVerticalScrollIndicator={false}
            style={styles.scroll}
          >
            {children}
          </ScrollView>
        </Animated.View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
    justifyContent: "flex-end",
  },
  scrim: {
    ...StyleSheet.absoluteFill,
    backgroundColor: palette.scrim,
  },
  sheet: {
    backgroundColor: palette.surface2,
    borderTopLeftRadius: radius.sheet,
    borderTopRightRadius: radius.sheet,
    paddingHorizontal: 16,
    paddingTop: 8,
    ...shadow.float,
  },
  grabber: {
    alignSelf: "center",
    backgroundColor: palette.hairlineStrong,
    borderRadius: radius.xxs,
    height: 4,
    marginBottom: 14,
    width: 36,
  },
  scroll: {
    flexGrow: 0,
  },
});
