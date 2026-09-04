// 旧金丹定义 → 丹方/库存迁移（金丹消耗品重构）
// 启动顺序要求：检查数据版本 → 升级前一致性备份 → 建表/索引 → 回填事务与数量校验 →
// 写完成标记 → 内置丹方初始化/一次性赠送（seed.go）→ 开放 HTTP。
// 本函数负责除内置初始化外的全部步骤；幂等：完成标记已存在则直接跳过。
// 失败不得静默丢弃记录或标记成功；旧表保留用于受控回滚；禁止在 Get/List 时懒迁移。
package dao

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// PillInventoryMigrationKey 迁移完成标记键
const PillInventoryMigrationKey = "pill-inventory-v1"

// PillInventoryMigrationBackupDir 一致性备份子目录（相对数据库文件所在目录）
const PillInventoryMigrationBackupDir = "backups"

// pillInventoryModels 金丹消耗品重构新建模型全集（AutoMigrate 自足用）
func pillInventoryModels() []any {
	return []any{
		&model.PillRecipe{}, &model.PillRecipeRevision{}, &model.PillItem{},
		&model.AgentPillEffect{}, &model.PillOperation{}, &model.FusionPreview{},
		&model.PillMigrationState{}, &model.PillLegacyMap{}, &model.PillStarterGrant{},
	}
}

// legacyPillColumns / legacyBindColumns 迁移预检要求的旧表列（缺失即异常 schema）
var legacyPillColumns = []string{"id", "uuid", "name", "description", "skill_schema", "tags", "author", "version", "is_builtin"}
var legacyBindColumns = []string{"id", "agent_id", "pill_id", "weight", "sort_order", "created_at"}

// MigratePillInventory 桌面启动对外开放 API 前调用。
// 流程：完成标记幂等 → fresh/legacy 判定 → legacy 预检（缺列/孤儿/重复绑定）→
// 一致性备份 → 单事务回填 + 数量断言 + 完成标记。任何失败返回错误并保持旧世界可用。
func MigratePillInventory(db *gorm.DB) error {
	// 1) 幂等：已迁移过直接跳过（第二次启动不新增任何数据，防重启复活）
	if db.Migrator().HasTable(&model.PillMigrationState{}) {
		var done int64
		if err := db.Model(&model.PillMigrationState{}).Where("key = ?", PillInventoryMigrationKey).Count(&done).Error; err != nil {
			return fmt.Errorf("查询迁移完成标记失败: %w", err)
		}
		if done > 0 {
			log.Printf("[炼丹炉] 金丹库存迁移已完成(pill-inventory-v1)，跳过")
			return nil
		}
	}

	// 2) 新安装/升级判定：旧表不存在或为空 = 全新安装
	legacyPillCount, err := countLegacyPills(db)
	if err != nil {
		return err
	}
	isFresh := legacyPillCount == 0
	// 异常数据：存在绑定记录却没有任何旧金丹定义，正常流程不可能产生
	// （绑定必须有定义才能创建）；不得当作全新安装静默吞掉，报告并阻止切换
	if isFresh && db.Migrator().HasTable("agent_pills") {
		var bindCount int64
		if err := db.Model(&model.AgentPill{}).Count(&bindCount).Error; err != nil {
			return fmt.Errorf("读取旧服用记录失败: %w", err)
		}
		if bindCount > 0 {
			return fmt.Errorf("异常数据: agent_pills 存在 %d 条绑定但 elixir_pills 为空，拒绝迁移（疑似数据损坏）", bindCount)
		}
	}

	report := model.JSONMap{
		"is_fresh_install": isFresh,
		"legacy_pills":     legacyPillCount,
		"legacy_binds":     0,
		"recipes":          0,
		"available_items":  0,
		"history_items":    0,
		"effects":          0,
	}

	// 3) 升级库：先一致性备份，再迁移（备份失败必须阻止升级）
	var backupPath string
	if !isFresh {
		backupPath, err = backupBeforeUpgrade(db)
		if err != nil {
			return fmt.Errorf("迁移前一致性备份失败，已阻止升级: %w", err)
		}
		report["backup_path"] = backupPath
		log.Printf("[炼丹炉] 金丹库存迁移前备份完成: %s", backupPath)
	}

	// 4) 建表/索引（幂等；测试直接调本函数时自足）
	if err := db.AutoMigrate(pillInventoryModels()...); err != nil {
		return fmt.Errorf("创建库存表失败: %w", err)
	}

	// 5) 单事务：预检 + 回填 + 数量断言 + 完成标记；失败整体回滚，不切新读取
	err = db.Transaction(func(tx *gorm.DB) error {
		if isFresh {
			return writeMigrationState(tx, report)
		}
		stats, err := migrateLegacyData(tx, report)
		if err != nil {
			return err
		}
		report["legacy_binds"] = stats.binds
		report["recipes"] = stats.recipes
		report["available_items"] = stats.available
		report["history_items"] = stats.history
		report["effects"] = stats.effects
		return writeMigrationState(tx, report)
	})
	if err != nil {
		return fmt.Errorf("金丹库存迁移失败: %w", err)
	}

	log.Printf("[炼丹炉] 金丹库存迁移完成：旧定义=%v 旧绑定=%v 丹方=%v 可用=%v 历史=%v 能力=%v",
		report["legacy_pills"], report["legacy_binds"], report["recipes"],
		report["available_items"], report["history_items"], report["effects"])
	return nil
}

