import { ActivityIndicator, Pressable, StyleSheet, Text, TextInput, View } from "react-native";

import { font, radius, weight } from "../../constants/theme";
import type { SessionRenameDraft } from "../../hooks/useSessionRename";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { maxSessionTitleLength, validateSessionTitle } from "../../utils/sessionRename";

type Props = {
  connected: boolean;
  draft: SessionRenameDraft;
  onCancel: () => void;
  onChange: (value: string) => void;
  onSave: () => void;
};

export function SessionRenameEditor({ connected, draft, onCancel, onChange, onSave }: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const { length, error } = validateSessionTitle(draft.input);
  const status = draft.feedback?.status;
  const saving = status === "pending";
  const blocked = saving || status === "unconfirmed" || status === "unavailable";
  const canSave = connected && !blocked && !error;
  const message = draft.feedback?.message || (!connected ? "请等待中继连接恢复。" : length > maxSessionTitleLength ? error : "");

  return (
    <View style={styles.editor}>
      <View style={styles.labelRow}>
        <Text style={styles.label}>会话标题</Text>
        <Text style={[styles.count, length > maxSessionTitleLength && styles.error]}>{length} / {maxSessionTitleLength}</Text>
      </View>
      <TextInput
        accessibilityLabel="会话标题"
        autoFocus
        editable={!blocked}
        onChangeText={onChange}
        onSubmitEditing={() => { if (canSave) onSave(); }}
        placeholder="输入会话标题"
        placeholderTextColor={palette.ink4}
        returnKeyType="done"
        style={styles.input}
        value={draft.input}
      />
      {message ? <Text accessibilityLiveRegion="polite" style={[styles.label, styles.error]}>{message}</Text> : null}
      <View style={styles.actions}>
        <Pressable accessibilityRole="button" onPress={onCancel} style={({ pressed }) => [styles.button, pressed && styles.pressed]}>
          <Text style={styles.cancel}>{saving || status === "unconfirmed" || status === "unavailable" ? "关闭" : "取消"}</Text>
        </Pressable>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={saving ? "正在保存标题" : "保存标题"}
          accessibilityState={{ busy: saving, disabled: !canSave }}
          disabled={!canSave}
          onPress={onSave}
          style={({ pressed }) => [styles.button, styles.save, pressed && styles.pressed, !canSave && styles.disabled]}
        >
          {saving ? <ActivityIndicator color={palette.inkInverse} size="small" /> : <Text style={styles.saveText}>保存</Text>}
        </Pressable>
      </View>
    </View>
  );
}

const makeStyles = ({ palette }: AppTheme) => StyleSheet.create({
  editor: { gap: 8, paddingVertical: 8 },
  labelRow: { flexDirection: "row", justifyContent: "space-between", gap: 8 },
  label: { color: palette.ink2, fontSize: font.label },
  count: { color: palette.ink3, fontSize: font.caption, fontVariant: ["tabular-nums"] },
  error: { color: palette.err },
  input: {
    backgroundColor: palette.well,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.md,
    borderWidth: 1,
    color: palette.ink1,
    fontSize: font.body,
    minHeight: 44,
    paddingHorizontal: 12,
  },
  actions: { flexDirection: "row", gap: 8, justifyContent: "flex-end" },
  button: { alignItems: "center", borderRadius: radius.md, justifyContent: "center", minHeight: 44, minWidth: 72, paddingHorizontal: 12 },
  cancel: { color: palette.ink2, fontSize: font.label },
  save: { backgroundColor: palette.brand },
  saveText: { color: palette.inkInverse, fontSize: font.label, fontWeight: weight.semibold },
  pressed: { opacity: 0.72 },
  disabled: { opacity: 0.5 },
});
