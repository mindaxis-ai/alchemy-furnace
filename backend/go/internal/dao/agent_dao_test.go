package dao

import (
	"context"
	"path/filepath"
	"testing"

	idao "github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// newAgentDAOTestDB 建立含道人/金丹/服用记录/语言模式缓存的独立 SQLite 测试库
func newAgentDAOTestDB(t *testing.T) *AgentDao {
	t.Helper()
	db := newSQLiteTestDB(t, filepath.Join(t.TempDir(), "agent-dao.db"))
	if err := db.AutoMigrate(&model.DaoAgent{}, &model.ElixirPill{}, &model.AgentPill{}, &model.LanguagePattern{}); err != nil {
		t.Fatalf("AutoMigrate agent models: %v", err)
	}
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	return NewAgentDao()
}

// seedAgentWithPills 建一个道人与 n 枚金丹,返回道人与金丹(按创建顺序)
func seedAgentWithPills(t *testing.T, pillCount int) (*model.DaoAgent, []*model.ElixirPill) {
	t.Helper()
	agent := &model.DaoAgent{UUID: uuid.New(), Name: "测试道人", Status: "active", ModelName: "test-model"}
	if err := DB.Create(agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	pills := make([]*model.ElixirPill, 0, pillCount)
	for i := 0; i < pillCount; i++ {
		pill := &model.ElixirPill{
			UUID:        uuid.New(),
			Name:        uuid.NewString(),
			SkillSchema: model.JSONMap{"identity_card": "x"},
		}
		if err := DB.Create(pill).Error; err != nil {
			t.Fatalf("create pill %d: %v", i, err)
		}
		pills = append(pills, pill)
	}
	return agent, pills
}

// listAgentPillRows 按 sort_order,id 升序读出某道人的全部服用记录
func listAgentPillRows(t *testing.T, agentID uint) []model.AgentPill {
	t.Helper()
	var rows []model.AgentPill
	if err := DB.Where("agent_id = ?", agentID).
		Order("sort_order ASC, id ASC").
		Find(&rows).Error; err != nil {
		t.Fatalf("list agent_pills: %v", err)
	}
	return rows
}

func TestReplaceAgentPillsWritesOrderedWeightsAndInvalidatesCache(t *testing.T) {
	dao := NewAgentDao()
	newAgentDAOTestDB(t)
	agent, pills := seedAgentWithPills(t, 3)
	// 预置有效语言模式缓存,成功后应同事务失效
	if err := DB.Create(&model.LanguagePattern{
		AgentID: agent.ID, SystemPrompt: "cached", SourceFingerprint: "sha256:x", IsValid: true,
	}).Error; err != nil {
		t.Fatalf("create language pattern: %v", err)
	}

	inputs := []idao.AgentPillInput{
		{PillID: pills[2].ID, Weight: 2.5},
		{PillID: pills[0].ID, Weight: 1.0},
		{PillID: pills[1].ID, Weight: 0.5},
	}
	if err := dao.ReplaceAgentPills(context.Background(), agent.ID, inputs); err != nil {
		t.Fatalf("ReplaceAgentPills error = %v", err)
	}

	rows := listAgentPillRows(t, agent.ID)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	// 请求顺序即 sort_order=1..n,权重随记录落库
	wantPills := []uint{pills[2].ID, pills[0].ID, pills[1].ID}
	wantWeights := []float64{2.5, 1.0, 0.5}
	for i, row := range rows {
		if row.SortOrder != i+1 {
			t.Fatalf("rows[%d].SortOrder = %d, want %d", i, row.SortOrder, i+1)
		}
		if row.PillID != wantPills[i] {
			t.Fatalf("rows[%d].PillID = %d, want %d", i, row.PillID, wantPills[i])
		}
		if row.Weight != wantWeights[i] {
			t.Fatalf("rows[%d].Weight = %v, want %v", i, row.Weight, wantWeights[i])
		}
	}

	var pattern model.LanguagePattern
	if err := DB.Where("agent_id = ?", agent.ID).First(&pattern).Error; err != nil {
		t.Fatalf("load language pattern: %v", err)
	}
	if pattern.IsValid {
		t.Fatal("language pattern still valid after ReplaceAgentPills, want invalidated")
	}
}

func TestReplaceAgentPillsRemovesOldRelations(t *testing.T) {
	dao := NewAgentDao()
	newAgentDAOTestDB(t)
	agent, pills := seedAgentWithPills(t, 3)

	if err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[0].ID, Weight: 1},
		{PillID: pills[1].ID, Weight: 1},
		{PillID: pills[2].ID, Weight: 1},
	}); err != nil {
		t.Fatalf("first ReplaceAgentPills error = %v", err)
	}
	// 用子集替换:仅保留 pills[1]
	if err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[1].ID, Weight: 3},
	}); err != nil {
		t.Fatalf("second ReplaceAgentPills error = %v", err)
	}

	rows := listAgentPillRows(t, agent.ID)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 after subset replace", len(rows))
	}
	if rows[0].PillID != pills[1].ID || rows[0].Weight != 3 || rows[0].SortOrder != 1 {
		t.Fatalf("remaining row = %+v, want pill=%d weight=3 sort_order=1", rows[0], pills[1].ID)
	}
}

