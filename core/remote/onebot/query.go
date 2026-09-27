package onebot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	tooldef "myai/core/tool/tool"
)

// MaxQueryResultRows 定义 `onebot_query` 单次查询最多返回的记录行数，防止批量导出数据。
const MaxQueryResultRows = 100

// forbiddenSQLKeywords 定义严格禁止出现在只读查询中的 SQL 关键字清单。
var forbiddenSQLKeywords = []string{
	"DROP", "DELETE", "UPDATE", "INSERT", "REPLACE",
	"ALTER", "CREATE", "TRUNCATE", "ATTACH", "DETACH",
	"PRAGMA", "VACUUM", "REINDEX", "LOAD_EXTENSION",
	"SQLITE_MASTER", "SQLITE_SCHEMA", "SQLITE_TEMP_MASTER",
}

// tableRefPattern 用于提取 SQL 语句中 FROM 与 JOIN 子句后引用的表名标识符。
var tableRefPattern = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)

// wordTokenPattern 用于切分 SQL 语句中的标识符与关键字词元。
var wordTokenPattern = regexp.MustCompile(`(?i)[a-z_][a-z0-9_]*`)

// QueryTool 实现 `6.9 onebot_query` 工具，支持大模型通过生成只读 SQL SELECT 语句灵活查询机器人内部数据。
type QueryTool struct {
	store Store
}

type queryToolArgs struct {
	SQL     string `json:"sql"`
	Explain string `json:"explain"`
}

// NewQueryTool 创建 `onebot_query` 工具实例。
func NewQueryTool(store Store) *QueryTool {
	return &QueryTool{store: store}
}

func (t *QueryTool) Name() string {
	return "onebot_query"
}

func (t *QueryTool) Description() string {
	return `对机器人内部数据执行只读 SQL SELECT 查询（需要 admin 或 super_admin 权限）。
可查询的表结构：
1. onebot_users (user_id BIGINT PRIMARY KEY, nickname TEXT, role TEXT, granted_by BIGINT, last_active_at DATETIME, created_at DATETIME, updated_at DATETIME)
2. onebot_groups (group_id BIGINT PRIMARY KEY, group_name TEXT, enabled BOOLEAN, persona TEXT, updated_by BIGINT, updated_at DATETIME)
限制：只允许 SELECT 语句；禁止访问会话隐私内容；最多返回 100 条记录。`
}

func (t *QueryTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"sql": map[string]any{
				"type":        "string",
				"description": "针对 onebot_users 或 onebot_groups 表的只读 SQL SELECT 查询语句",
			},
			"explain": map[string]any{
				"type":        "string",
				"description": "用一句话说明本次查询的业务意图（用于安全审计日志）",
			},
		},
		"required": []string{"sql", "explain"},
	}
}

