// Base gorm.Model 约定字段的业务键版完整实现
// 四字段对齐 gorm.Model:ID uint 自增主键(物理键;按裁决保留存在、业务代码暂不使用)
// + CreatedAt/UpdatedAt(时间戳自动维护) + DeletedAt(软删:查询自动追加 WHERE deleted_at IS NULL,
// Delete 写 deleted_at;需物理删除的 junction 清理点显式 Unscoped())。
// 业务主键由各实体自行声明 <EntityID> string = uuid.UUID.String()(011 修订轮裁决),
// 以 uniqueIndex 保证唯一并充当 FK 目标;业务代码一律按 uuid 业务键查询/关联,不读取 Base.ID。
package model

import (
	"time"

	"gorm.io/gorm"
)

type Base struct {
	ID        uint           `json:"-" gorm:"primaryKey;autoIncrement;comment:内部自增主键(暂不使用,保留约定)"`
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index;comment:软删时间(空=存活)"`
}
