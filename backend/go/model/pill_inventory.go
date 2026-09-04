// 丹方与消耗性金丹库存模型（金丹消耗品重构）
// 三个对象必须区分：
//   - PillRecipe 丹方：可复用、可导出的能力配方，有明确版本，永久保留
//   - PillItem 金丹实例：按某一版本丹方炼出的实体库存，每枚独立 ID，服用/融合后退出可用库存
//   - AgentPillEffect 已吸收能力：道人服用时获得的能力快照，不依赖原金丹还在库存
//
// 旧 ElixirPill / AgentPill 暂时保留用于迁移（见 dao/pill_inventory_migration.go）
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------- 丹方 ----------

// PillRecipe 丹方，对应 pill_recipes 表
// 一份可复用能力配方；归档后停止新炼制，既有金丹仍可服用/融合，历史仍可查看；禁止硬删除
type PillRecipe struct {
	ID                uint       `json:"id" gorm:"primaryKey;autoIncrement;comment:丹方唯一标识"`
	UUID              uuid.UUID  `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	CurrentRevisionID *string    `json:"-" gorm:"type:uuid;comment:当前版本UUID文本(创建事务内先空后回填,提交前不得为空)"`
	IsBuiltin         bool       `json:"is_builtin" gorm:"default:false;index;comment:是否系统内置丹方"`
	ArchivedAt        *time.Time `json:"archived_at" gorm:"comment:归档时间;空=未归档"`
	CreatedAt         time.Time  `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt         time.Time  `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (PillRecipe) TableName() string {
	return "pill_recipes"
}

// BeforeCreate 默认对外 UUID
func (m *PillRecipe) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// ---------- 丹方版本（不可变） ----------

// PillRecipeRevision 丹方版本，对应 pill_recipe_revisions 表
// 内容不可变：旧库存、已吸收能力不随新版本变化；已有版本不可原地编辑
// Revision 是从 1 开始的内部递增整数；VersionLabel 保留旧版本字符串，两者不能混用
type PillRecipeRevision struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:版本唯一标识"`
	UUID         uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识(API 指定版本用)"`
	RecipeID     string    `json:"-" gorm:"type:uuid;not null;uniqueIndex:idx_recipe_revision;comment:所属丹方UUID文本(011 业务键)"`
	Revision     int       `json:"revision" gorm:"not null;uniqueIndex:idx_recipe_revision;comment:内部递增版本号(从1开始)"`
	Name         string    `json:"name" gorm:"size:100;not null;comment:丹方名称"`
	Description  string    `json:"description" gorm:"type:text;comment:丹方简介(含触发语、反触发语)"`
	SkillSchema  JSONMap   `json:"skill_schema" gorm:"not null;serializer:json;comment:nuwa-skill 结构化内容(完整保留未知字段)"`
	Tags         JSONList  `json:"tags" gorm:"serializer:json;comment:标签数组"`
	Author       string    `json:"author" gorm:"size:100;comment:作者"`
	VersionLabel string    `json:"version_label" gorm:"size:20;default:1.0.0;comment:展示用版本字符串"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`

	// 011 关联标签：关系列存父实体 UUID 文本，FK 按父表 uuid 列约束
	Recipe PillRecipe `json:"-" gorm:"foreignKey:RecipeID;references:UUID;constraint:OnDelete:Restrict"`
}

// TableName 指定表名
func (PillRecipeRevision) TableName() string {
	return "pill_recipe_revisions"
}

// BeforeCreate 默认对外 UUID
func (m *PillRecipeRevision) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// ---------- 金丹实例（单枚库存） ----------

// PillItemState 库存状态：只允许 available 转为其他状态，终态不能回到 available
type PillItemState string

const (
	PillAvailable        PillItemState = "available"          // 可用
	PillConsumedByAgent  PillItemState = "consumed_by_agent"  // 已被道人服用（能力保留）
	PillConsumedByFusion PillItemState = "consumed_by_fusion" // 已作为融合材料消耗
	PillDiscarded        PillItemState = "discarded"          // 用户弃置（显式确认，不物理删除）
)

