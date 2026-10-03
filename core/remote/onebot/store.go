package onebot

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// UsersCollection 定义 MongoDB 中存储 QQ 用户身份与私聊会话映射的集合名。
	UsersCollection = "onebot_users"
	// GroupsCollection 定义 MongoDB 中存储 QQ 群聊开关、人设与群共享会话映射的集合名。
	GroupsCollection = "onebot_groups"
)

// UserIdentity 对应数据库中的 `onebot_users` 表，记录每个与机器人交互过的 QQ 用户身份与会话映射。
// 1.1 UserID：QQ 号，同时作为 MongoDB 文档主键 `_id`；
// 1.2 Nickname：最近一次从 NapCatQQ 事件中获取的 QQ 昵称或群名片；
// 1.3 Role：用户角色（super_admin / admin / user / banned）；
// 1.4 GrantedBy：是谁授予了该角色（0 表示由启动参数 --super-admin 初始化或默认注册）；
// 1.5 PrivateSessionID：该用户在私聊模式下绑定的 MyAI SessionID；
// 1.6 LastActiveAt / CreatedAt / UpdatedAt：活跃时间与审计时间戳。
type UserIdentity struct {
	UserID           int64     `bson:"_id" json:"user_id"`
	Nickname         string    `bson:"nickname" json:"nickname"`
	Role             Role      `bson:"role" json:"role"`
	GrantedBy        int64     `bson:"granted_by" json:"granted_by"`
	PrivateSessionID string    `bson:"private_session_id" json:"private_session_id"`
	Persona          string    `bson:"persona,omitempty" json:"persona,omitempty"`
	LastActiveAt     time.Time `bson:"last_active_at" json:"last_active_at"`
	CreatedAt        time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt        time.Time `bson:"updated_at" json:"updated_at"`
}

