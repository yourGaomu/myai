import { Pressable, StyleSheet, Text, useWindowDimensions, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { ButtonContent } from "../../common/ButtonContent";
import { ConfirmModal } from "../../common/ConfirmModal";
import { fontFamily, radius, weight } from "../../../constants/theme";
import type { MCPDetail, MCPInventory } from "../../../hooks/useMCPState";
import { mcpTrustOutcomeUnclear, type MCPTrustControls } from "../../../hooks/useMCPTrust";
import type { MCPServerSummary, MCPToolSchemaSummary, MCPToolSummary, MCPTrustService } from "../../../protocol";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../../theme/ThemeProvider";
import type { ButtonFeedback } from "../../../types/ui";

export type MCPCatalogControls = {
  inventory: MCPInventory;
  detail: MCPDetail | null;
  reloading: boolean;
  reloadMessage: string;
  refresh: () => void;
  reload: () => void;
  openDetail: (serverName: string) => void;
  openTool: (toolID: string) => void;
  loadMoreDetail: () => void;
  retryDetail: () => void;
  trust: MCPTrustControls;
};

type Props = MCPCatalogControls & { connected: boolean; buttonFeedback: ButtonFeedback };

const serverStates: Record<MCPServerSummary["state"], string> = {
  loaded: "已加载", disabled: "已禁用", unavailable: "不可用",
};
const serverReasons: Record<NonNullable<MCPServerSummary["reason"]>, string> = {
  pending_trust: "待信任审批；审批通过后需手动重载 MCP。若没有手机审批入口，请使用电脑端 Agent。",
  start_failed: "服务启动失败，请检查电脑端 Agent。",
  discovery_failed: "服务工具发现失败，请检查电脑端 Agent。",
};
const eligibility: Record<MCPToolSummary["eligibility"], string> = {
  available: "当前会话可选", filtered: "被会话策略过滤", host_only: "仅 Agent 内部使用", not_evaluated: "未评估",
};
const eligibilityReasons: Record<NonNullable<MCPToolSummary["reason"]>, string> = {
  agent_mode: "对话／计划模式限制", permission_mode: "会话权限模式限制",
  permission_ceiling: "继承的权限上限限制", allowlist: "不在会话工具允许名单中",
  not_registered: "当前生效的模型工具目录未注册此工具",
};
const inventoryFeedback: Partial<Record<MCPInventory["status"], string>> = {
  loading: "正在获取 MCP 服务列表…",
  unsupported: "当前中继或电脑端 Agent 版本不支持查看 MCP 列表。",
  timeout: "MCP 列表请求超时。请确认电脑端 Agent 在线且支持此功能，然后刷新。",
  error: "无法获取 MCP 列表，请检查电脑端 Agent 后刷新。",
};
const detailFeedback: Partial<Record<MCPDetail["status"], string>> = {
  loading: "正在获取 MCP 工具详情…",
  unsupported: "当前中继或电脑端 Agent 版本不支持查看 MCP 工具详情。服务摘要仍可查看。",
  timeout: "MCP 工具详情请求超时。请确认电脑端 Agent 在线且支持工具详情，然后重试。",
  error: "无法获取 MCP 工具详情，请检查电脑端 Agent 后重试。",
  changed: "MCP 目录或会话评估状态已变化。请从第一页刷新工具目录。",
};
const schemaStates = {
  undeclared: "未声明", empty: "没有顶层字段", unsupported: "结构无法简要展示", fields: "",
};

function inventoryStatus({ servers, status }: MCPInventory, connected: boolean) {
  if (!connected) return servers === null
    ? "连接中继后可查看 MCP 服务列表。"
    : "中继未连接，显示上次获取的 MCP 列表。";
  if (status !== "ready" && servers !== null) {
    return [inventoryFeedback[status], "显示上次获取的 MCP 列表。"].filter(Boolean).join(" ");
  }
  if (status === "ready" && servers?.length === 0) return "没有已配置的 MCP 服务。";
  return inventoryFeedback[status];
}

function Status({ children }: { children: string }) {
  const styles = useThemedStyles(makeStyles);
  return <Text accessibilityLiveRegion="polite" role="status" style={styles.status}>{children}</Text>;
}

function ServiceSummary({ server }: { server: MCPServerSummary }) {
  const styles = useThemedStyles(makeStyles);
  const reason = server.reason && serverReasons[server.reason];
  return (
    <View style={styles.stack}>
      <Text style={styles.identifier}>{server.name}</Text>
      <Text style={styles.state}>{serverStates[server.state]}</Text>
      <Text style={styles.meta}>{`发现工具 ${server.inventory_tool_count} 个 · 模型暴露 ${server.exposed_tool_count} 个`}</Text>
      {reason ? <Text style={styles.meta}>{reason}</Text> : null}
    </View>
  );
}

function ToolEligibility({ tool }: { tool: MCPToolSummary }) {
  const styles = useThemedStyles(makeStyles);
  const reason = tool.eligibility === "filtered" && tool.reason && eligibilityReasons[tool.reason];
  return <Text style={styles.state}>{eligibility[tool.eligibility]}{reason ? ` · ${reason}` : ""}</Text>;
}

function Schema({ label, schema }: { label: string; schema: MCPToolSchemaSummary }) {
  const styles = useThemedStyles(makeStyles);
  const fields = schema.state === "fields" ? schema.fields : undefined;
  return (
    <View style={styles.card}>
      <Text accessibilityRole="header" style={styles.heading}>{label}</Text>
      {schemaStates[schema.state] ? <Text style={styles.meta}>{schemaStates[schema.state]}</Text> : null}
      {fields?.map((field, index) => (
        <View key={`${field.name}:${index}`} style={styles.field}>
          <Text selectable style={styles.identifier}>{field.name}</Text>
          <Text style={styles.meta}>{field.type || "类型未声明"} · {field.required ? "必填" : "选填"}</Text>
        </View>
      ))}
      {schema.truncated ? <Text style={styles.meta}>部分字段信息已截断。</Text> : null}
    </View>
  );
}

function ToolDetail({ sessionEvaluated, tool }: { sessionEvaluated: boolean; tool: MCPToolSummary }) {
  const styles = useThemedStyles(makeStyles);
  return (
    <View style={styles.section}>
      <View style={styles.card}>
        <Text accessibilityRole="header" style={styles.heading}>{tool.title || "工具详情"}</Text>
        <Text selectable style={styles.identifier}>{tool.name}</Text>
        <Text selectable style={styles.meta}>{tool.alias ? `模型别名：${tool.alias}` : "无模型别名"}</Text>
        <ToolEligibility tool={tool} />
        <Text style={styles.meta}>{sessionEvaluated
          ? "资格仅供参考；工具可选不代表本次执行已获授权。"
          : "未评估会话资格：未指定会话，或会话与电脑端 Agent 当前会话不匹配。"}</Text>
        {tool.description ? <Text selectable style={styles.body}>{tool.description}</Text> : null}
        {tool.metadata_truncated ? <Text style={styles.meta}>部分工具信息已截断。</Text> : null}
      </View>
      <Schema label="输入参数" schema={tool.input} />
      <Schema label="输出结构" schema={tool.output} />
    </View>
  );
}

const trustRuntime: Record<MCPTrustService["runtime"], string> = {
  loaded: "已加载当前配置", older: "已加载旧配置", not_loaded: "未加载",
};
const trustAuthentication: Record<MCPTrustService["authentication"], string> = {
  none: "无身份验证", headers_configured: "已配置请求标头（不显示值）",
};
const trustPermission: Record<MCPTrustService["permission"], string> = {
  read: "读取", write: "写入", execute: "执行",
};

export function MCPCatalogSection({
  connected, buttonFeedback, detail, inventory, loadMoreDetail, openDetail, openTool, refresh, reload,
  reloading, reloadMessage, retryDetail, trust,
}: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const wide = useWindowDimensions().width >= 760;
  const action = (label: string, onPress: () => void, { busy = false, disabled = false, text = label } = {}) => (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ busy, disabled }}
      disabled={disabled}
      onPress={onPress}
      style={({ pressed }) => buttonFeedback([styles.button, disabled && styles.disabled], pressed)}
    >
      <ButtonContent color={palette.ink1} loading={busy} text={text} />
    </Pressable>
  );
  const affordance = (
    <View importantForAccessibility="no-hide-descendants" accessibilityElementsHidden style={styles.chevron}>
      <Feather color={palette.ink2} name="chevron-right" size={18} />
    </View>
  );

  if (detail) {
    const { result, status } = detail;
    const tool = result?.tools.find((item) => item.id === detail.selectedToolID);
    if (tool && result) return <ToolDetail sessionEvaluated={result.session_evaluated} tool={tool} />;

    const server = result?.server ?? inventory.servers?.find((item) => item.name === detail.serverName);
    const feedback = status === "loading" && result ? "正在加载更多工具…" : detailFeedback[status];
    return (
      <View style={styles.section}>
        <View style={styles.card}>
          <Text accessibilityRole="header" style={styles.heading}>服务工具目录</Text>
          {server ? <ServiceSummary server={server} /> : <Text style={styles.identifier}>{detail.serverName}</Text>}
          {!result && inventory.status !== "ready" ? <Text style={styles.meta}>显示上次获取的服务摘要。</Text> : null}
        </View>
        {result ? (
          <>
            <Status>{result.session_evaluated
              ? "资格按当前会话策略评估，仅供参考。"
              : "未评估会话资格：未指定会话，或会话与电脑端 Agent 当前会话不匹配。"}</Status>
            {result.tools.length === 0 ? <Status>此服务没有已发现的工具。</Status> : null}
            <View style={[styles.rows, wide && styles.grid]}>
              {result.tools.map((item) => (
                <Pressable
                  key={item.id}
                  accessibilityRole="button"
                  accessibilityHint="打开工具说明、参数及返回结构"
                  onPress={() => openTool(item.id)}
                  style={({ pressed }) => buttonFeedback([styles.card, styles.rowCard, wide && styles.gridItem], pressed)}
                >
                  <View style={[styles.rowBody, styles.stack]}>
                    <Text style={styles.identifier}>{item.name}</Text>
                    {item.title && item.title !== item.name ? <Text style={styles.body}>{item.title}</Text> : null}
                    <ToolEligibility tool={item} />
                  </View>
                  {affordance}
                </Pressable>
              ))}
            </View>
          </>
        ) : null}
        {feedback ? <Status>{feedback}</Status> : null}
        {status === "ready" || status === "loading" ? (
          result?.next_cursor ? action("加载更多工具", loadMoreDetail, { busy: status === "loading", disabled: !connected || status === "loading" }) : null
        ) : status === "unsupported" ? null : action(
          status === "changed" ? "刷新工具目录" : result ? "重试加载更多工具" : "重试工具详情",
          retryDetail, { disabled: !connected },
        )}
      </View>
    );
  }

  const feedback = inventoryStatus(inventory, connected);
  const loading = inventory.status === "loading";
  const trustServices = connected ? trust.services : null;
  const review = trustServices && !reloading ? trust.review : null;
  const reviewing = trust.pendingAction === "review";
  const approving = trust.pendingAction === "approve";
  const listing = trust.pendingAction === "list";
  const trustBusy = !!trust.pendingAction || !!review || reloading;
  return (
    <View style={styles.section}>
      <View style={styles.card}>
        <Text accessibilityRole="header" style={styles.heading}>MCP 运行时</Text>
        <Text style={styles.meta}>只读目录：服务诊断、工具定义与会话资格</Text>
        <View style={styles.actions}>
          {action("刷新 MCP 列表", () => { refresh(); trust.refresh(); }, { busy: loading, disabled: !connected || loading, text: "刷新列表" })}
          {action("重载 MCP", reload, { busy: reloading, disabled: !connected || reloading, text: reloading ? "重载中" : "重载 MCP" })}
        </View>
      </View>
      {reloadMessage ? <Status>{`${reloadMessage}${inventory.servers !== null && reloadMessage.startsWith("MCP 重载失败") ? " 现有列表仍可查看。" : ""}`}</Status> : null}
      {feedback ? <Status>{feedback}</Status> : null}
      <View style={[styles.rows, wide && styles.grid]}>
        {inventory.servers?.map((server) => connected ? (
          <Pressable
            key={server.name}
            accessibilityRole="button"
            accessibilityHint="打开服务工具目录"
            onPress={() => openDetail(server.name)}
            style={({ pressed }) => buttonFeedback([styles.card, styles.rowCard, wide && styles.gridItem], pressed)}
          >
            <View style={styles.rowBody}><ServiceSummary server={server} /></View>
            {affordance}
          </Pressable>
        ) : (
          <View key={server.name} style={[styles.card, wide && styles.gridItem]}><ServiceSummary server={server} /></View>
        ))}
      </View>
      {trust.unclear ? <Status>{mcpTrustOutcomeUnclear}</Status> : null}
      {trustServices ? (
        <View style={styles.card}>
          <Text accessibilityRole="header" style={styles.heading}>HTTP MCP 信任审批</Text>
          <Text style={styles.meta}>仅限电脑端已配置且允许远程审批的服务。审批只保存信任，不会连接服务或重载 MCP。</Text>
          {action("刷新 MCP 信任状态", trust.refresh, { busy: listing, disabled: trustBusy, text: "刷新信任状态" })}
          {approving ? <Status>正在保存信任审批…</Status> : null}
          {trust.notice ? <Status>{trust.notice}</Status> : null}
          {trustServices.length === 0 ? <Status>没有可在手机端审批的 HTTP MCP 服务。</Status> : null}
          {trustServices.map((service) => (
            <View key={service.name} style={styles.field}>
              <Text style={styles.identifier}>{service.name}</Text>
              <Text selectable style={styles.meta}>{service.endpoint}</Text>
              <Text style={styles.meta}>{`权限：${trustPermission[service.permission]} · ${trustAuthentication[service.authentication]}`}</Text>
              <Text style={styles.state}>{`${service.trusted ? "已信任" : "未信任"} · 运行时：${trustRuntime[service.runtime]}`}</Text>
              {!service.trusted ? action(`查看并审批：${service.name}`, () => trust.requestReview(service.name), {
                disabled: trustBusy, text: reviewing ? "正在查看…" : "查看并审批",
              }) : null}
            </View>
          ))}
        </View>
      ) : null}
      {review ? (
        <ConfirmModal
          confirmLabel="信任此配置"
          danger
          message={`服务：${review.service.name}\n端点：${review.service.endpoint}\n权限：${trustPermission[review.service.permission]}\n认证：${trustAuthentication[review.service.authentication]}\n运行时：${trustRuntime[review.service.runtime]}\n\n仅在你确认信任此服务及其配置时继续。审批不会连接服务或重载 MCP。`}
          onCancel={trust.cancelReview}
          onConfirm={() => trust.confirm(review.id)}
          title="确认信任 HTTP MCP 服务"
          visible
        />
      ) : null}
    </View>
  );
}

