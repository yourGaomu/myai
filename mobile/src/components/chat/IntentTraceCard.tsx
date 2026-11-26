import { createContext, useContext, useEffect, useRef, useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font } from "../../constants/theme";
import type { IntentTracePayload } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { useChatStyles } from "./styles";

type DetailSource = {
  details: Record<string, IntentTracePayload>;
  errors: Record<string, string>;
  loadingID: string;
  request: (traceID: string) => boolean;
};

export const IntentTraceDetailContext = createContext<DetailSource | null>(null);

const choices: Record<string, string> = {
  conversation: "普通对话", explanation: "解释说明", implementation: "项目实现", resume_plan: "继续计划",
};
const routes: Record<string, string> = {
  chat: "普通聊天", plan_only: "生成计划，等待确认", plan_execute: "建议规划并执行", plan_resume: "恢复计划",
};
const failures: Record<string, string> = {
  http_error: "请求失败", timeout: "请求超时", invalid_response: "响应无法解析",
  cancelled: "已取消", record_error: "记录保存失败",
};

export function IntentTraceCard({ buttonFeedback, trace: summary }: { buttonFeedback: ButtonFeedback; trace: IntentTracePayload }) {
  const { palette } = useAppTheme();
  const styles = useChatStyles();
  const local = useThemedStyles(makeStyles);
  const source = useContext(IntentTraceDetailContext);
  const [expanded, setExpanded] = useState(false);
  const record = source?.details[summary.trace_id];
  const detail = record?.session_id === summary.session_id ? record : undefined;
  const trace = summary.status === "started" && detail?.status !== "started" ? detail ?? summary : summary;
  const started = trace.status === "started";
  const succeeded = trace.status === "succeeded";
  const failed = !started && !succeeded;
  const requestBody = trace.request_body ?? detail?.request_body;
  const responseBody = trace.response_body ?? detail?.response_body;
  const needsDetails = !requestBody || (!started && !responseBody && detail?.status !== trace.status);
  const request = source?.request;
  const requested = useRef("");
  const requestKey = `${trace.trace_id}:${trace.status}`;
  const error = source?.errors[trace.trace_id];
  const loading = source?.loadingID === trace.trace_id;
  const input = readLatestRequest(requestBody);

  useEffect(() => {
    if (!expanded || !needsDetails || loading || !request || requested.current === requestKey) return;
    requested.current = requestKey;
    request(trace.trace_id);
  }, [expanded, loading, needsDetails, request, requestKey, trace.trace_id]);

  const status = started ? "调用中" : succeeded ? "已返回" : failures[trace.status] || "调用失败";
  return (
    <View style={[styles.message, styles.toolMessage]}>
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ expanded }}
        onPress={() => setExpanded((value) => !value)}
        style={({ pressed }) => buttonFeedback(styles.toolHeader, pressed)}
      >
        {started ? <ActivityIndicator color={palette.brand} size="small" /> : (
          <Feather color={failed ? palette.err : palette.brand} name={failed ? "alert-circle" : "cpu"} size={16} />
        )}
        <View style={local.heading}>
          <Text style={styles.toolTitle}>Jev 意图判断</Text>
          {input ? <Text numberOfLines={1} style={local.caption}>{input}</Text> : null}
        </View>
        <Text style={[styles.toolSubtitle, failed && styles.toolErrorText]}>{status}</Text>
        <Feather color={palette.ink4} name={expanded ? "chevron-down" : "chevron-right"} size={14} />
      </Pressable>
      {trace.choice ? (
        <Text style={local.outcome}>
          {choices[trace.choice] || trace.choice}
          {` · 置信度 ${((trace.confidence ?? 0) * 100).toFixed(0)}%`}
          {trace.route ? ` · ${routes[trace.route] || trace.route}` : ""}
        </Text>
      ) : null}
      {expanded ? (
        <View style={styles.toolBody}>
          <Text style={styles.toolSectionTitle}>
            {trace.response_model || trace.requested_model || "Jev"}
            {!started ? ` · ${trace.duration_ms} ms` : ""}
            {trace.http_status ? ` · HTTP ${trace.http_status}` : ""}
          </Text>
          {needsDetails && loading ? <Text style={local.caption}>正在读取调用详情…</Text> : null}
          {needsDetails && error ? (
            <Pressable accessibilityRole="button" onPress={() => request?.(trace.trace_id)}>
              <Text style={styles.toolErrorText}>{error}</Text>
            </Pressable>
          ) : null}
          <View style={styles.toolSection}>
            <Text style={styles.toolSectionTitle}>请求输入</Text>
            <Text selectable style={styles.toolCode}>{formatJSON(requestBody) || (loading ? "读取中…" : "无可用记录")}</Text>
          </View>
          <View style={styles.toolSection}>
            <Text style={styles.toolSectionTitle}>原始输出</Text>
            <Text selectable style={styles.toolCode}>
              {formatJSON(responseBody) || (started ? "等待响应…" : loading ? "读取中…" : "无响应正文")}
            </Text>
            {trace.response_truncated ? <Text style={local.caption}>响应超过记录上限，已截断。</Text> : null}
          </View>
        </View>
      ) : null}
    </View>
  );
}

function readLatestRequest(body?: string) {
  if (!body) return "";
  try {
    const value = JSON.parse(body)?.state?.latest_request;
    return typeof value === "string" ? value : "";
  } catch {
    return "";
  }
}

function formatJSON(body?: string) {
  if (!body) return "";
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) => StyleSheet.create({
  heading: { flex: 1, gap: 2, minWidth: 0 },
  caption: { color: palette.ink3, fontSize: font.caption, lineHeight: 17 },
  outcome: { color: palette.ink2, fontSize: font.caption, lineHeight: 18, marginLeft: 24 },
});
