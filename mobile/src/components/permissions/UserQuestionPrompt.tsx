import { useMemo, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

import type { UserQuestionState } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";

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
                  <Text style={styles.optionText}>{selected ? "●  " : "○  "}{option}</Text>
                </Pressable>
              );
            })}
            {item.allow_other || !item.options?.length ? (
              <TextInput
                editable={!question.submitted}
                onChangeText={(value) => setAnswer(item.id, value)}
                placeholder={item.options?.length ? "其他答案" : "输入你的回答"}
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
    backgroundColor: "#fff8db",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 10,
    maxHeight: 420,
    padding: 12,
  },
  title: { color: "#12100e", fontSize: 16, fontWeight: "900" },
  subtitle: { color: "#4b4538", lineHeight: 20 },
  questions: { flexGrow: 0, maxHeight: 285 },
  item: { gap: 7, paddingVertical: 5 },
  prompt: { color: "#12100e", fontWeight: "700", lineHeight: 21 },
  option: {
    backgroundColor: "#ffffff",
    borderColor: "#59564e",
    borderRadius: 6,
    borderWidth: 1,
    minHeight: 40,
    justifyContent: "center",
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  optionSelected: { backgroundColor: "#e7f3e7", borderColor: "#216e39", borderWidth: 2 },
  optionText: { color: "#12100e", lineHeight: 20 },
  answerInput: {
    backgroundColor: "#ffffff",
    borderColor: "#6b665a",
    borderRadius: 6,
    borderWidth: 1,
    color: "#12100e",
    minHeight: 42,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  row: { flexDirection: "row", gap: 8 },
  flex: { flex: 1, minWidth: 0 },
  primaryButton: { alignItems: "center", backgroundColor: "#ffd84f", borderColor: "#12100e", borderRadius: 7, borderWidth: 2, justifyContent: "center", minHeight: 44, paddingHorizontal: 10 },
  primaryButtonText: { color: "#12100e", fontWeight: "800" },
  secondaryButton: { alignItems: "center", backgroundColor: "#ffffff", borderColor: "#59564e", borderRadius: 7, borderWidth: 1, justifyContent: "center", minHeight: 44, paddingHorizontal: 10 },
  secondaryButtonText: { color: "#12100e", fontWeight: "700" },
  disabled: { opacity: 0.45 },
  waiting: { color: "#4b4538", fontWeight: "700", textAlign: "center" },
});