// countLegacyPills 旧表存在性 + 行数（新安装判定依据）
func countLegacyPills(db *gorm.DB) (int64, error) {
	if !db.Migrator().HasTable(&model.ElixirPill{}) {
		return 0, nil
	}
	var n int64
	if err := db.Model(&model.ElixirPill{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("读取旧金丹表失败: %w", err)
	}
	return n, nil
}

// migrationStats 回填计数（事务内断言用）
type migrationStats struct {
	binds     int64
	recipes   int64
	available int64
	history   int64
	effects   int64
}

// migrateLegacyData 迁移回填主体（调用方保证在事务内）：
// 每旧定义 → 1 丹方 + 1 v1 版本 + 1 迁移来源操作；未绑定 → 1 可用实例；
// N 个绑定 → N 份已服用历史实例 + N 份能力快照（保留权重/顺序/时间/未知字段）。
func migrateLegacyData(tx *gorm.DB, report model.JSONMap) (*migrationStats, error) {
	// 结构预检先行：旧表缺列（含表不存在）必须在读取数据前报出，
	// 否则会以 "no such table" 之类噪声掩盖异常 schema，且不满足"报告不静默"要求
	if err := checkColumns(tx, "elixir_pills", legacyPillColumns); err != nil {
		return nil, err
	}
	if tx.Migrator().HasTable("agent_pills") {
		if err := checkColumns(tx, "agent_pills", legacyBindColumns); err != nil {
			return nil, err
		}
	}

	// 读旧表快照（按 id 排序保证确定性）
	var pills []model.ElixirPill
	if err := tx.Order("id").Find(&pills).Error; err != nil {
		return nil, fmt.Errorf("读取旧金丹定义失败: %w", err)
	}
	var binds []model.AgentPill
	if err := tx.Order("id").Find(&binds).Error; err != nil {
		return nil, fmt.Errorf("读取旧服用记录失败: %w", err)
	}

	// 数据完整性预检：孤儿绑定 / 重复绑定——报告并阻止切换，不静默丢弃
	if err := preflightLegacy(tx, pills, binds); err != nil {
		return nil, err
	}

	byPill := map[string][]model.AgentPill{}
	for _, b := range binds {
		byPill[b.PillID] = append(byPill[b.PillID], b)
	}

	stats := &migrationStats{binds: int64(len(binds))}

	for _, pill := range pills {
		// 迁移来源操作：OriginOperationID 必填，提供来源追溯
		op := model.PillOperation{
			Kind:        "migration",
			PayloadHash: migrationPayloadHash(pill),
			ResultJSON: model.JSONMap{
				"kind": "migration", "pill_uuid": pill.UUID.String(), "legacy_pill_id": pill.ID,
			},
		}
		if err := tx.Create(&op).Error; err != nil {
			return nil, fmt.Errorf("写入迁移来源操作失败: %w", err)
		}

		// 丹方 + 不可变 v1 版本（完整保留未知 skill_schema 字段）
		recipe := model.PillRecipe{IsBuiltin: pill.IsBuiltin, CreatedAt: pill.CreatedAt}
		if err := tx.Create(&recipe).Error; err != nil {
			return nil, fmt.Errorf("写入丹方失败: %w", err)
		}
		rev := model.PillRecipeRevision{
			RecipeID:     recipe.UUID.String(),
			Revision:     1,
			Name:         pill.Name,
			Description:  pill.Description,
			SkillSchema:  deepCopyJSON(pill.SkillSchema),
			Tags:         pill.Tags,
			Author:       pill.Author,
			VersionLabel: pill.Version,
			CreatedAt:    pill.CreatedAt,
		}
		if err := tx.Create(&rev).Error; err != nil {
			return nil, fmt.Errorf("写入丹方版本失败: %w", err)
		}
		if err := tx.Model(&recipe).Update("current_revision_id", rev.UUID.String()).Error; err != nil {
			return nil, fmt.Errorf("回填丹方当前版本失败: %w", err)
		}
		stats.recipes++

		// 旧定义 → 丹方映射（旧链接跳转与回填核对）
		if err := tx.Create(&model.PillLegacyMap{
			LegacyKind: "pill", LegacyID: pill.UUID.String(), TargetUUID: recipe.UUID,
		}).Error; err != nil {
			return nil, fmt.Errorf("写入旧金丹映射失败: %w", err)
		}

		// 实例：未绑定 → 1 枚可用；绑定 N 个 → N 枚历史已服用（可用库存 0）
		myBinds := byPill[pill.UUID.String()]
		if len(myBinds) == 0 {
			item := model.PillItem{
				RecipeRevisionID:  rev.UUID.String(),
				State:             model.PillAvailable,
				OriginOperationID: op.UUID.String(),
				OriginIndex:       0,
				CreatedAt:         pill.CreatedAt,
			}
			if err := tx.Create(&item).Error; err != nil {
				return nil, fmt.Errorf("写入可用实例失败: %w", err)
			}
			stats.available++
			continue
		}

		// 排序保持旧服用顺序（sort_order 优先，其次行 id）
		sort.SliceStable(myBinds, func(i, j int) bool {
			if myBinds[i].SortOrder != myBinds[j].SortOrder {
				return myBinds[i].SortOrder < myBinds[j].SortOrder
			}
			return myBinds[i].ID < myBinds[j].ID
		})
		for i, b := range myBinds {
			consumedAt := b.CreatedAt
			opUID := op.UUID.String()
			item := model.PillItem{
				RecipeRevisionID:   rev.UUID.String(),
				State:              model.PillConsumedByAgent,
				ConsumedAt:         &consumedAt,
				ConsumeOperationID: &opUID,
				OriginOperationID:  opUID,
				OriginIndex:        i,
				CreatedAt:          pill.CreatedAt,
			}
			if err := tx.Create(&item).Error; err != nil {
				return nil, fmt.Errorf("写入历史实例失败: %w", err)
			}
			stats.history++

			// 能力快照：保留名称/完整内容/权重/顺序/吸收时间
			eff := model.AgentPillEffect{
				AgentID:          b.AgentID,
				ItemID:           item.UUID.String(),
				RecipeRevisionID: rev.UUID.String(),
				NameSnapshot:     pill.Name,
				SchemaSnapshot:   deepCopyJSON(pill.SkillSchema),
				Weight:           b.Weight,
				SortOrder:        b.SortOrder,
				CreatedAt:        b.CreatedAt,
			}
			if err := tx.Create(&eff).Error; err != nil {
				return nil, fmt.Errorf("写入能力快照失败: %w", err)
			}
			stats.effects++

			// 旧绑定 → 能力映射
			if err := tx.Create(&model.PillLegacyMap{
				LegacyKind: "bind", LegacyID: fmt.Sprintf("%d", b.ID), TargetUUID: eff.UUID,
			}).Error; err != nil {
				return nil, fmt.Errorf("写入旧绑定映射失败: %w", err)
			}
		}
	}

	// 数量断言：回填事务内校验，任何偏差整体回滚
	if err := assertMigrationCounts(tx, pills, binds, stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// preflightLegacy 数据完整性预检：孤儿绑定、重复绑定（结构预检见 migrateLegacyData 开头）
func preflightLegacy(tx *gorm.DB, pills []model.ElixirPill, binds []model.AgentPill) error {
	// 孤儿绑定：绑定指向不存在的旧定义，无法确定丹方
	pillUUIDs := map[string]bool{}
	for _, p := range pills {
		pillUUIDs[p.UUID.String()] = true
	}
	for _, b := range binds {
		if !pillUUIDs[b.PillID] {
			return fmt.Errorf("异常数据: agent_pills.id=%d 引用不存在的旧金丹 pill_id=%s，拒绝迁移", b.ID, b.PillID)
		}
	}

	// 重复绑定：同旧定义与道人重复行，迁移前报告并阻止切换，不静默丢弃
	seen := map[[2]string]bool{}
	for _, b := range binds {
		key := [2]string{b.AgentID, b.PillID}
		if seen[key] {
			return fmt.Errorf("异常数据: agent_pills 存在重复绑定 (agent_id=%s, pill_id=%s)，行 id=%d，拒绝迁移", b.AgentID, b.PillID, b.ID)
		}
		seen[key] = true
	}
	return nil
}

// checkColumns 检查表是否包含全部必需列
func checkColumns(tx *gorm.DB, table string, want []string) error {
	cols := map[string]bool{}
	rows, err := tx.Raw(fmt.Sprintf("PRAGMA table_info(%s)", table)).Rows()
	if err != nil {
		return fmt.Errorf("读取旧表 %s 结构失败: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("解析旧表 %s 结构失败: %w", table, err)
		}
		cols[name] = true
	}
	for _, w := range want {
		if !cols[w] {
			return fmt.Errorf("异常 schema: 旧表 %s 缺少列 %q，拒绝迁移", table, w)
		}
	}
	return nil
}

// assertMigrationCounts 回填数量断言（事务内）
func assertMigrationCounts(tx *gorm.DB, pills []model.ElixirPill, binds []model.AgentPill, stats *migrationStats) error {
	counts := []struct {
		table string
		want  int64
	}{
		{"pill_recipes", stats.recipes},
		{"pill_recipe_revisions", stats.recipes},
		{"pill_items", stats.available + stats.history},
		{"agent_pill_effects", stats.effects},
		{"pill_legacy_maps", int64(len(pills) + len(binds))},
	}
	for _, c := range counts {
		var got int64
		if err := tx.Table(c.table).Count(&got).Error; err != nil {
			return fmt.Errorf("数量断言查询 %s 失败: %w", c.table, err)
		}
		if got != c.want {
			return fmt.Errorf("迁移数量断言失败: %s got %d want %d，整体回滚", c.table, got, c.want)
		}
	}
	return nil
}

// writeMigrationState 写完成标记（迁移报告不记录 schema 全文或密钥）
func writeMigrationState(tx *gorm.DB, report model.JSONMap) error {
	if err := tx.Create(&model.PillMigrationState{Key: PillInventoryMigrationKey, ReportJSON: report}).Error; err != nil {
		return fmt.Errorf("写入迁移完成标记失败: %w", err)
	}
	return nil
}

// migrationPayloadHash 迁移来源操作的标准化负载哈希
func migrationPayloadHash(pill model.ElixirPill) string {
	payload := "migration|" + pill.UUID.String() + "|" + fmt.Sprintf("%d", pill.ID)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// deepCopyJSON 深拷贝能力内容：不保存指向可变 map 的共享对象，完整保留未知字段
func deepCopyJSON(v model.JSONMap) model.JSONMap {
	if v == nil {
		return model.JSONMap{}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		// 结构上不可能失败（调用方已从库中读出）；兜底直接拷贝引用
		out := model.JSONMap{}
		for k, val := range v {
			out[k] = val
		}
		return out
	}
	var out model.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil {
		return model.JSONMap{}
	}
	return out
}

// backupBeforeUpgrade 升级前一致性备份：
// VACUUM INTO 由 SQLite 自身产生一致性快照（WAL 自动合并），再重开备份文件做 integrity_check。
// 备份文件落在数据库同目录 backups/ 下；备份失败必须阻止升级。
func backupBeforeUpgrade(db *gorm.DB) (string, error) {
	dial, ok := db.Dialector.(*sqlite.Dialector)
	if !ok || dial.DSN == "" {
		return "", fmt.Errorf("无法确定数据库文件路径（仅支持 SQLite 文件库）")
	}
	dbPath, err := sqliteFilePath(dial.DSN)
	if err != nil {
		return "", err
	}
	backupDir := filepath.Join(filepath.Dir(dbPath), PillInventoryMigrationBackupDir)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("创建备份目录失败: %w", err)
	}
	dest := filepath.Join(backupDir, fmt.Sprintf("pill-inventory-%s.db", time.Now().Format("20060102-150405")))

	if err := db.Exec("VACUUM INTO ?", dest).Error; err != nil {
		return "", fmt.Errorf("VACUUM INTO 失败: %w", err)
	}
	// 重开备份做一致性校验
	backupDB, err := gorm.Open(sqlite.Open(dest), &gorm.Config{})
	if err != nil {
		return "", fmt.Errorf("备份文件打不开: %w", err)
	}
	var integrity string
	if err := backupDB.Raw("PRAGMA integrity_check").Row().Scan(&integrity); err != nil {
		return "", fmt.Errorf("备份一致性校验失败: %w", err)
	}
	if integrity != "ok" {
		return "", fmt.Errorf("备份一致性校验 = %q, want ok", integrity)
	}
	return dest, nil
}

// sqliteFilePath 从 DSN 解析 SQLite 文件路径（file: 前缀与 query 参数剥离）
func sqliteFilePath(dsn string) (string, error) {
	p := dsn
	if strings.HasPrefix(p, "file:") {
		p = strings.TrimPrefix(p, "file:")
	}
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSpace(p)
	if p == "" || p == ":memory:" {
		return "", fmt.Errorf("数据库不是文件库 (dsn=%q)，无法备份", dsn)
	}
	return p, nil
}