// PillItem 金丹实例，对应 pill_items 表
// 每枚有独立 ID；消耗后退出可用库存但保留记录用于去向展示与追溯
// OriginOperationID 必填：来源（炼制/迁移）成功操作；OriginIndex 为同操作内产出序号
type PillItem struct {
	ID                 uint          `json:"id" gorm:"primaryKey;autoIncrement;comment:实例唯一标识"`
	UUID               uuid.UUID     `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	RecipeRevisionID   string        `json:"-" gorm:"type:uuid;not null;index;comment:所属丹方版本UUID文本(禁止删除父记录)"`
	State              PillItemState `json:"state" gorm:"size:24;not null;default:available;index;comment:库存状态"`
	CreatedAt          time.Time     `json:"created_at" gorm:"autoCreateTime;comment:炼成时间"`
	ConsumedAt         *time.Time    `json:"consumed_at" gorm:"comment:消耗时间;空=未消耗"`
	ConsumeOperationID *string       `json:"-" gorm:"type:uuid;comment:消耗成功操作UUID文本;空=未消耗"`
	OriginOperationID  string        `json:"-" gorm:"type:uuid;not null;uniqueIndex:idx_item_origin;comment:来源成功操作UUID文本(炼制/迁移)"`
	OriginIndex        int           `json:"-" gorm:"not null;default:0;uniqueIndex:idx_item_origin;comment:同操作内产出序号(0起)"`

	// 011 关联标签：关系列存父实体 UUID 文本，FK 按父表 uuid 列约束
	RecipeRevision   PillRecipeRevision `json:"-" gorm:"foreignKey:RecipeRevisionID;references:UUID;constraint:OnDelete:Restrict"`
	OriginOperation  PillOperation      `json:"-" gorm:"foreignKey:OriginOperationID;references:UUID;constraint:OnDelete:Restrict"`
	ConsumeOperation *PillOperation     `json:"-" gorm:"foreignKey:ConsumeOperationID;references:UUID;constraint:OnDelete:Restrict"`
}

// TableName 指定表名
func (PillItem) TableName() string {
	return "pill_items"
}

// BeforeCreate 默认对外 UUID
func (m *PillItem) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// ---------- 已吸收能力（快照） ----------

// AgentPillEffect 道人已吸收能力，对应 agent_pill_effects 表
// 服用成功时保存配方内容、权重和顺序快照；不依赖原金丹还在库存
// 同一道人默认不重复吸收同一丹方版本：部分唯一索引 + 事务检查双保险
// ItemID 唯一：一枚实例只能产生一份能力
type AgentPillEffect struct {
	ID               uint       `json:"id" gorm:"primaryKey;autoIncrement;comment:能力唯一标识"`
	UUID             uuid.UUID  `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	AgentID          string     `json:"-" gorm:"type:uuid;not null;index;uniqueIndex:idx_agent_active_recipe_revision,where:removed_at IS NULL;comment:所属道人UUID文本"`
	ItemID           string     `json:"-" gorm:"type:uuid;not null;uniqueIndex;comment:来源实例UUID文本"`
	RecipeRevisionID string     `json:"-" gorm:"type:uuid;not null;uniqueIndex:idx_agent_active_recipe_revision,where:removed_at IS NULL;comment:吸收的丹方版本UUID文本"`
	NameSnapshot     string     `json:"name_snapshot" gorm:"size:100;not null;comment:吸收时的名称快照"`
	SchemaSnapshot   JSONMap    `json:"schema_snapshot" gorm:"not null;serializer:json;comment:吸收时的完整能力内容快照(深拷贝)"`
	Weight           float64    `json:"weight" gorm:"default:1.0;comment:权重(0-10,非剂量)"`
	SortOrder        int        `json:"sort_order" gorm:"default:0;comment:能力编排顺序"`
	CreatedAt        time.Time  `json:"created_at" gorm:"autoCreateTime;comment:吸收时间"`
	RemovedAt        *time.Time `json:"removed_at" gorm:"index;comment:移除时间;空=活跃(移除不返还金丹)"`

	// 来源实例（语言模式指纹/turn policy 身份均使用实例 UUID；禁止删除父记录）
	// 011 关联标签：关系列存父实体 UUID 文本，FK 按父表 uuid 列约束
	Item           PillItem           `json:"-" gorm:"foreignKey:ItemID;references:UUID;constraint:OnDelete:Restrict"`
	RecipeRevision PillRecipeRevision `json:"-" gorm:"foreignKey:RecipeRevisionID;references:UUID;constraint:OnDelete:Restrict"`
}

// TableName 指定表名
func (AgentPillEffect) TableName() string {
	return "agent_pill_effects"
}

// BeforeCreate 默认对外 UUID
func (m *AgentPillEffect) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// ---------- 幂等操作 ----------

// PillOperation 库存写操作结果，对应 pill_operations 表
// UUID 即全局幂等键；PayloadHash = 操作种类 + 标准化参数的 SHA-256
// 仅保留成功提交的结果；占位与业务变更同事务，外部不能读到"空结果的成功操作"
type PillOperation struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:操作唯一标识"`
	UUID        uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:全局幂等键"`
	Kind        string    `json:"kind" gorm:"size:32;not null;index;comment:操作种类: save_recipe/craft/consume/confirm_fusion/migration"`
	PayloadHash string    `json:"-" gorm:"char:64;not null;comment:请求负载哈希"`
	ResultJSON  JSONMap   `json:"-" gorm:"type:text;not null;serializer:json;comment:成功操作的结果"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;comment:提交时间"`
}

// TableName 指定表名
func (PillOperation) TableName() string {
	return "pill_operations"
}

// BeforeCreate 默认对外 UUID
func (m *PillOperation) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// ---------- 融合预览（两阶段） ----------