func (t *QueryTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

// ValidateOneBotSQL 对大模型生成的 SQL 语句执行严格的白名单安全校验。
// 1.1 清理首尾空白字符与末尾单个可选分号，若语句为空则直接拒绝；
// 1.2 拦截多语句拼接符 `;` 及 SQL 注释符 `--`、`/*`、`*/`；
// 1.3 校验语句必须以 `SELECT` 关键字开头；
// 1.4 扫描全部词元（Word Tokens），拦截任何写操作、DDL、系统表或危险关键字；
// 1.5 提取所有 `FROM` / `JOIN` 引用的表名，强制要求至少引用一张表且所有表名均在白名单 `{onebot_users, onebot_groups}` 内。
func ValidateOneBotSQL(rawSQL string) (string, error) {
	// 1.1 清理首尾空白及末尾单个分号
	cleaned := strings.TrimSpace(rawSQL)
	cleaned = strings.TrimSuffix(cleaned, ";")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return "", errors.New("SQL 查询语句不能为空")
	}

	// 1.2 拦截多语句分号与注释符号
	if strings.Contains(cleaned, ";") {
		return "", errors.New("禁止执行多条 SQL 语句（不允许包含分号）")
	}
	if strings.Contains(cleaned, "--") || strings.Contains(cleaned, "/*") || strings.Contains(cleaned, "*/") {
		return "", errors.New("禁止在 SQL 中包含注释符号")
	}

	// 1.3 抹平字符串字面量后再提取词元，避免字符串内部内容干扰关键字与表名提取
	masked := maskSQLStringLiterals(cleaned)
	upperMasked := strings.ToUpper(strings.TrimSpace(masked))
	if !strings.HasPrefix(upperMasked, "SELECT ") && !strings.HasPrefix(upperMasked, "SELECT\t") && !strings.HasPrefix(upperMasked, "SELECT\n") {
		return "", errors.New("只允许执行 SELECT 只读查询")
	}

	// 1.4 校验是否包含危险关键字
	tokens := wordTokenPattern.FindAllString(upperMasked, -1)
	forbiddenSet := make(map[string]struct{}, len(forbiddenSQLKeywords))
	for _, kw := range forbiddenSQLKeywords {
		forbiddenSet[kw] = struct{}{}
	}
	for _, tok := range tokens {
		if _, banned := forbiddenSet[tok]; banned {
			return "", fmt.Errorf("SQL 包含禁止使用的关键字: %s", tok)
		}
	}

	// 1.5 校验 FROM / JOIN 引用的表名是否全部位于白名单内
	matches := tableRefPattern.FindAllStringSubmatch(masked, -1)
	if len(matches) == 0 {
		return "", errors.New("SQL 必须通过 FROM 指定查询表（仅支持 onebot_users 或 onebot_groups）")
	}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		tableName := strings.ToLower(strings.TrimSpace(match[1]))
		if tableName != UsersCollection && tableName != GroupsCollection {
			return "", fmt.Errorf("禁止访问非白名单数据表: %s（仅允许 onebot_users 和 onebot_groups）", match[1])
		}
	}

	return cleaned, nil
}