const makeStyles = ({ palette }: AppTheme) => StyleSheet.create({
  section: { gap: 12 },
  rows: { gap: 8 },
  grid: { flexDirection: "row", flexWrap: "wrap" },
  gridItem: { flexBasis: "48%", flexGrow: 1 },
  card: { padding: 14, gap: 8, minWidth: 0, borderWidth: 1, borderColor: palette.hairline, borderRadius: radius.lg, backgroundColor: palette.surface1 },
  rowCard: { minHeight: 48, flexDirection: "row", alignItems: "center" },
  rowBody: { flex: 1, minWidth: 0 },
  stack: { gap: 4 },
  chevron: { flexShrink: 0 },
  actions: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  button: { alignSelf: "flex-start", maxWidth: "100%", minHeight: 44, paddingVertical: 9, paddingHorizontal: 12, justifyContent: "center", borderRadius: radius.md, borderWidth: 1, borderColor: palette.hairlineStrong, backgroundColor: palette.surface2 },
  disabled: { opacity: 0.5 },
  heading: { color: palette.ink1, fontSize: 17, fontWeight: weight.semibold },
  identifier: { color: palette.ink1, fontSize: 14, fontFamily: fontFamily.mono },
  body: { color: palette.ink1, fontSize: 14, lineHeight: 21 },
  state: { color: palette.brandLight, fontSize: 13, fontWeight: weight.semibold },
  meta: { color: palette.ink2, fontSize: 13, lineHeight: 19 },
  status: { color: palette.ink2, fontSize: 13, lineHeight: 19, paddingHorizontal: 2 },
  field: { paddingTop: 8, gap: 3, borderTopWidth: 1, borderColor: palette.hairline },
});
