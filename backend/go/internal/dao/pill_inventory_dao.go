// 丹方/金丹库存事务 DAO（金丹消耗品重构）
// 约定：所有函数第一参数是事务句柄 tx（调用方从 service 事务传入），
// 绝不内部使用全局 DB —— 保证「占位与业务变更同一事务、失败一起回滚」。
// 写操作中可能并发争抢的部分用条件更新 + RowsAffected 判断，不依赖 SELECT FOR UPDATE。
package dao

import (
	"strings"
	"time"

	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------- 单条查询 ----------

// PillRecipeByUUID 按对外 UUID 查丹方（gorm.ErrRecordNotFound 表示不存在）
func PillRecipeByUUID(tx *gorm.DB, uid uuid.UUID) (*model.PillRecipe, error) {
	var r model.PillRecipe
	err := tx.Where("uuid = ?", uid).First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// PillRecipeByID 按丹方 UUID 文本(011 关系列值)查丹方
func PillRecipeByID(tx *gorm.DB, uid string) (*model.PillRecipe, error) {
	var r model.PillRecipe
	err := tx.Where("uuid = ?", uid).First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// PillRecipeRevisionByUUID 按对外 UUID 查不可变版本
func PillRecipeRevisionByUUID(tx *gorm.DB, uid uuid.UUID) (*model.PillRecipeRevision, error) {
	var r model.PillRecipeRevision
	err := tx.Where("uuid = ?", uid).First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// PillRecipeRevisionByID 按版本 UUID 文本(011 关系列值)查不可变版本
func PillRecipeRevisionByID(tx *gorm.DB, uid string) (*model.PillRecipeRevision, error) {
	var r model.PillRecipeRevision
	err := tx.Where("uuid = ?", uid).First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// PillItemByUUID 按对外 UUID 查金丹实例
func PillItemByUUID(tx *gorm.DB, uid uuid.UUID) (*model.PillItem, error) {
	var i model.PillItem
	err := tx.Where("uuid = ?", uid).First(&i).Error
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// PillOperationByUUID 按幂等键查已提交操作
func PillOperationByUUID(tx *gorm.DB, uid uuid.UUID) (*model.PillOperation, error) {
	var op model.PillOperation
	err := tx.Where("uuid = ?", uid).First(&op).Error
	if err != nil {
		return nil, err
	}
	return &op, nil
}

// PillOperationByID 按操作 UUID 文本(011 关系列值)查已提交操作（预览绑定关系读操作信息用）
func PillOperationByID(tx *gorm.DB, uid string) (*model.PillOperation, error) {
	var op model.PillOperation
	err := tx.Where("uuid = ?", uid).First(&op).Error
	if err != nil {
		return nil, err
	}
	return &op, nil
}

// FusionPreviewByUUID 按对外 UUID 查融合预览
func FusionPreviewByUUID(tx *gorm.DB, uid uuid.UUID) (*model.FusionPreview, error) {
	var p model.FusionPreview
	err := tx.Where("uuid = ?", uid).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPillItemsByUUIDs 按 UUID 列表批量加载金丹实例（融合预览/确认材料加载）
func ListPillItemsByUUIDs(tx *gorm.DB, uids []uuid.UUID) ([]model.PillItem, error) {
	var items []model.PillItem
	if err := tx.Where("uuid IN ?", uids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ---------- 写入 ----------

// CreatePillRecipe 建丹方（BeforeCreate 生成 UUID）
func CreatePillRecipe(tx *gorm.DB, recipe *model.PillRecipe) error {
	return tx.Create(recipe).Error
}

// CreatePillRecipeRevision 建不可变版本
func CreatePillRecipeRevision(tx *gorm.DB, rev *model.PillRecipeRevision) error {
	return tx.Create(rev).Error
}

// SetPillRecipeCurrentRevision 回填丹方当前版本（创建事务内先空后回填）
// recipeID 为丹方自表主键；revUID 为版本 UUID 文本(011 关系列值)
func SetPillRecipeCurrentRevision(tx *gorm.DB, recipeID uint, revUID string) error {
	return tx.Model(&model.PillRecipe{}).Where("id = ?", recipeID).Update("current_revision_id", revUID).Error
}

// CreatePillItem 建金丹实例（来源操作 + 同操作内序号唯一）
func CreatePillItem(tx *gorm.DB, item *model.PillItem) error {
	return tx.Create(item).Error
}

// CreatePillOperation 插入操作占位（UUID 唯一约束承担并发幂等兜底）
func CreatePillOperation(tx *gorm.DB, op *model.PillOperation) error {
	return tx.Create(op).Error
}

// SetPillOperationResult 事务内写完整结果（占位与结果同事务，外部读不到空结果操作）
func SetPillOperationResult(tx *gorm.DB, opID uint, result model.JSONMap) error {
	return tx.Model(&model.PillOperation{}).Where("id = ?", opID).Update("result_json", result).Error
}

// ---------- 条件更新（CAS） ----------

// DiscardPillItemCAS available→discarded 终态；返回 false 表示已非可用（竞争/重复）
func DiscardPillItemCAS(tx *gorm.DB, itemID uint) (bool, error) {
	res := tx.Model(&model.PillItem{}).
		Where("id = ? AND state = ?", itemID, model.PillAvailable).
		Update("state", model.PillDiscarded)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// SetPillRecipeArchived 写归档时间（幂等：已归档重复执行无副作用）
func SetPillRecipeArchived(tx *gorm.DB, recipeID uint, now time.Time) error {
	return tx.Model(&model.PillRecipe{}).Where("id = ?", recipeID).Update("archived_at", now).Error
}

// CreateFusionPreview 持久化融合预览（预览本身非幂等写：模型调用不在事务内，失败不落行）
func CreateFusionPreview(tx *gorm.DB, preview *model.FusionPreview) error {
	return tx.Create(preview).Error
}

// ConfirmFusionPreviewCAS 单 SQL 完成「写 lineage 附加后的输出 + 条件绑定确认操作」：
// 只允许未确认预览绑定成功；RowsAffected==0 表示已被其他操作确认（并发双确认防护）
func ConfirmFusionPreviewCAS(tx *gorm.DB, previewID uint, opUID string, outputJSON model.JSONMap) (bool, error) {
	res := tx.Model(&model.FusionPreview{}).
		Where("id = ? AND confirmed_operation_id IS NULL", previewID).
		Updates(map[string]any{
			"confirmed_operation_id": opUID,
			"output_json":            outputJSON,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// ConsumeFusionItemsCAS 批量消耗融合材料：全部材料 available→consumed_by_fusion 并写去向；
// 任一材料已非可用则整批 0 行（条件更新原子性，不部分消耗）
func ConsumeFusionItemsCAS(tx *gorm.DB, itemIDs []uint, now time.Time, opUID string) (bool, error) {
	res := tx.Model(&model.PillItem{}).
		Where("id IN ? AND state = ?", itemIDs, model.PillAvailable).
		Updates(map[string]any{
			"state":                model.PillConsumedByFusion,
			"consumed_at":          now,
			"consume_operation_id": opUID,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == int64(len(itemIDs)), nil
}

// ---------- 列表与聚合 ----------

// ListPillRecipesPaged 丹方分页；keyword 匹配名称；includeArchived=false 时排除已归档
func ListPillRecipesPaged(tx *gorm.DB, page, size int, keyword string, includeArchived bool) (int64, []model.PillRecipe, error) {
	q := tx.Model(&model.PillRecipe{})
	if kw := strings.TrimSpace(keyword); kw != "" {
		q = q.Where("name LIKE ?", "%"+kw+"%")
	}
	if !includeArchived {
		q = q.Where("archived_at IS NULL")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var recipes []model.PillRecipe
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&recipes).Error; err != nil {
		return 0, nil, err
	}
	return total, recipes, nil
}

// availableCountRow 聚合行
type availableCountRow struct {
	RecipeID string
	N        int64
}

// AvailablePillCountByRecipe 可用数量聚合：
// 按 state='available' GROUP BY recipe_id（011 关系列为 UUID 文本），不新增可漂移的 quantity 字段
func AvailablePillCountByRecipe(tx *gorm.DB) (map[string]int64, error) {
	var rows []availableCountRow
	err := tx.Raw(`
		SELECT r.recipe_id AS recipe_id, COUNT(*) AS n
		FROM pill_items i
		JOIN pill_recipe_revisions r ON r.uuid = i.recipe_revision_id
		WHERE i.state = ?
		GROUP BY r.recipe_id
	`, model.PillAvailable).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.RecipeID] = row.N
	}
	return out, nil
}

// ListAvailablePillItems 可用库存分页；recipeUID 非空时按丹方过滤（经版本表 join，
// 011 关系列为 UUID 文本）
func ListAvailablePillItems(tx *gorm.DB, page, size int, recipeUID *string) (int64, []model.PillItem, error) {
	base := tx.Table("pill_items AS i").
		Joins("JOIN pill_recipe_revisions r ON r.uuid = i.recipe_revision_id").
		Where("i.state = ?", model.PillAvailable)
	if recipeUID != nil {
		base = base.Where("r.recipe_id = ?", *recipeUID)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var items []model.PillItem
	if err := base.Select("i.*").Order("i.id DESC").Offset((page - 1) * size).Limit(size).Scan(&items).Error; err != nil {
		return 0, nil, err
	}
	return total, items, nil
}

// ---------- 批量查询（任务 5 能力列表组装） ----------

// PillItemsByIDs 按 UUID 文本(011 关系列值)批量查实例；返回 uuid 文本→实例 映射（不存在的键不在 map 中）
func PillItemsByIDs(tx *gorm.DB, uids []string) (map[string]model.PillItem, error) {
	out := make(map[string]model.PillItem, len(uids))
	if len(uids) == 0 {
		return out, nil
	}
	var items []model.PillItem
	if err := tx.Where("uuid IN ?", uids).Find(&items).Error; err != nil {
		return nil, err
	}
	for _, it := range items {
		out[it.UUID.String()] = it
	}
	return out, nil
}

// PillRecipeRevisionsByIDs 按 UUID 文本(011 关系列值)批量查不可变版本；返回 uuid 文本→版本 映射
func PillRecipeRevisionsByIDs(tx *gorm.DB, uids []string) (map[string]model.PillRecipeRevision, error) {
	out := make(map[string]model.PillRecipeRevision, len(uids))
	if len(uids) == 0 {
		return out, nil
	}
	var revs []model.PillRecipeRevision
	if err := tx.Where("uuid IN ?", uids).Find(&revs).Error; err != nil {
		return nil, err
	}
	for _, r := range revs {
		out[r.UUID.String()] = r
	}
	return out, nil
}

// PillRecipesByIDs 按 UUID 文本(011 关系列值)批量查丹方；返回 uuid 文本→丹方 映射（任务 5 库存列表组装）
func PillRecipesByIDs(tx *gorm.DB, uids []string) (map[string]model.PillRecipe, error) {
	out := make(map[string]model.PillRecipe, len(uids))
	if len(uids) == 0 {
		return out, nil
	}
	var recipes []model.PillRecipe
	if err := tx.Where("uuid IN ?", uids).Find(&recipes).Error; err != nil {
		return nil, err
	}
	for _, r := range recipes {
		out[r.UUID.String()] = r
	}
	return out, nil
}

// PillLegacyMapByKindID 按 (legacy_kind, legacy_id) 唯一键读取旧实体映射
// （任务 5 旧入口封堵：旧金丹详情跳转与旧 pill ID 导出）。
// 未找到返回 gorm.ErrRecordNotFound，调用方映射为 404。
func PillLegacyMapByKindID(tx *gorm.DB, kind, legacyID string) (*model.PillLegacyMap, error) {
	var m model.PillLegacyMap
	if err := tx.Where("legacy_kind = ? AND legacy_id = ?", kind, legacyID).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}