// maskSQLStringLiterals 将单引号字符串字面量内容替换为空格，防止字符串内的词元误触发语法扫描。
func maskSQLStringLiterals(sqlText string) string {
	var b strings.Builder
	b.Grow(len(sqlText))
	inSingleQuote := false
	for i := 0; i < len(sqlText); i++ {
		ch := sqlText[i]
		if ch == '\'' {
			if inSingleQuote && i+1 < len(sqlText) && sqlText[i+1] == '\'' {
				b.WriteString("  ")
				i++
				continue
			}
			inSingleQuote = !inSingleQuote
			b.WriteByte('\'')
			continue
		}
		if inSingleQuote {
			b.WriteByte(' ')
		} else {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// Call 执行只读 SQL 查询并返回 JSON 结果数组。
// 2.1 实时查表校验调用者是否为 admin 或 super_admin；
// 2.2 解析参数并调用 ValidateOneBotSQL 进行安全白名单校验；
// 2.3 从 Store（MongoDB 或内存）加载最新的 `onebot_users` 与 `onebot_groups` 快照，写入一次性隔离内存 SQLite 沙箱；
// 2.4 在内存 SQLite 沙箱中执行 SELECT 查询，强制截断至最多 100 条记录并返回。
func (t *QueryTool) Call(ctx context.Context, rawArgs json.RawMessage) (tooldef.ToolOutput, error) {
	// 2.1 校验管理员权限
	if _, _, err := authorizeCaller(ctx, t.store, RoleAdmin); err != nil {
		return permissionDeniedOutput(err), nil
	}

	// 2.2 解析并校验 SQL 语句
	var args queryToolArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return invalidInputOutput("解析参数失败: " + err.Error()), nil
	}
	cleanedSQL, err := ValidateOneBotSQL(args.SQL)
	if err != nil {
		return invalidInputOutput(err.Error()), nil
	}

	// 2.3 在隔离的内存 SQLite 沙箱中加载快照并执行查询
	rows, truncated, err := executeSandboxedSQL(ctx, t.store, cleanedSQL)
	if err != nil {
		return executionFailedOutput("query_execution_failed", err), nil
	}

	// 2.4 返回查询结果与元信息
	return jsonSuccessOutput(map[string]any{
		"explain":   strings.TrimSpace(args.Explain),
		"sql":       cleanedSQL,
		"row_count": len(rows),
		"truncated": truncated,
		"rows":      rows,
	})
}

// executeSandboxedSQL 将 Store 中的用户与群配置装载至一次性 `:memory:` SQLite 沙箱中执行标准 SQL 查询。
// 3.1 开启独立的 `:memory:` SQLite 实例（与业务库物理隔离，且天然不包含 private_session_id 等敏感字段）；
// 3.2 创建 `onebot_users` 与 `onebot_groups` 表结构；
// 3.3 从 Store 拉取最新用户与群数据并写入内存表；
// 3.4 执行经白名单校验的 SELECT 语句，动态读取列名与行值，超过 MaxQueryResultRows (100) 时自动截断。
func executeSandboxedSQL(ctx context.Context, store Store, selectSQL string) ([]map[string]any, bool, error) {
	// 3.1 开启一次性内存 SQLite 数据库
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, false, fmt.Errorf("初始化查询沙箱失败: %w", err)
	}
	defer db.Close()

	// 3.2 创建白名单表结构（故意排除 session_id 等内部字段以确保隐私安全）
	schemaSQL := `
CREATE TABLE onebot_users (
	user_id        INTEGER PRIMARY KEY,
	nickname       TEXT NOT NULL DEFAULT '',
	role           TEXT NOT NULL DEFAULT 'user',
	granted_by     INTEGER NOT NULL DEFAULT 0,
	last_active_at DATETIME,
	created_at     DATETIME,
	updated_at     DATETIME
);
CREATE TABLE onebot_groups (
	group_id   INTEGER PRIMARY KEY,
	group_name TEXT NOT NULL DEFAULT '',
	enabled    BOOLEAN NOT NULL DEFAULT 1,
	persona    TEXT NOT NULL DEFAULT '',
	updated_by INTEGER NOT NULL DEFAULT 0,
	updated_at DATETIME
);`
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return nil, false, fmt.Errorf("创建沙箱表结构失败: %w", err)
	}

	// 3.3 装载最新数据快照
	users, err := store.ListUsers(ctx, "all")
	if err != nil {
		return nil, false, fmt.Errorf("读取用户数据失败: %w", err)
	}
	for _, u := range users {
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO onebot_users (user_id, nickname, role, granted_by, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			u.UserID,
			u.Nickname,
			string(u.Role),
			u.GrantedBy,
			formatSQLiteTime(u.LastActiveAt),
			formatSQLiteTime(u.CreatedAt),
			formatSQLiteTime(u.UpdatedAt),
		); err != nil {
			return nil, false, fmt.Errorf("装载用户数据失败: %w", err)
		}
	}

	groups, err := store.ListGroups(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("读取群配置失败: %w", err)
	}
	for _, g := range groups {
		enabledInt := 0
		if g.Enabled {
			enabledInt = 1
		}
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO onebot_groups (group_id, group_name, enabled, persona, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			g.GroupID,
			g.GroupName,
			enabledInt,
			g.Persona,
			g.UpdatedBy,
			formatSQLiteTime(g.UpdatedAt),
		); err != nil {
			return nil, false, fmt.Errorf("装载群数据失败: %w", err)
		}
	}

	// 3.4 执行只读查询并扫描结果集
	queryRows, err := db.QueryContext(ctx, selectSQL)
	if err != nil {
		return nil, false, fmt.Errorf("执行 SQL 查询失败: %w", err)
	}
	defer queryRows.Close()

	columns, err := queryRows.Columns()
	if err != nil {
		return nil, false, err
	}

	result := make([]map[string]any, 0)
	truncated := false
	for queryRows.Next() {
		if len(result) >= MaxQueryResultRows {
			truncated = true
			break
		}
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := queryRows.Scan(valuePtrs...); err != nil {
			return nil, false, err
		}
		rowMap := make(map[string]any, len(columns))
		for i, col := range columns {
			switch v := values[i].(type) {
			case []byte:
				rowMap[col] = string(v)
			default:
				rowMap[col] = v
			}
		}
		result = append(result, rowMap)
	}
	if err := queryRows.Err(); err != nil {
		return nil, false, err
	}
	return result, truncated, nil
}

// formatSQLiteTime 将 Go time.Time 格式化为 SQLite datetime 函数可直接比较的 UTC 字符串格式。
func formatSQLiteTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}
