import { useMemo, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { UserQuestionState } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { font, palette, radius, weight } from "../../constants/theme";

type Props = {
  buttonFeedback: ButtonFeedback;
  onAnswer: (answers: Record<string, string>, cancelled?: boolean) => boolean;
  question: UserQuestionState;
};

export function UserQuestionPrompt({ buttonFeedback, onAnswer, question }: Props) {
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const canSubmit = useMemo(
    () => question.questions.every((item) => Boolean(answers[item.id]?.trim())),
    [answers, question.questions],
  );

  const setAnswer = (id: string, value: string) => {
    setAnswers((current) => ({ ...current, [id]: value }));
  };

  return (
    <View style={styles.questionBox}>
      <Text style={styles.title}>{question.sourceTaskID ? "子代理需要你的意见" : question.title}</Text>
      {question.title && question.sourceTaskID ? <Text style={styles.subtitle}>{question.title}</Text> : null}
      <ScrollView nestedScrollEnabled style={styles.questions}>
        {question.questions.map((item, index) => (
          <View key={item.id} style={styles.item}>
            <Text style={styles.prompt}>{question.questions.length > 1 ? `${index + 1}. ` : ""}{item.prompt}</Text>
            {item.options?.map((option) => {
              const selected = answers[item.id] === option;
              return (
                <Pressable
                  key={option}
                  accessibilityRole="radio"
                  accessibilityState={{ selected }}
                  disabled={question.submitted}
                  onPress={() => setAnswer(item.id, option)}
                  style={({ pressed }) => buttonFeedback([styles.option, selected && styles.optionSelected], pressed)}
                >
                  <Feather color={selected ? palette.ok : palette.ink3} name={selected ? "check-circle" : "circle"} size={14} />
                  <Text style={styles.optionText}>{option}</Text>
                </Pressable>
              );
            })}
            {item.allow_other || !item.options?.length ? (
              <TextInput
                editable={!question.submitted}
                onChangeText={(value) => setAnswer(item.id, value)}
                placeholder={item.options?.length ? "其他答案" : "输入你的回答"}
                placeholderTextColor={palette.ink4}
                style={styles.answerInput}
                value={answers[item.id] || ""}
              />
            ) : null}
          </View>
        ))}
      </ScrollView>
      {question.submitted ? (
        <Text style={styles.waiting}>已提交，等待助手继续</Text>
      ) : (
        <View style={styles.row}>
          <Pressable onPress={() => onAnswer({}, true)} style={({ pressed }) => buttonFeedback([styles.secondaryButton, styles.flex], pressed)}>
            <Text style={styles.secondaryButtonText}>暂不回答</Text>
          </Pressable>
          <Pressable disabled={!canSubmit} onPress={() => onAnswer(answers)} style={({ pressed }) => buttonFeedback([styles.primaryButton, styles.flex, !canSubmit && styles.disabled], pressed)}>
            <Text style={styles.primaryButtonText}>提交回答</Text>
          </Pressable>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  questionBox: {
    backgroundColor: palette.brandWashStrong,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 10,
    maxHeight: 420,
    padding: 12,
  },
  title: { color: palette.ink1, fontSize: font.title, fontWeight: weight.bold },
  subtitle: { color: palette.ink1, lineHeight: 20 },
  questions: { flexGrow: 0, maxHeight: 285 },
  item: { gap: 7, paddingVertical: 5 },
  prompt: { color: palette.ink1, fontWeight: weight.bold, lineHeight: 21 },
  option: {
    alignItems: "center",
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 8,
    minHeight: 40,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  optionSelected: { backgroundColor: palette.okWash, borderColor: palette.okBorder, borderWidth: 1 },
  optionText: { color: palette.ink1, flex: 1, lineHeight: 20 },
  answerInput: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    color: palette.ink1,
    minHeight: 42,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  row: { flexDirection: "row", gap: 8 },
  flex: { flex: 1, minWidth: 0 },
  primaryButton: { alignItems: "center", backgroundColor: palette.brand, borderColor: palette.hairline, borderRadius: radius.sm, borderWidth: 1, justifyContent: "center", minHeight: 44, paddingHorizontal: 10 },
  primaryButtonText: { color: palette.inkInverse, fontWeight: weight.bold },
  secondaryButton: { alignItems: "center", backgroundColor: palette.surface2, borderColor: palette.hairline, borderRadius: radius.sm, borderWidth: 1, justifyContent: "center", minHeight: 44, paddingHorizontal: 10 },
  secondaryButtonText: { color: palette.ink1, fontWeight: weight.bold },
  disabled: { opacity: 0.45 },
  waiting: { color: palette.ink1, fontWeight: weight.bold, textAlign: "center" },
});