// GroupConfig 对应数据库中的 `onebot_groups` 表，记录机器人所在的 QQ 群配置、开关与人设。
// 2.1 GroupID：QQ 群号，同时作为 MongoDB 文档主键 `_id`；
// 2.2 GroupName：群名称；
// 2.3 Enabled：是否在该群开启机器人回复服务（默认 true）；
// 2.4 SessionID：该群在共享会话模式下绑定的 MyAI SessionID；
// 2.5 Persona：该群专属的说话风格/人设指令（对应 Session.StyleInstruction）；
// 2.6 UpdatedBy / UpdatedAt：最后修改该群配置的管理员 QQ 号与更新时间。
type GroupConfig struct {
	GroupID   int64     `bson:"_id" json:"group_id"`
	GroupName string    `bson:"group_name" json:"group_name"`
	Enabled   bool      `bson:"enabled" json:"enabled"`
	SessionID string    `bson:"session_id" json:"session_id"`
	Persona   string    `bson:"persona" json:"persona"`
	UpdatedBy int64     `bson:"updated_by" json:"updated_by"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// Store 定义 OneBot 身份鉴权与群聊配置的持久化契约（相当于 Spring Data 的 Repository 接口）。
// 3.1 GetUser / UpsertUser / ListUsers：管理 `onebot_users` 表；
// 3.2 GetGroup / UpsertGroup / ListGroups：管理 `onebot_groups` 表；
// 3.3 SeedSuperAdmins：启动时将命令行指定的超级管理员 QQ 号幂等写入数据库。
type Store interface {
	GetUser(ctx context.Context, userID int64) (UserIdentity, bool, error)
	UpsertUser(ctx context.Context, user UserIdentity) error
	ListUsers(ctx context.Context, roleFilter string) ([]UserIdentity, error)

	GetGroup(ctx context.Context, groupID int64) (GroupConfig, bool, error)
	UpsertGroup(ctx context.Context, group GroupConfig) error
	ListGroups(ctx context.Context) ([]GroupConfig, error)

	SeedSuperAdmins(ctx context.Context, superAdmins []int64) error
}

// NewStore 根据传入的 MongoDB 数据库句柄自动选择持久化实现。
// 4.1 若 db 为 nil（未配置 MongoDB），则回退至线程安全的内存存储 MemoryStore；
// 4.2 若 db 不为 nil，则创建 MongoStore 并在启动时初始化集合索引。
func NewStore(ctx context.Context, db *gomongo.Database) (Store, error) {
	if db == nil {
		return NewMemoryStore(), nil
	}
	store := NewMongoStore(db)
	if err := store.EnsureIndexes(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

// MemoryStore 提供基于读写锁 sync.RWMutex 的内存存储实现，用于无 MongoDB 环境或单元测试。
type MemoryStore struct {
	mu          sync.RWMutex
	users       map[int64]UserIdentity
	groups      map[int64]GroupConfig
	groupUserSS map[string]string // key: "groupID:userID" -> sessionID（用于群内独立会话模式）
}

// NewMemoryStore 创建并初始化一个空的内存存储实例。
// 1.1 初始化 users、groups 及群成员独立会话映射表。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:       make(map[int64]UserIdentity),
		groups:      make(map[int64]GroupConfig),
		groupUserSS: make(map[string]string),
	}
}

// GetUser 从内存中按 QQ 号查询用户身份记录。
// 1.1 加读锁读取 map；
// 1.2 若不存在则返回零值与 false。
func (m *MemoryStore) GetUser(_ context.Context, userID int64) (UserIdentity, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.users[userID]
	return u, ok, nil
}

// UpsertUser 新增或更新内存中的用户身份记录。
// 1.1 校验 userID 合法性（必须 > 0）；
// 1.2 规范化角色字段并补全 CreatedAt / UpdatedAt / LastActiveAt 时间戳；
// 1.3 加写锁写入 map。
func (m *MemoryStore) UpsertUser(_ context.Context, user UserIdentity) error {
	if user.UserID <= 0 {
		return errors.New("user_id must be positive")
	}
	now := time.Now().UTC()
	user.Role = NormalizeRole(string(user.Role))
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}
	if user.LastActiveAt.IsZero() {
		user.LastActiveAt = now
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.users[user.UserID]; ok && !existing.CreatedAt.IsZero() && user.CreatedAt.IsZero() {
		user.CreatedAt = existing.CreatedAt
	} else if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	m.users[user.UserID] = user
	return nil
}

// ListUsers 按角色过滤条件列出内存中的所有用户记录，并按最后活跃时间倒序排列。
// 1.1 解析 roleFilter（为空或 "all" 时不过滤）；
// 1.2 遍历收集符合条件的用户切片；
// 1.3 按 LastActiveAt 倒序、UserID 升序稳定排序后返回。
func (m *MemoryStore) ListUsers(_ context.Context, roleFilter string) ([]UserIdentity, error) {
	filter := strings.ToLower(strings.TrimSpace(roleFilter))

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]UserIdentity, 0, len(m.users))
	for _, u := range m.users {
		if filter != "" && filter != "all" && string(u.Role) != filter {
			continue
		}
		result = append(result, u)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].LastActiveAt.Equal(result[j].LastActiveAt) {
			return result[i].LastActiveAt.After(result[j].LastActiveAt)
		}
		return result[i].UserID < result[j].UserID
	})
	return result, nil
}

// GetGroup 从内存中按群号查询群配置。
// 1.1 加读锁查询 groups map。
func (m *MemoryStore) GetGroup(_ context.Context, groupID int64) (GroupConfig, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	g, ok := m.groups[groupID]
	return g, ok, nil
}

// UpsertGroup 新增或更新内存中的群配置。
// 1.1 校验 groupID 合法性（必须 > 0）；
// 1.2 补全 UpdatedAt 时间戳并写入 map。
func (m *MemoryStore) UpsertGroup(_ context.Context, group GroupConfig) error {
	if group.GroupID <= 0 {
		return errors.New("group_id must be positive")
	}
	if group.UpdatedAt.IsZero() {
		group.UpdatedAt = time.Now().UTC()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.groups[group.GroupID] = group
	return nil
}

// ListGroups 列出内存中的所有群配置，并按群号升序排列。
// 1.1 加读锁复制切片；
// 1.2 按 GroupID 升序排序返回。
func (m *MemoryStore) ListGroups(_ context.Context) ([]GroupConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]GroupConfig, 0, len(m.groups))
	for _, g := range m.groups {
		result = append(result, g)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].GroupID < result[j].GroupID
	})
	return result, nil
}

// SeedSuperAdmins 将启动参数指定的超级管理员列表幂等写入内存存储。
// 1.1 遍历 superAdmins 列表；
// 1.2 若用户已存在，则将其 Role 强制提升为 RoleSuperAdmin 并保留原昵称与 PrivateSessionID；
// 1.3 若用户不存在，则创建新的 RoleSuperAdmin 记录（GrantedBy = 0）。
func (m *MemoryStore) SeedSuperAdmins(ctx context.Context, superAdmins []int64) error {
	now := time.Now().UTC()
	for _, qq := range superAdmins {
		if qq <= 0 {
			continue
		}
		existing, ok, err := m.GetUser(ctx, qq)
		if err != nil {
			return err
		}
		if ok {
			existing.Role = RoleSuperAdmin
			existing.GrantedBy = 0
			existing.UpdatedAt = now
			if err := m.UpsertUser(ctx, existing); err != nil {
				return err
			}
			continue
		}
		if err := m.UpsertUser(ctx, UserIdentity{
			UserID:       qq,
			Role:         RoleSuperAdmin,
			GrantedBy:    0,
			LastActiveAt: now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}); err != nil {
			return err
		}
	}
	return nil
}

// MongoStore 提供基于 MongoDB 的 `onebot_users` 与 `onebot_groups` 持久化实现。
type MongoStore struct {
	db *gomongo.Database
}

// NewMongoStore 创建绑定到指定 MongoDB 数据库的存储实例。
// 1.1 保存 *gomongo.Database 引用。
func NewMongoStore(db *gomongo.Database) *MongoStore {
	return &MongoStore{db: db}
}

// EnsureIndexes 在 MongoDB 中为 `onebot_users` 与 `onebot_groups` 创建常用查询索引。
// 1.1 为 `onebot_users` 创建 `role` 与 `last_active_at` 索引，加速按角色过滤与活跃度统计；
// 1.2 为 `onebot_groups` 创建 `enabled` 索引。
func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	userIndexes := []gomongo.IndexModel{
		{Keys: bson.D{{Key: "role", Value: 1}}},
		{Keys: bson.D{{Key: "last_active_at", Value: -1}}},
	}
	if _, err := s.db.Collection(UsersCollection).Indexes().CreateMany(ctx, userIndexes); err != nil {
		return err
	}

	groupIndexes := []gomongo.IndexModel{
		{Keys: bson.D{{Key: "enabled", Value: 1}}},
	}
	if _, err := s.db.Collection(GroupsCollection).Indexes().CreateMany(ctx, groupIndexes); err != nil {
		return err
	}
	return nil
}

// GetUser 从 MongoDB `onebot_users` 集合按 QQ 号查询用户身份记录。
// 1.1 以 `_id: userID` 执行 FindOne；
// 1.2 若命中 gomongo.ErrNoDocuments 则返回 (零值, false, nil)。
func (s *MongoStore) GetUser(ctx context.Context, userID int64) (UserIdentity, bool, error) {
	if s == nil || s.db == nil {
		return UserIdentity{}, false, errors.New("mongo database is nil")
	}
	var user UserIdentity
	err := s.db.Collection(UsersCollection).FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if errors.Is(err, gomongo.ErrNoDocuments) {
			return UserIdentity{}, false, nil
		}
		return UserIdentity{}, false, err
	}
	user.Role = NormalizeRole(string(user.Role))
	return user, true, nil
}

// UpsertUser 新增或更新 MongoDB `onebot_users` 集合中的用户记录。
// 1.1 校验 userID > 0 并补全默认时间戳；
// 1.2 使用 `$set` 更新可变字段，使用 `$setOnInsert` 写入首次创建时的 `created_at`；
// 1.3 开启 `SetUpsert(true)` 执行原子 Upsert。
func (s *MongoStore) UpsertUser(ctx context.Context, user UserIdentity) error {
	if s == nil || s.db == nil {
		return errors.New("mongo database is nil")
	}
	if user.UserID <= 0 {
		return errors.New("user_id must be positive")
	}
	now := time.Now().UTC()
	user.Role = NormalizeRole(string(user.Role))
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}
	if user.LastActiveAt.IsZero() {
		user.LastActiveAt = now
	}
	createdAt := user.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	update := bson.M{
		"$set": bson.M{
			"nickname":           user.Nickname,
			"role":               string(user.Role),
			"granted_by":         user.GrantedBy,
			"private_session_id": user.PrivateSessionID,
			"persona":            user.Persona,
			"last_active_at":     user.LastActiveAt,
			"updated_at":         user.UpdatedAt,
		},
		"$setOnInsert": bson.M{
			"_id":        user.UserID,
			"created_at": createdAt,
		},
	}
	_, err := s.db.Collection(UsersCollection).UpdateOne(
		ctx,
		bson.M{"_id": user.UserID},
		update,
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// ListUsers 从 MongoDB `onebot_users` 集合按角色过滤并按最后活跃时间倒序查询用户列表。
// 1.1 构造过滤条件 filter（当 roleFilter 非空且非 "all" 时按 role 精确匹配）；
// 1.2 设置按 `last_active_at` 降序排序；
// 1.3 解码全部结果并规范化 Role 字段后返回。
func (s *MongoStore) ListUsers(ctx context.Context, roleFilter string) ([]UserIdentity, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("mongo database is nil")
	}
	filter := bson.M{}
	rf := strings.ToLower(strings.TrimSpace(roleFilter))
	if rf != "" && rf != "all" {
		filter["role"] = rf
	}
	cursor, err := s.db.Collection(UsersCollection).Find(
		ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "last_active_at", Value: -1}, {Key: "_id", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []UserIdentity
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}
	for i := range users {
		users[i].Role = NormalizeRole(string(users[i].Role))
	}
	return users, nil
}

// GetGroup 从 MongoDB `onebot_groups` 集合按群号查询群配置。
// 1.1 以 `_id: groupID` 执行 FindOne；
// 1.2 若命中 gomongo.ErrNoDocuments 则返回 (零值, false, nil)。
func (s *MongoStore) GetGroup(ctx context.Context, groupID int64) (GroupConfig, bool, error) {
	if s == nil || s.db == nil {
		return GroupConfig{}, false, errors.New("mongo database is nil")
	}
	var group GroupConfig
	err := s.db.Collection(GroupsCollection).FindOne(ctx, bson.M{"_id": groupID}).Decode(&group)
	if err != nil {
		if errors.Is(err, gomongo.ErrNoDocuments) {
			return GroupConfig{}, false, nil
		}
		return GroupConfig{}, false, err
	}
	return group, true, nil
}

// UpsertGroup 新增或更新 MongoDB `onebot_groups` 集合中的群配置。
// 1.1 校验 groupID > 0 并补全 UpdatedAt 时间戳；
// 1.2 使用 `$set` 与 `$setOnInsert` 执行原子 Upsert。
func (s *MongoStore) UpsertGroup(ctx context.Context, group GroupConfig) error {
	if s == nil || s.db == nil {
		return errors.New("mongo database is nil")
	}
	if group.GroupID <= 0 {
		return errors.New("group_id must be positive")
	}
	if group.UpdatedAt.IsZero() {
		group.UpdatedAt = time.Now().UTC()
	}

	update := bson.M{
		"$set": bson.M{
			"group_name": group.GroupName,
			"enabled":    group.Enabled,
			"session_id": group.SessionID,
			"persona":    group.Persona,
			"updated_by": group.UpdatedBy,
			"updated_at": group.UpdatedAt,
		},
		"$setOnInsert": bson.M{
			"_id": group.GroupID,
		},
	}
	_, err := s.db.Collection(GroupsCollection).UpdateOne(
		ctx,
		bson.M{"_id": group.GroupID},
		update,
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// ListGroups 列出 MongoDB `onebot_groups` 集合中的所有群配置并按群号升序排序。
// 1.1 执行 Find 查询并按 `_id` 升序排列；
// 1.2 解码全部群记录后返回。
func (s *MongoStore) ListGroups(ctx context.Context) ([]GroupConfig, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("mongo database is nil")
	}
	cursor, err := s.db.Collection(GroupsCollection).Find(
		ctx,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var groups []GroupConfig
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// SeedSuperAdmins 将启动参数指定的超级管理员列表幂等写入 MongoDB `onebot_users` 集合。
// 1.1 遍历 superAdmins 列表；
// 1.2 对每个 QQ 号执行原子 UpdateOne（设置 role = "super_admin", granted_by = 0），不覆盖已有的 nickname 与 private_session_id。
func (s *MongoStore) SeedSuperAdmins(ctx context.Context, superAdmins []int64) error {
	if s == nil || s.db == nil {
		return errors.New("mongo database is nil")
	}
	now := time.Now().UTC()
	for _, qq := range superAdmins {
		if qq <= 0 {
			continue
		}
		update := bson.M{
			"$set": bson.M{
				"role":       string(RoleSuperAdmin),
				"granted_by": int64(0),
				"updated_at": now,
			},
			"$setOnInsert": bson.M{
				"_id":                qq,
				"nickname":           "",
				"private_session_id": "",
				"last_active_at":     now,
				"created_at":         now,
			},
		}
		if _, err := s.db.Collection(UsersCollection).UpdateOne(
			ctx,
			bson.M{"_id": qq},
			update,
			options.UpdateOne().SetUpsert(true),
		); err != nil {
			return err
		}
	}
	return nil
}