// FusionPreview 融合预览，对应 fusion_previews 表
// 预览不扣料、不占料；只能确认一次；有效期默认 15 分钟
// InputItemsJSON 保存输入实例 UUID 列表；InputHash 为排序后材料集合哈希；OutputJSON 为模型生成结果
type FusionPreview struct {
	ID                   uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:预览唯一标识"`
	UUID                 uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	InputItemsJSON       JSONList  `json:"-" gorm:"type:text;not null;serializer:json;comment:输入实例 UUID 列表"`
	InputHash            string    `json:"-" gorm:"char:64;not null;comment:排序后材料集合哈希"`
	OutputJSON           JSONMap   `json:"-" gorm:"type:text;not null;serializer:json;comment:模型生成结果(丹方草稿)"`
	OperatorSnapshot     JSONMap   `json:"-" gorm:"type:text;not null;serializer:json;comment:操作者信息快照(名称/UUID)"`
	CreatedAt            time.Time `json:"created_at" gorm:"autoCreateTime;comment:生成时间"`
	ExpiresAt            time.Time `json:"expires_at" gorm:"index;comment:过期时间"`
	ConfirmedOperationID *string   `json:"-" gorm:"type:uuid;comment:确认成功操作UUID文本;空=未确认(只能确认一次)"`

	// 011 关联标签：关系列存父实体 UUID 文本，FK 按父表 uuid 列约束
	ConfirmedOperation *PillOperation `json:"-" gorm:"foreignKey:ConfirmedOperationID;references:UUID;constraint:OnDelete:Restrict"`
}

// TableName 指定表名
func (FusionPreview) TableName() string {
	return "fusion_previews"
}

// BeforeCreate 默认对外 UUID
func (m *FusionPreview) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// ---------- 迁移状态 ----------

// PillMigrationState 库存迁移状态，对应 pill_migration_states 表
// 完成标记持久化：迁移用户不得再领取一次启动赠送；第二次启动直接跳过回填
type PillMigrationState struct {
	Key         string    `json:"key" gorm:"primaryKey;size:64;comment:迁移版本键(如 pill-inventory-v1)"`
	CompletedAt time.Time `json:"completed_at" gorm:"autoCreateTime;comment:完成时间"`
	ReportJSON  JSONMap   `json:"-" gorm:"type:text;not null;serializer:json;comment:迁移报告(计数/备份路径/判定;不含 schema 全文)"`
}

// TableName 指定表名
func (PillMigrationState) TableName() string {
	return "pill_migration_states"
}

// ---------- 旧数据映射 ----------

// PillLegacyMap 旧数据映射，对应 pill_legacy_maps 表
// 支持旧链接跳转与回填核对：旧定义→丹方(legacy_kind=pill)、旧绑定→能力(legacy_kind=bind)
type PillLegacyMap struct {
	LegacyKind string    `json:"-" gorm:"size:16;not null;uniqueIndex:idx_legacy_kind_id;comment:旧实体类型: pill/bind"`
	LegacyID   string    `json:"-" gorm:"size:64;not null;uniqueIndex:idx_legacy_kind_id;comment:旧实体标识(定义 UUID / 绑定行 ID)"`
	TargetUUID uuid.UUID `json:"-" gorm:"type:uuid;not null;comment:新实体 UUID"`
	CreatedAt  time.Time `json:"-" gorm:"autoCreateTime;comment:记录时间"`
}

// TableName 指定表名
func (PillLegacyMap) TableName() string {
	return "pill_legacy_maps"
}

// ---------- 一次性赠送 ----------

// PillStarterGrant 内置丹方首次启动赠送记录，对应 pill_starter_grants 表
// Disposition: granted=新用户实际赠送; legacy_accounted=迁移用户按旧数据核算过(不再领取)
type PillStarterGrant struct {
	ID          uint      `json:"-" gorm:"primaryKey;autoIncrement;comment:记录唯一标识"`
	RecipeID    string    `json:"-" gorm:"type:uuid;not null;uniqueIndex;comment:内置丹方UUID文本"`
	Disposition string    `json:"-" gorm:"size:24;not null;comment:granted/legacy_accounted"`
	ItemID      *string   `json:"-" gorm:"type:uuid;comment:赠送的可用实例UUID文本;legacy_accounted 为空"`
	CreatedAt   time.Time `json:"-" gorm:"autoCreateTime;comment:记录时间"`

	// 011 关联标签：关系列存父实体 UUID 文本，FK 按父表 uuid 列约束
	Recipe PillRecipe `json:"-" gorm:"foreignKey:RecipeID;references:UUID;constraint:OnDelete:Restrict"`
	Item   *PillItem  `json:"-" gorm:"foreignKey:ItemID;references:UUID;constraint:OnDelete:Restrict"`
}

// TableName 指定表名
func (PillStarterGrant) TableName() string {
	return "pill_starter_grants"
}
