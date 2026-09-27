package onebot

import (
	"fmt"
	"strings"
	"time"
)

// DefaultWSURL 定义默认的 NapCatQQ 正向 WebSocket 服务地址。
const DefaultWSURL = "ws://127.0.0.1:6700"

// DefaultCooldown 定义普通用户在群聊或私聊中的默认冷却时长。
const DefaultCooldown = 3 * time.Second

// Config 定义 OneBot (NapCatQQ) 机器人接入层的完整运行配置。
// 1.1 WSURL：NapCatQQ 正向 WebSocket 地址（例如 ws://127.0.0.1:6700）；
// 1.2 Token：NapCatQQ WebSocket 鉴权 Token，连接时通过 Authorization Header 传递；
// 1.3 SuperAdmins：启动时种子化写入数据库的超级管理员 QQ 号列表；
// 1.4 GroupAtOnly：群聊是否仅在 @机器人 时才触发回复（默认 true，防止群内刷屏）；
// 1.5 GroupSessionPerUser：群聊是否按每个群成员独立分配 Session（默认 false 即全群共享）；
// 1.6 Cooldown：普通用户（role == user）连续触发对话的最小冷却时间间隔；
// 1.7 Workspace：机器人绑定的本地工作区目录。
type Config struct {
	WSURL               string
	Token               string
	SuperAdmins         []int64
	GroupAtOnly         bool
	GroupSessionPerUser bool
	Cooldown            time.Duration
	Workspace           string
}

// Normalize 规范化并校验启动配置项，为缺省字段填充安全的默认值。
// 1.1 清理 WSURL 首尾空白字符，若为空则回退至默认地址 ws://127.0.0.1:6700；
// 1.2 校验 WSURL 协议头，确保以 ws:// 或 wss:// 开头；
// 1.3 清理 Token 首尾空白字符；
// 1.4 过滤并去重 SuperAdmins 列表中的无效 QQ 号（<= 0）；
// 1.5 若 Cooldown 未设置或为负数，则使用默认的 3 秒冷却时间。
func (c Config) Normalize() (Config, error) {
	// 1.1 清理并补全默认 WebSocket URL
	c.WSURL = strings.TrimSpace(c.WSURL)
	if c.WSURL == "" {
		c.WSURL = DefaultWSURL
	}

	// 1.2 校验 WebSocket 协议前缀合法性
	lowerURL := strings.ToLower(c.WSURL)
	if !strings.HasPrefix(lowerURL, "ws://") && !strings.HasPrefix(lowerURL, "wss://") {
		return Config{}, fmt.Errorf("invalid onebot websocket url %q: must start with ws:// or wss://", c.WSURL)
	}

	// 1.3 清理鉴权 Token
	c.Token = strings.TrimSpace(c.Token)

	// 1.4 过滤并去重超级管理员 QQ 号列表
	if len(c.SuperAdmins) > 0 {
		seen := make(map[int64]struct{}, len(c.SuperAdmins))
		cleaned := make([]int64, 0, len(c.SuperAdmins))
		for _, qq := range c.SuperAdmins {
			if qq <= 0 {
				continue
			}
			if _, ok := seen[qq]; ok {
				continue
			}
			seen[qq] = struct{}{}
			cleaned = append(cleaned, qq)
		}
		c.SuperAdmins = cleaned
	}

	// 1.5 规范化冷却时长
	if c.Cooldown < 0 {
		c.Cooldown = DefaultCooldown
	}

	return c, nil
}