func TestReplaceAgentPillsEmptyClearsRelations(t *testing.T) {
	dao := NewAgentDao()
	newAgentDAOTestDB(t)
	agent, pills := seedAgentWithPills(t, 2)

	if err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[0].ID, Weight: 1},
		{PillID: pills[1].ID, Weight: 1},
	}); err != nil {
		t.Fatalf("seed ReplaceAgentPills error = %v", err)
	}
	if err := dao.ReplaceAgentPills(context.Background(), agent.ID, nil); err != nil {
		t.Fatalf("empty ReplaceAgentPills error = %v", err)
	}
	if rows := listAgentPillRows(t, agent.ID); len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 after empty replace", len(rows))
	}
}

func TestReplaceAgentPillsRollsBackWhenPillMissing(t *testing.T) {
	dao := NewAgentDao()
	newAgentDAOTestDB(t)
	agent, pills := seedAgentWithPills(t, 2)
	if err := DB.Create(&model.LanguagePattern{
		AgentID: agent.ID, SystemPrompt: "cached", SourceFingerprint: "sha256:x", IsValid: true,
	}).Error; err != nil {
		t.Fatalf("create language pattern: %v", err)
	}
	// 旧关系: pills[0]
	if err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[0].ID, Weight: 1},
	}); err != nil {
		t.Fatalf("seed ReplaceAgentPills error = %v", err)
	}
	// 种子成功后缓存被失效;重建有效缓存以验证回滚时缓存不被误失效
	if err := DB.Model(&model.LanguagePattern{}).Where("agent_id = ?", agent.ID).Update("is_valid", true).Error; err != nil {
		t.Fatalf("re-validate pattern: %v", err)
	}

	// 含一个不存在的 pill id,整个事务应回滚
	missingID := uint(999999)
	err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[1].ID, Weight: 2},
		{PillID: missingID, Weight: 1},
	})
	if err == nil {
		t.Fatal("ReplaceAgentPills with missing pill succeeded, want error")
	}

	// 旧关系保持 pills[0],新关系不写入
	rows := listAgentPillRows(t, agent.ID)
	if len(rows) != 1 || rows[0].PillID != pills[0].ID {
		t.Fatalf("rows after rollback = %+v, want only pill %d preserved", rows, pills[0].ID)
	}
	// 缓存同事务:回滚后仍保持有效,不被误失效
	var pattern model.LanguagePattern
	if qerr := DB.Where("agent_id = ?", agent.ID).First(&pattern).Error; qerr != nil {
		t.Fatalf("load language pattern: %v", qerr)
	}
	if !pattern.IsValid {
		t.Fatal("language pattern invalidated despite rollback, want still valid")
	}
}

func TestReplaceAgentPillsRejectsDuplicatePillWithoutPartialWrite(t *testing.T) {
	dao := NewAgentDao()
	newAgentDAOTestDB(t)
	agent, pills := seedAgentWithPills(t, 2)
	// 旧关系: pills[0]
	if err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[0].ID, Weight: 1},
	}); err != nil {
		t.Fatalf("seed ReplaceAgentPills error = %v", err)
	}

	// 重复 pill id:应被拒绝,且不产生部分写入
	err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[1].ID, Weight: 1},
		{PillID: pills[1].ID, Weight: 2},
	})
	if err == nil {
		t.Fatal("ReplaceAgentPills with duplicate pill succeeded, want error")
	}

	// 旧关系保持 pills[0],未写入任何新关系
	rows := listAgentPillRows(t, agent.ID)
	if len(rows) != 1 || rows[0].PillID != pills[0].ID {
		t.Fatalf("rows after duplicate reject = %+v, want only pill %d preserved", rows, pills[0].ID)
	}
}

func TestReplaceAgentPillsRollsBackOnInsertFailure(t *testing.T) {
	dao := NewAgentDao()
	newAgentDAOTestDB(t)
	agent, pills := seedAgentWithPills(t, 2)
	// 旧关系: pills[0]
	if err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[0].ID, Weight: 1},
	}); err != nil {
		t.Fatalf("seed ReplaceAgentPills error = %v", err)
	}

	// 触发器强制 agent_pills 插入失败,验证删除旧关系也被回滚
	trigger := `
CREATE TRIGGER fail_agent_pill_insert
BEFORE INSERT ON agent_pills
BEGIN
  SELECT RAISE(ABORT, 'forced agent_pill insert failure');
END`
	if err := DB.Exec(trigger).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := dao.ReplaceAgentPills(context.Background(), agent.ID, []idao.AgentPillInput{
		{PillID: pills[1].ID, Weight: 2},
	})
	if err == nil {
		t.Fatal("ReplaceAgentPills with failing insert succeeded, want error")
	}

	// 删除旧关系也随事务回滚,pills[0] 保留
	rows := listAgentPillRows(t, agent.ID)
	if len(rows) != 1 || rows[0].PillID != pills[0].ID {
		t.Fatalf("rows after insert-failure rollback = %+v, want only pill %d preserved", rows, pills[0].ID)
	}
}
