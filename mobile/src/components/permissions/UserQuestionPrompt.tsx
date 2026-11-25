import { useEffect, useMemo, useRef, useState } from "react";
import { Animated, Platform, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";
import * as Haptics from "expo-haptics";

import type { UserQuestionAnswerSubmission, UserQuestionDraftAnswer, UserQuestionState } from "../../types/app";
import {
  applySingleSelectOption, applySingleSelectOther, buildUserQuestionSubmission, canAdoptRecommended,
  emptyQuestionDraft, getQuestionDraft, isQuestionDraftComplete,
  mergeWithRecommendedDefaults, toggleMultiSelectOption, updateMultiSelectOther, validateQuestionChoices,
} from "../../utils/userQuestion";
import type { ButtonFeedback } from "../../types/ui";
import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { FadeSlideView, useReduceMotion } from "../common/MotionEffects";

type Props = {
  buttonFeedback: ButtonFeedback;
  onAnswer: (submission: UserQuestionAnswerSubmission, cancelled?: boolean) => boolean;
  onChangeDraft?: (drafts: Record<string, UserQuestionDraftAnswer>) => void;
  question: UserQuestionState;
};

// 对话流内联提问卡片：支持单题/多题分页指示、预设单选、自定义补充说明与跳过取消。
export function UserQuestionPrompt({ buttonFeedback, onAnswer, onChangeDraft, question }: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const reduceMotion = useReduceMotion();
  const [localDrafts, setLocalDrafts] = useState<Record<string, UserQuestionDraftAnswer>>(() => question.draftAnswers || {});
  const drafts = onChangeDraft ? question.draftAnswers || localDrafts : localDrafts;
  const [activeIndex, setActiveIndex] = useState(0);
  const cardScale = useRef(new Animated.Value(1)).current;
  const autoAdvance = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(autoAdvance.current), []);

  const items = question.questions;
  const total = items.length;
  const safeIndex = Math.min(activeIndex, Math.max(0, total - 1));
  const currentItem = items[safeIndex];
  const currentDraft = (currentItem && getQuestionDraft(drafts, currentItem.id)) || emptyQuestionDraft();
  const customAnswerActive = currentDraft.other.trim() !== "";

  const protocolError = useMemo(() => {
    try { items.forEach(validateQuestionChoices); return ""; }
    catch (error) { return error instanceof Error ? error.message : "问答协议版本不匹配，请同时升级客户端与服务端。"; }
  }, [items]);
  const canSubmit = useMemo(() => items.every((item) => isQuestionDraftComplete(getQuestionDraft(drafts, item.id))), [drafts, items]);
  const answeredCount = useMemo(() => items.filter((item) => isQuestionDraftComplete(getQuestionDraft(drafts, item.id))).length, [drafts, items]);
  const recommendedAvailable = useMemo(() => canAdoptRecommended(items, drafts), [items, drafts]);

  const setDraft = (id: string, draft: UserQuestionDraftAnswer) => {
    const next = { ...drafts, [id]: draft };
    if (onChangeDraft) onChangeDraft(next);
    else setLocalDrafts(next);
  };
  const submitRecommended = () => {
    if (!recommendedAvailable || protocolError) return;
    if (Platform.OS !== "web") void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success).catch(() => {});
    pulseCard();
    const merged = mergeWithRecommendedDefaults(items, drafts);
    const submission = buildUserQuestionSubmission(items, merged, true);
    if (onChangeDraft) onChangeDraft(merged); else setLocalDrafts(merged);
    onAnswer(submission);
  };
  const selectQuestion = (index: number) => {
    clearTimeout(autoAdvance.current);
    setActiveIndex(index);
  };

  const handleDismiss = () => {
    clearTimeout(autoAdvance.current);
    if (Platform.OS !== "web") void Haptics.selectionAsync().catch(() => {});
    onAnswer({ structuredAnswers: {} }, true);
  };

  const pulseCard = () => {
    if (!reduceMotion) {
      cardScale.setValue(0.985);
      Animated.spring(cardScale, { bounciness: 6, speed: 24, toValue: 1, useNativeDriver: true }).start();
    }
  };

  const handleConfirm = () => {
    if (Platform.OS !== "web") void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success).catch(() => {});
    pulseCard();
    try {
      onAnswer(buildUserQuestionSubmission(items, drafts));
    } catch {
      // The primary action is disabled until every question has a valid draft.
    }
  };

  return (
    <Animated.View style={[styles.questionBox, { transform: [{ scale: cardScale }] }]}>
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
            const done = isQuestionDraftComplete(getQuestionDraft(drafts, item.id));
            return (
              <Pressable
                key={item.id}
                onPress={() => selectQuestion(idx)}
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
        <ScrollView keyboardShouldPersistTaps="handled" nestedScrollEnabled style={styles.questions}>
          <FadeSlideView direction="right" keyProp={currentItem.id} style={styles.item}>
            <Text style={styles.prompt}>
              {total > 1 ? `${safeIndex + 1}. ` : ""}
              {currentItem.prompt}
            </Text>
            {currentItem.multi_select ? <Text style={styles.multiSelectBadge}>{currentItem.allow_other ? "多选 · 可叠加补充" : "多选"}</Text> : null}
            {currentItem.choices.map((choice) => {
              const selected = currentDraft.selected.includes(choice.label);
              return (
                <Pressable
                  key={choice.label}
                  accessibilityRole={currentItem.multi_select ? "checkbox" : "radio"}
                  accessibilityState={{ selected }}
                  disabled={question.submitted || Boolean(protocolError)}
                  onPress={() => {
                    if (Platform.OS !== "web") void Haptics.selectionAsync().catch(() => {});
                    if (currentItem.multi_select) {
                      setDraft(currentItem.id, toggleMultiSelectOption(currentDraft, choice.label));
                    } else {
                      setDraft(currentItem.id, applySingleSelectOption(choice.label));
                      if (total > 1 && safeIndex < total - 1) {
                        clearTimeout(autoAdvance.current);
                        autoAdvance.current = setTimeout(() => setActiveIndex(safeIndex + 1), 160);
                      }
                    }
                  }}
                  style={({ pressed }) => buttonFeedback([styles.option, choice.recommended && styles.optionRecommended, selected && styles.optionSelected], pressed)}
                >
                  <Feather color={selected ? palette.brandLight : palette.ink3} name={currentItem.multi_select ? (selected ? "check-square" : "square") : (selected ? "check-circle" : "circle")} size={17} />
                  <View style={styles.choiceBody}>
                    <View style={styles.choiceTitleRow}>
                      <Text style={[styles.optionText, selected && { color: palette.ink1, fontWeight: weight.medium }]}>{choice.label}</Text>
                      {choice.recommended ? <Text style={styles.recommendedBadge}>推荐</Text> : null}
                    </View>
                    {choice.description ? <Text numberOfLines={3} style={styles.choiceDescription}>{choice.description}</Text> : null}
                  </View>
                </Pressable>
              );
            })}
            {currentItem.allow_other ? (
              <>
                <TextInput
                  editable={!question.submitted && !protocolError}
                  onChangeText={(value) => {
                    clearTimeout(autoAdvance.current);
                    setDraft(currentItem.id, currentItem.multi_select ? updateMultiSelectOther(currentDraft, value) : applySingleSelectOther(value));
                  }}
                  placeholder={currentItem.multi_select ? "补充其他要求（可与上方选项同时生效）…" : currentItem.choices.length ? "或者输入自定义回答…" : "输入你的回答"}
                  placeholderTextColor={palette.ink4}
                  style={[styles.answerInput, customAnswerActive && styles.optionRecommended]}
                  value={currentDraft.other}
                />
                {customAnswerActive && currentItem.multi_select && currentDraft.selected.length > 0 ? (
                  <Text style={styles.choiceDescription}>已选 {currentDraft.selected.length} 项并添加补充说明</Text>
                ) : null}
                {customAnswerActive && !currentItem.multi_select && currentItem.choices.length > 0 ? (
                  <Text style={styles.choiceDescription}>已使用自定义回答</Text>
                ) : null}
              </>
            ) : null}
            {protocolError ? <Text style={styles.protocolError}>{protocolError}</Text> : null}
          </FadeSlideView>
        </ScrollView>
      ) : null}

      {question.submitted ? (
        <View style={styles.waitingBar}>
          <Feather color={palette.ok} name="check" size={14} />
          <View style={styles.waitingBody}>
            <Text style={styles.waiting}>已提交回答，等待助手继续执行…</Text>
            {question.submittedSummary ? <Text style={styles.submittedSummary}>{question.submittedSummary}</Text> : null}
          </View>
        </View>
      ) : (
        <>
        {recommendedAvailable ? (
          <Pressable disabled={Boolean(protocolError)} onPress={submitRecommended} style={({ pressed }) => buttonFeedback([styles.recommendedAction], pressed)}>
            <Text style={styles.recommendedActionText}>{answeredCount === 0 ? "按 AI 推荐方案直接开始" : "其余采用推荐并提交"}</Text>
          </Pressable>
        ) : null}
        <View style={styles.row}>
          <Pressable
            onPress={handleDismiss}
            style={({ pressed }) => buttonFeedback([styles.secondaryButton], pressed)}
          >
            <Text style={styles.secondaryButtonText}>暂不回答</Text>
          </Pressable>
          {total > 1 && safeIndex < total - 1 ? (
            <Pressable
              onPress={() => selectQuestion(safeIndex + 1)}
              style={({ pressed }) => buttonFeedback([styles.nextButton, styles.flex], pressed)}
            >
              <Text style={styles.nextButtonText}>下一题 ({answeredCount}/{total})</Text>
            </Pressable>
          ) : (
            <Pressable
              disabled={!canSubmit || Boolean(protocolError)}
              onPress={handleConfirm}
              style={({ pressed }) => buttonFeedback([styles.primaryButton, styles.flex, (!canSubmit) && styles.disabled], pressed)}
            >
              <Text style={styles.primaryButtonText}>确认提交</Text>
            </Pressable>
          )}
        </View>
        </>
      )}
    </Animated.View>
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
    choiceBody: { flex: 1, gap: 3 },
    choiceTitleRow: { alignItems: "center", flexDirection: "row", gap: 8 },
    choiceDescription: { color: palette.ink3, fontSize: 12, lineHeight: 17 },
    multiSelectBadge: { alignSelf: "flex-start", backgroundColor: palette.brandWash, borderRadius: radius.pill, color: palette.brandLight, fontSize: 11, paddingHorizontal: 8, paddingVertical: 3 },
    recommendedBadge: { backgroundColor: palette.brandWashStrong, borderColor: palette.brandDeep, borderRadius: radius.pill, borderWidth: 1, color: palette.brandLight, fontSize: 10, fontWeight: weight.semibold, overflow: "hidden", paddingHorizontal: 7, paddingVertical: 2 },
    protocolError: { color: palette.ink1, fontSize: 12 },
    prompt: {
      color: palette.ink1,
      fontSize: font.body,
      fontWeight: weight.medium,
      lineHeight: 22,
    },
    option: {
      alignItems: "flex-start",
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
    optionRecommended: { borderColor: palette.brandDeep },
    optionSelected: {
      backgroundColor: palette.brandWash,
      borderColor: palette.brand,
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
    waitingBody: { flex: 1, gap: 4 },
    submittedSummary: { color: palette.ink3, fontSize: 12, lineHeight: 17 },
    recommendedAction: { alignItems: "center", backgroundColor: palette.brandWash, borderColor: palette.brandDeep, borderRadius: radius.md, borderWidth: 1, justifyContent: "center", minHeight: 40, paddingHorizontal: 12 },
    recommendedActionText: { color: palette.brandLight, fontSize: font.label, fontWeight: weight.semibold },
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
