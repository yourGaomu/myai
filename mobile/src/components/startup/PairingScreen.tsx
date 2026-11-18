import { useEffect, useRef, useState } from "react";
import {
  ActivityIndicator,
  Animated,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { Feather } from "@expo/vector-icons";
import * as Haptics from "expo-haptics";
import { LinearGradient } from "expo-linear-gradient";

import { fontFamily, font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { CharacterHalo, PulseDot, useReduceMotion } from "../common/MotionEffects";

type Props = {
  bindCode: string;
  bottomInset: number;
  failed: boolean;
  normalizedRelayURL: string;
  onBindCodeChange: (code: string) => void;
  onPair: () => void;
  onRelayURLChange: (url: string) => void;
  pending: boolean;
  relayURL: string;
  topInset: number;
};

const cells = [0, 1, 2, 3, 4, 5];

// 首次启动 / 未配对时的全屏引导：绑定动态角色、适应深浅主题，带输入弹跳与失败抖动反馈。
export function PairingScreen(props: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const reduceMotion = useReduceMotion();
  const { bindCode, failed, pending } = props;
  const inputRef = useRef<TextInput>(null);
  const [focused, setFocused] = useState(false);
  const [custom, setCustom] = useState(false);
  const [editingRelay, setEditingRelay] = useState(false);
  const code = bindCode.trim();
  const canPair = !pending && (custom ? code.length > 0 : code.length === 6);
  const cursor = Math.min(code.length, 5);

  const cellScales = useRef(cells.map(() => new Animated.Value(1))).current;
  const prevLenRef = useRef(code.length);
  const shakeX = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    const nextLen = code.length;
    const prevLen = prevLenRef.current;
    prevLenRef.current = nextLen;
    if (nextLen > prevLen && nextLen <= 6 && !custom) {
      const idx = nextLen - 1;
      if (Platform.OS !== "web") {
        void Haptics.selectionAsync().catch(() => {});
      }
      if (!reduceMotion && cellScales[idx]) {
        cellScales[idx].setValue(0.84);
        Animated.spring(cellScales[idx], {
          bounciness: 12,
          speed: 24,
          toValue: 1,
          useNativeDriver: true,
        }).start();
      }
    }
  }, [cellScales, code.length, custom, reduceMotion]);

  useEffect(() => {
    if (!failed) return;
    if (Platform.OS !== "web") {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error).catch(() => {});
    }
    if (reduceMotion) return;
    shakeX.setValue(0);
    Animated.sequence([
      Animated.timing(shakeX, { duration: 55, toValue: -8, useNativeDriver: true }),
      Animated.timing(shakeX, { duration: 55, toValue: 8, useNativeDriver: true }),
      Animated.timing(shakeX, { duration: 50, toValue: -6, useNativeDriver: true }),
      Animated.timing(shakeX, { duration: 50, toValue: 6, useNativeDriver: true }),
      Animated.timing(shakeX, { duration: 45, toValue: 0, useNativeDriver: true }),
    ]).start();
  }, [failed, reduceMotion, shakeX]);

  return (
    <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={styles.root}>
      <View pointerEvents="none" style={styles.auroraOrb} />
      <ScrollView
        contentContainerStyle={[styles.content, { paddingBottom: props.bottomInset + 16, paddingTop: props.topInset + 24 }]}
        keyboardShouldPersistTaps="handled"
      >
        <CharacterHalo ringSize={146}>
          <CharacterAvatar accessibilityLabel="MyAI 角色" height={168} size={168} slot="idle" variant="full" />
        </CharacterHalo>
        <Text style={styles.title}>连接你的电脑</Text>
        <Text style={styles.sub}>在电脑上启动 MyAI Agent，{"\n"}输入终端里显示的配对码。</Text>

        {custom ? (
          <TextInput
            autoCapitalize="none"
            autoCorrect={false}
            onChangeText={props.onBindCodeChange}
            placeholder="配对码"
            placeholderTextColor={palette.ink4}
            style={styles.customInput}
            value={bindCode}
          />
        ) : (
          <Animated.View style={[styles.otpWrap, { transform: [{ translateX: shakeX }] }]}>
            <Pressable accessibilityLabel="6 位配对码" onPress={() => inputRef.current?.focus()} style={styles.otp}>
              {cells.map((index) => {
                const char = code[index] || "";
                const isCursor = focused && index === cursor && code.length < 6;
                return (
                  <Animated.View
                    key={index}
                    style={[
                      styles.cell,
                      Boolean(char) && styles.cellFilled,
                      isCursor && styles.cellActive,
                      { transform: [{ scale: cellScales[index] }] },
                    ]}
                  >
                    <Text style={[styles.cellText, Boolean(char) && styles.cellTextFilled]}>{char}</Text>
                  </Animated.View>
                );
              })}
              <TextInput
                autoComplete="one-time-code"
                caretHidden
                keyboardType="number-pad"
                maxLength={6}
                onBlur={() => setFocused(false)}
                onChangeText={(value) => props.onBindCodeChange(value.replace(/\D/g, "").slice(0, 6))}
                onFocus={() => setFocused(true)}
                ref={inputRef}
                style={styles.hiddenInput}
                textContentType="oneTimeCode"
                value={code}
              />
            </Pressable>
          </Animated.View>
        )}
        <Pressable hitSlop={8} onPress={() => setCustom((value) => !value)}>
          <Text style={styles.switchLink}>{custom ? "使用 6 位数字配对码" : "配对码不是 6 位数字？"}</Text>
        </Pressable>

        <Pressable
          accessibilityRole="button"
          disabled={!canPair}
          onPress={props.onPair}
          style={({ pressed }) => [styles.button, !canPair && !pending && styles.buttonDisabled, pressed && styles.pressed]}
        >
          {canPair || pending ? (
            <LinearGradient
              colors={[palette.brandLight, palette.brand]}
              end={{ x: 1, y: 0 }}
              start={{ x: 0, y: 0 }}
              style={StyleSheet.absoluteFill}
            />
          ) : null}
          {pending ? <ActivityIndicator color={palette.inkInverse} size="small" /> : null}
          <Text style={[styles.buttonText, !canPair && !pending && styles.buttonTextDisabled]}>
            {pending ? "正在配对…" : "配对并连接"}
          </Text>
        </Pressable>
        {props.failed ? <Text style={styles.error}>配对失败。请检查配对码，并确认电脑上的 Agent 正在运行。</Text> : null}

        <View style={styles.spacer} />
        {editingRelay ? (
          <View style={styles.relayEdit}>
            <TextInput
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="url"
              onChangeText={props.onRelayURLChange}
              onSubmitEditing={() => setEditingRelay(false)}
              placeholder="https://relay.example.com"
              placeholderTextColor={palette.ink4}
              style={styles.relayInput}
              value={props.relayURL}
            />
            <Pressable hitSlop={8} onPress={() => setEditingRelay(false)}>
              <Text style={styles.link}>完成</Text>
            </Pressable>
          </View>
        ) : (
          <View style={styles.relay}>
            <PulseDot color={props.normalizedRelayURL ? palette.ok : palette.ink4} pulse={Boolean(props.normalizedRelayURL)} size={7} />
            <Feather color={palette.ink3} name="server" size={14} />
            <Text numberOfLines={1} style={styles.relayText}>中继 {props.normalizedRelayURL.replace(/^[a-z]+:\/\//i, "") || "未设置"}</Text>
            <Pressable hitSlop={8} onPress={() => setEditingRelay(true)}>
              <Text style={styles.link}>修改</Text>
            </Pressable>
          </View>
        )}
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    root: { backgroundColor: palette.canvas, flex: 1 },
    auroraOrb: {
      alignSelf: "center",
      backgroundColor: palette.ambientGlow,
      borderRadius: 150,
      height: 260,
      position: "absolute",
      top: 32,
      width: 260,
    },
    content: { alignItems: "center", flexGrow: 1, paddingHorizontal: 24 },
    title: { color: palette.ink1, fontSize: font.display + 2, fontWeight: weight.semibold, marginTop: 8 },
    sub: { color: palette.ink2, fontSize: font.body - 1, lineHeight: 22, marginTop: 8, textAlign: "center" },
    otpWrap: { marginTop: 26, maxWidth: 320, width: "100%" },
    otp: { flexDirection: "row", gap: 8, justifyContent: "center", width: "100%" },
    cell: {
      alignItems: "center",
      backgroundColor: palette.surface1,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.md,
      borderWidth: 1,
      flex: 1,
      height: 54,
      justifyContent: "center",
      maxWidth: 46,
    },
    cellFilled: {
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
    },
    cellActive: {
      borderColor: palette.brand,
      borderWidth: 2,
    },
    cellText: { color: palette.ink1, fontFamily: fontFamily.mono, fontSize: font.display, fontWeight: weight.semibold },
    cellTextFilled: { color: palette.brandLight },
    hiddenInput: { ...StyleSheet.absoluteFill, color: palette.transparent, opacity: 0.02 },
    customInput: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.md,
      borderWidth: 1,
      color: palette.ink1,
      fontFamily: fontFamily.mono,
      fontSize: font.title,
      height: 54,
      marginTop: 26,
      maxWidth: 320,
      paddingHorizontal: 16,
      textAlign: "center",
      width: "100%",
    },
    switchLink: { color: palette.ink3, fontSize: font.caption, marginTop: 14 },
    button: {
      alignItems: "center",
      backgroundColor: palette.brand,
      borderRadius: radius.md,
      flexDirection: "row",
      gap: 8,
      height: 50,
      justifyContent: "center",
      marginTop: 20,
      maxWidth: 320,
      overflow: "hidden",
      width: "100%",
    },
    buttonDisabled: { backgroundColor: palette.well },
    buttonText: { color: palette.inkInverse, fontSize: font.body, fontWeight: weight.bold },
    buttonTextDisabled: { color: palette.ink3 },
    pressed: { opacity: 0.84, transform: [{ scale: 0.98 }] },
    error: { color: palette.err, fontSize: font.label, lineHeight: 20, marginTop: 14, maxWidth: 300, textAlign: "center" },
    spacer: { flex: 1, minHeight: 28 },
    relay: {
      alignItems: "center",
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 8,
      maxWidth: 320,
      paddingHorizontal: 14,
      paddingVertical: 8,
    },
    relayText: { color: palette.ink3, flexShrink: 1, fontSize: font.caption },
    relayEdit: {
      alignItems: "center",
      backgroundColor: palette.surface1,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 10,
      height: 44,
      maxWidth: 340,
      paddingHorizontal: 12,
      width: "100%",
    },
    relayInput: { color: palette.ink1, flex: 1, fontSize: font.label, padding: 0 },
    link: { color: palette.brandLight, fontSize: font.caption, fontWeight: weight.semibold },
  });
