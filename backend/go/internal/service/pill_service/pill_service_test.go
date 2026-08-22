package pill_service

import (
	"context"
	"reflect"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// ---------- fake dao.Pill(只实现用到的;其他方法 panic 表示意外调用) ----------

// fakePillDao 的 Take 返回浅拷贝,与存储种子共享嵌套 JSON 引用——
// 这样 ClonePill 若未深复制,对副本的篡改会直接污染种子,测试才能咬住。
type fakePillDao struct {
	pills           map[string]*model.ElixirPill
	nextID          uint
	saveErr         errors.Error
	saveCalls       int
	updateCalls     int
	deleteCalls     int
	invalidateCalls int
	invalidatedIDs  []uint
}

func (f *fakePillDao) TakePillByUUID(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error) {
	if p, ok := f.pills[uid.String()]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, errors.ErrorRecordNotFound("test.fake.take_pill")
}
func (f *fakePillDao) FindPillsByUUIDs(ctx context.Context, uids []uuid.UUID) ([]*model.ElixirPill, errors.Error) {
	panic("unused")
}
func (f *fakePillDao) FindPills(ctx context.Context, page, size int, keyword string, isBuiltin *bool) (int64, []*model.ElixirPill, errors.Error) {
	panic("unused")
}
func (f *fakePillDao) SavePill(ctx context.Context, pill *model.ElixirPill) errors.Error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saveCalls++
	f.nextID++
	pill.ID = f.nextID
	if pill.UUID == uuid.Nil {
		pill.UUID = uuid.New()
	}
	f.pills[pill.UUID.String()] = pill
	return nil
}
func (f *fakePillDao) UpdatePill(ctx context.Context, pill *model.ElixirPill, updates map[string]any) errors.Error {
	f.updateCalls++
	stored := f.pills[pill.UUID.String()]
	for k, v := range updates {
		switch k {
		case "name":
			stored.Name = v.(string)
		case "description":
			stored.Description = v.(string)
		case "skill_schema":
			stored.SkillSchema = v.(model.JSONMap)
		case "tags":
			stored.Tags = v.(model.JSONList)
		case "author":
			stored.Author = v.(string)
		case "version":
			stored.Version = v.(string)
		}
	}
	return nil
}
func (f *fakePillDao) DeletePill(ctx context.Context, pill *model.ElixirPill) errors.Error {
	f.deleteCalls++
	delete(f.pills, pill.UUID.String())
	return nil
}
func (f *fakePillDao) FindAgentIDsByPillID(ctx context.Context, pillID uint) ([]uint, errors.Error) {
	return []uint{7}, nil
}
func (f *fakePillDao) InvalidateLanguagePatternsByAgentIDs(ctx context.Context, agentIDs []uint) errors.Error {
	f.invalidateCalls++
	f.invalidatedIDs = agentIDs
	return nil
}

func newPillSvc() (*Pill, *fakePillDao) {
	fake := &fakePillDao{pills: map[string]*model.ElixirPill{}}
	return New(fake), fake
}

func seedPill(f *fakePillDao, builtin bool) *model.ElixirPill {
	pill := &model.ElixirPill{
		ID:          42,
		UUID:        uuid.New(),
		Name:        "丹心妙语",
		Description: "温润如茶的表达风格",
		SkillSchema: model.JSONMap{
			"expression_dna": map[string]interface{}{"tone": "温润", "pace": "舒缓"},
			"mental_models":  []interface{}{"阴阳转化"},
			"future_unknown": map[string]interface{}{"nested": []interface{}{"甲", "乙"}},
		},
		Tags:      model.JSONList{"古风", "炼丹"},
		Author:    "太上老君",
		Version:   "2.1.0",
		IsBuiltin: builtin,
	}
	f.pills[pill.UUID.String()] = pill
	return pill
}

func TestBuiltinPillUpdateRejected(t *testing.T) {
	svc, fake := newPillSvc()
	builtin := seedPill(fake, true)
	name := "新名字"

	_, err := svc.UpdatePill(context.Background(), builtin.UUID, &name, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("内置金丹 UpdatePill 应被拒绝")
	}
	if err.GetCode() != "service.pill.builtin_readonly" {
		t.Fatalf("错误码 = %q, want service.pill.builtin_readonly", err.GetCode())
	}
	if fake.updateCalls != 0 {
		t.Fatalf("内置金丹拒绝后仍调用了 DAO UpdatePill %d 次", fake.updateCalls)
	}
	if fake.pills[builtin.UUID.String()].Name != "丹心妙语" {
		t.Fatal("内置金丹名称被改动")
	}
}

func TestBuiltinPillDeleteRejected(t *testing.T) {
	svc, fake := newPillSvc()
	builtin := seedPill(fake, true)

	err := svc.DeletePill(context.Background(), builtin.UUID)
	if err == nil {
		t.Fatal("内置金丹 DeletePill 应被拒绝")
	}
	if err.GetCode() != "service.pill.builtin_readonly" {
		t.Fatalf("错误码 = %q, want service.pill.builtin_readonly", err.GetCode())
	}
	if fake.deleteCalls != 0 {
		t.Fatalf("内置金丹拒绝后仍调用了 DAO DeletePill %d 次", fake.deleteCalls)
	}
	if _, ok := fake.pills[builtin.UUID.String()]; !ok {
		t.Fatal("内置金丹被删除")
	}
}

