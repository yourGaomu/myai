import { useMemo, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { UserQuestionState } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { CharacterAvatar } from "../common/CharacterAvatar";

type Props = {
  buttonFeedback: ButtonFeedback;
  onAnswer: (answers: Record<string, string>, cancelled?: boolean) => boolean;
  question: UserQuestionState;
};

// 对话流内联提问卡片：支持单题/多题分页指示、预设单选、自定义补充说明与跳过取消。
export function UserQuestionPrompt({ buttonFeedback, onAnswer, question }: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [activeIndex, setActiveIndex] = useState(0);

  const items = question.questions;
  const total = items.length;
  const safeIndex = Math.min(activeIndex, Math.max(0, total - 1));
  const currentItem = items[safeIndex];

  const canSubmit = useMemo(
    () => items.every((item) => Boolean(answers[item.id]?.trim())),
    [answers, items],
  );
  const answeredCount = useMemo(
    () => items.filter((item) => Boolean(answers[item.id]?.trim())).length,
    [answers, items],
  );

  const setAnswer = (id: string, value: string) => {
    setAnswers((current) => ({ ...current, [id]: value }));
  };

  return (
    <View style={styles.questionBox}>
      <View style={styles.head}>
        <CharacterAvatar circular size={24} slot="attention" variant="avatar" />
        <View style={styles.headBody}>
          <Text style={styles.badge}>
            {question.sourceTaskID ? "子代理请求确认" : "需要你的确认"}
            {total > 1 ? ` · 第 ${safeIndex + 1}/${total} 题` : ""}
          </Text>
          <Text style={styles.title}>{question.title || "请补充以下信息以继续执行"}</Text>
        </View>
      </View>

      {total > 1 ? (
        <View style={styles.stepTabs}>
          {items.map((item, idx) => {
            const selected = idx === safeIndex;
            const done = Boolean(answers[item.id]?.trim());
            return (
              <Pressable
                key={item.id}
                onPress={() => setActiveIndex(idx)}
                style={[styles.stepTab, selected && styles.stepTabActive, done && !selected && styles.stepTabDone]}
              >
                <Text style={[styles.stepTabText, selected && styles.stepTabTextActive]}>
                  {done ? "✓ " : ""}问题 {idx + 1}
                </Text>
              </Pressable>
            );
          })}
        </View>
      ) : null}

      {currentItem ? (
        <ScrollView nestedScrollEnabled style={styles.questions}>
          <View style={styles.item}>
            <Text style={styles.prompt}>
              {total > 1 ? `${safeIndex + 1}. ` : ""}
              {currentItem.prompt}
            </Text>
            {currentItem.options?.map((option) => {
              const selected = answers[currentItem.id] === option;
              return (
                <Pressable
                  key={option}
                  accessibilityRole="radio"
                  accessibilityState={{ selected }}
                  disabled={question.submitted}
                  onPress={() => {
                    setAnswer(currentItem.id, option);
                    if (total > 1 && safeIndex < total - 1) {
                      setTimeout(() => setActiveIndex(safeIndex + 1), 160);
                    }
                  }}
                  style={({ pressed }) => buttonFeedback([styles.option, selected && styles.optionSelected], pressed)}
                >
                  <Feather color={selected ? palette.brandLight : palette.ink3} name={selected ? "check-circle" : "circle"} size={17} />
                  <Text style={[styles.optionText, selected && { color: palette.ink1, fontWeight: weight.medium }]}>{option}</Text>
                </Pressable>
              );
            })}
            {currentItem.allow_other || !currentItem.options?.length ? (
              <TextInput
                editable={!question.submitted}
                onChangeText={(value) => setAnswer(currentItem.id, value)}
                placeholder={currentItem.options?.length ? "或者输入自定义补充答案…" : "输入你的回答"}
                placeholderTextColor={palette.ink4}
                style={styles.answerInput}
                value={answers[currentItem.id] || ""}
              />
            ) : null}
          </View>
        </ScrollView>
      ) : null}

      {question.submitted ? (
        <View style={styles.waitingBar}>
          <Feather color={palette.ok} name="check" size={14} />
          <Text style={styles.waiting}>已提交回答，等待助手继续执行…</Text>
        </View>
      ) : (
        <View style={styles.row}>
          <Pressable
            onPress={() => onAnswer({}, true)}
            style={({ pressed }) => buttonFeedback([styles.secondaryButton], pressed)}
          >
            <Text style={styles.secondaryButtonText}>暂不回答</Text>
          </Pressable>
          {total > 1 && safeIndex < total - 1 ? (
            <Pressable
              onPress={() => setActiveIndex(safeIndex + 1)}
              style={({ pressed }) => buttonFeedback([styles.nextButton, styles.flex], pressed)}
            >
              <Text style={styles.nextButtonText}>下一题 ({answeredCount}/{total})</Text>
            </Pressable>
          ) : (
            <Pressable
              disabled={!canSubmit}
              onPress={() => onAnswer(answers)}
              style={({ pressed }) => buttonFeedback([styles.primaryButton, styles.flex, !canSubmit && styles.disabled], pressed)}
            >
              <Text style={styles.primaryButtonText}>确认提交</Text>
            </Pressable>
          )}
        </View>
      )}
    </View>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    questionBox: {
      backgroundColor: palette.surface1,
      borderColor: palette.brandDeep,
      borderRadius: radius.lg,
      borderWidth: 1,
      gap: 10,
      padding: 14,
    },
    head: {
      alignItems: "flex-start",
      flexDirection: "row",
      gap: 10,
    },
    headBody: {
      flex: 1,
      gap: 2,
      minWidth: 0,
    },
    badge: {
      color: palette.brandLight,
      fontSize: 11,
      fontWeight: weight.semibold,
    },
    title: {
      color: palette.ink1,
      fontSize: font.bodyLg,
      fontWeight: weight.semibold,
    },
    stepTabs: {
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 6,
    },
    stepTab: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      paddingHorizontal: 10,
      paddingVertical: 4,
    },
    stepTabActive: {
      backgroundColor: palette.brandWashStrong,
      borderColor: palette.brand,
    },
    stepTabDone: {
      borderColor: palette.okBorder,
    },
    stepTabText: {
      color: palette.ink3,
      fontSize: 11.5,
      fontWeight: weight.medium,
    },
    stepTabTextActive: {
      color: palette.brandLight,
      fontWeight: weight.semibold,
    },
    questions: { flexGrow: 0, maxHeight: 290 },
    item: { gap: 7, paddingVertical: 2 },
    prompt: {
      color: palette.ink1,
      fontSize: font.body,
      fontWeight: weight.medium,
      lineHeight: 22,
    },
    option: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 10,
      minHeight: 44,
      paddingHorizontal: 12,
      paddingVertical: 9,
    },
    optionSelected: {
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
    },
    optionText: {
      color: palette.ink2,
      flex: 1,
      fontSize: font.label + 0.5,
      lineHeight: 20,
    },
    answerInput: {
      backgroundColor: palette.canvas,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.md,
      borderWidth: 1,
      color: palette.ink1,
      fontSize: font.body,
      minHeight: 44,
      paddingHorizontal: 12,
      paddingVertical: 10,
    },
    waitingBar: {
      alignItems: "center",
      flexDirection: "row",
      gap: 6,
    },
    waiting: {
      color: palette.ok,
      fontSize: font.label,
      fontWeight: weight.medium,
    },
    row: { flexDirection: "row", gap: 8 },
    flex: { flex: 1 },
    secondaryButton: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      justifyContent: "center",
      minHeight: 42,
      paddingHorizontal: 14,
    },
    secondaryButtonText: {
      color: palette.ink2,
      fontSize: font.label,
      fontWeight: weight.medium,
    },
    nextButton: {
      alignItems: "center",
      backgroundColor: palette.brandWashStrong,
      borderColor: palette.brand,
      borderRadius: radius.md,
      borderWidth: 1,
      justifyContent: "center",
      minHeight: 42,
      paddingHorizontal: 14,
    },
    nextButtonText: {
      color: palette.brandLight,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    primaryButton: {
      alignItems: "center",
      backgroundColor: palette.brand,
      borderRadius: radius.md,
      justifyContent: "center",
      minHeight: 42,
      paddingHorizontal: 14,
    },
    primaryButtonText: {
      color: palette.inkInverse,
      fontSize: font.body,
      fontWeight: weight.bold,
    },
    disabled: { opacity: 0.45 },
  });
