import { useEffect, useRef, useState, type ReactNode } from "react";
import { Animated, Easing, Modal, Platform, Pressable, ScrollView, StyleSheet, useWindowDimensions, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { radius } from "../../constants/theme";
import { useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";

type Props = {
  accessibilityLabel: string;
  children: ReactNode;
  onDismiss: () => void;
  visible: boolean;
};

const hiddenOffset = 640;

// 底部弹层：遮罩淡入，面板从底部滑入；自适应深浅主题。
export function BottomSheet({ accessibilityLabel, children, onDismiss, visible }: Props) {
  const styles = useThemedStyles(makeStyles);
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

const makeStyles = ({ palette, shadow }: AppTheme) =>
  StyleSheet.create({
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
      borderColor: palette.hairline,
      borderTopLeftRadius: radius.sheet,
      borderTopRightRadius: radius.sheet,
      borderWidth: 1,
      borderBottomWidth: 0,
      paddingTop: 10,
      ...shadow.float,
    },
    grabber: {
      alignSelf: "center",
      backgroundColor: palette.hairlineStrong,
      borderRadius: radius.pill,
      height: 4,
      marginBottom: 10,
      width: 38,
    },
    scroll: {
      paddingHorizontal: 18,
    },
  });