func TestClonePillDeepCopiesSchemaAndMetadata(t *testing.T) {
	svc, fake := newPillSvc()
	builtin := seedPill(fake, true)

	clone, err := svc.ClonePill(context.Background(), builtin.UUID)
	if err != nil {
		t.Fatalf("ClonePill() error = %v", err)
	}
	if clone == builtin {
		t.Fatal("副本与原丹是同一指针")
	}
	if clone.UUID == uuid.Nil || clone.UUID == builtin.UUID {
		t.Fatalf("副本 UUID 必须新生成且不同于原丹: %s", clone.UUID)
	}
	if clone.IsBuiltin {
		t.Fatal("副本 is_builtin 必须为 false")
	}
	if clone.Name != builtin.Name+" 副本" {
		t.Fatalf("副本名称 = %q, want %q", clone.Name, builtin.Name+" 副本")
	}
	if clone.Description != builtin.Description || clone.Author != builtin.Author || clone.Version != builtin.Version {
		t.Fatalf("副本元数据未完整复制: %+v", clone)
	}
	if !reflect.DeepEqual(clone.SkillSchema, builtin.SkillSchema) {
		t.Fatalf("副本 schema 内容不等: %+v", clone.SkillSchema)
	}
	if !reflect.DeepEqual(clone.Tags, builtin.Tags) {
		t.Fatalf("副本 tags 内容不等: %+v", clone.Tags)
	}
	if fake.saveCalls != 1 {
		t.Fatalf("ClonePill 应恰好保存一次, 实际 %d 次", fake.saveCalls)
	}

	// 篡改副本嵌套 JSON,原丹(种子)不得受影响——fake Take 共享嵌套引用,未深复制必暴露
	clone.SkillSchema["expression_dna"].(map[string]interface{})["tone"] = "篡改"
	clone.SkillSchema["future_unknown"].(map[string]interface{})["nested"].([]interface{})[0] = "篡改"
	clone.Tags[0] = "篡改"

	stored := fake.pills[builtin.UUID.String()]
	if got := stored.SkillSchema["expression_dna"].(map[string]interface{})["tone"]; got != "温润" {
		t.Fatalf("篡改副本污染了原丹 expression_dna.tone = %v", got)
	}
	if got := stored.SkillSchema["future_unknown"].(map[string]interface{})["nested"].([]interface{})[0]; got != "甲" {
		t.Fatalf("篡改副本污染了原丹未知嵌套字段 = %v", got)
	}
	if got := stored.Tags[0]; got != "古风" {
		t.Fatalf("篡改副本污染了原丹 tags[0] = %v", got)
	}
	if !stored.IsBuiltin {
		t.Fatal("原丹 is_builtin 被改动")
	}
}

func TestClonePillNotFound(t *testing.T) {
	svc, fake := newPillSvc()

	if _, err := svc.ClonePill(context.Background(), uuid.New()); err == nil {
		t.Fatal("克隆不存在金丹应报错")
	} else if !errors.IsType(err, errors.ErrorTypeRecordNotFound) {
		t.Fatalf("错误类型 = %v, want RecordNotFound", err)
	}
	if fake.saveCalls != 0 {
		t.Fatal("克隆失败不应产生保存")
	}
}

func TestClonePillSaveFailureLeavesOriginalUntouched(t *testing.T) {
	svc, fake := newPillSvc()
	builtin := seedPill(fake, true)
	fake.saveErr = errors.ErrorServerInternalError("test.fake.save")

	if _, err := svc.ClonePill(context.Background(), builtin.UUID); err == nil {
		t.Fatal("保存失败应返回错误")
	}
	stored := fake.pills[builtin.UUID.String()]
	if stored.Name != "丹心妙语" || !stored.IsBuiltin {
		t.Fatalf("克隆失败改动了原丹: %+v", stored)
	}
	if len(fake.pills) != 1 {
		t.Fatalf("克隆失败遗留了半成品, pills = %d", len(fake.pills))
	}
}

func TestCustomPillUpdateDeleteUnaffected(t *testing.T) {
	svc, fake := newPillSvc()
	custom := seedPill(fake, false)
	name := "自定义新名"
	desc := "新描述"

	updated, err := svc.UpdatePill(context.Background(), custom.UUID, &name, &desc, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("自定义金丹 UpdatePill() error = %v", err)
	}
	if updated.Name != "自定义新名" || updated.Description != "新描述" {
		t.Fatalf("更新未生效: %+v", updated)
	}
	if fake.updateCalls != 1 {
		t.Fatalf("DAO UpdatePill 调用 %d 次, want 1", fake.updateCalls)
	}
	if fake.invalidateCalls != 1 {
		t.Fatalf("更新后缓存失效调用 %d 次, want 1", fake.invalidateCalls)
	}

	if err := svc.DeletePill(context.Background(), custom.UUID); err != nil {
		t.Fatalf("自定义金丹 DeletePill() error = %v", err)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("DAO DeletePill 调用 %d 次, want 1", fake.deleteCalls)
	}
	if _, ok := fake.pills[custom.UUID.String()]; ok {
		t.Fatal("自定义金丹删除后仍在库中")
	}
}
