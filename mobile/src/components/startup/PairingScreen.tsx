import { useRef, useState } from "react";
import { ActivityIndicator, Image, KeyboardAvoidingView, Platform, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { fontFamily, font, palette, radius, weight } from "../../constants/theme";

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

// 首次启动 / 未配对时的全屏引导。配对成功后 client_token 写入，外层自动发起连接并进入对话页。
export function PairingScreen(props: Props) {
  const { bindCode, pending } = props;
  const inputRef = useRef<TextInput>(null);
  const [focused, setFocused] = useState(false);
  // Agent 可以用 --bind-code 指定任意配对码，所以保留自定义输入
  const [custom, setCustom] = useState(false);
  const [editingRelay, setEditingRelay] = useState(false);
  const code = bindCode.trim();
  const canPair = !pending && (custom ? code.length > 0 : code.length === 6);
  const cursor = Math.min(code.length, 5);

  return (
    <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={styles.root}>
      <ScrollView
        contentContainerStyle={[styles.content, { paddingBottom: props.bottomInset + 16, paddingTop: props.topInset + 24 }]}
        keyboardShouldPersistTaps="handled"
      >
        <Image source={require("../../../assets/hero-character.png")} style={styles.hero} />
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
          <Pressable accessibilityLabel="6 位配对码" onPress={() => inputRef.current?.focus()} style={styles.otp}>
            {cells.map((index) => (
              <View key={index} style={[styles.cell, focused && index === cursor && styles.cellActive]}>
                <Text style={styles.cellText}>{code[index] || ""}</Text>
              </View>
            ))}
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
          {pending ? <ActivityIndicator color={palette.inkInverse} size="small" /> : null}
          <Text style={[styles.buttonText, !canPair && !pending && styles.buttonTextDisabled]}>{pending ? "正在配对…" : "配对并连接"}</Text>
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

const styles = StyleSheet.create({
  root: { backgroundColor: palette.canvas, flex: 1 },
  content: { alignItems: "center", flexGrow: 1, paddingHorizontal: 24 },
  hero: { height: 176, width: 176 },
  title: { color: palette.ink1, fontSize: font.display + 2, fontWeight: weight.semibold, marginTop: 4 },
  sub: { color: palette.ink2, fontSize: font.body - 1, lineHeight: 22, marginTop: 8, textAlign: "center" },
  otp: { flexDirection: "row", gap: 8, justifyContent: "center", marginTop: 26, maxWidth: 320, width: "100%" },
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
  cellActive: { borderColor: palette.brand, borderWidth: 2 },
  cellText: { color: palette.ink1, fontFamily: fontFamily.mono, fontSize: font.display, fontWeight: weight.semibold },
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
  switchLink: { color: palette.ink3, fontSize: font.caption, marginTop: 12 },
  button: {
    alignItems: "center",
    backgroundColor: palette.brand,
    borderRadius: radius.lg,
    flexDirection: "row",
    gap: 8,
    justifyContent: "center",
    marginTop: 22,
    maxWidth: 360,
    minHeight: 52,
    width: "100%",
  },
  buttonDisabled: { backgroundColor: palette.well },
  pressed: { opacity: 0.85 },
  buttonText: { color: palette.inkInverse, fontSize: font.bodyLg, fontWeight: weight.semibold },
  buttonTextDisabled: { color: palette.ink4 },
  error: { color: palette.err, fontSize: font.label, lineHeight: 20, marginTop: 12, textAlign: "center" },
  spacer: { flex: 1, minHeight: 24 },
  relay: { alignItems: "center", flexDirection: "row", gap: 6, maxWidth: "100%" },
  relayText: { color: palette.ink3, flexShrink: 1, fontSize: font.caption },
  relayEdit: { alignItems: "center", flexDirection: "row", gap: 10, width: "100%" },
  relayInput: {
    backgroundColor: palette.surface1,
    borderRadius: radius.md - 2,
    color: palette.ink1,
    flex: 1,
    fontSize: font.label,
    height: 44,
    paddingHorizontal: 12,
  },
  link: { color: palette.brandLight, fontSize: font.label },
});
