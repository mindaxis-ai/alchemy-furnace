package distillation_service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/distillation"
	appErrors "github.com/alchemy-furnace/server/internal/errors"
	service "github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

type fakeClient struct {
	called bool
	result *distillation.Response
	err    error

	exportCalled bool
	exportResult *distillation.ExportResult
	exportErr    error
	exportSkill  *distillation.ExportableSkill
	exportFormat string
}

func (f *fakeClient) Distill(_ context.Context, _, _, _ string, _ *credential.ModelCredentials) (*distillation.Response, error) {
	f.called = true
	return f.result, f.err
}

func (f *fakeClient) SkillExport(_ context.Context, skill *distillation.ExportableSkill, format string) (*distillation.ExportResult, error) {
	f.exportCalled = true
	f.exportSkill = skill
	f.exportFormat = format
	return f.exportResult, f.exportErr
}

type fakeResolver struct {
	credentials *credential.ModelCredentials
	err         error
}

func (f fakeResolver) ResolveCredentials(context.Context, string) (*credential.ModelCredentials, error) {
	return f.credentials, f.err
}

func (f fakeResolver) ResolveSynthesisCredentials(context.Context) (*credential.ModelCredentials, error) {
	return f.credentials, f.err
}

func (f fakeResolver) ResolveFusionCredentials(context.Context) (*credential.ModelCredentials, error) {
	return f.credentials, f.err
}

func TestDistillValidatesBeforeCallingDependencies(t *testing.T) {
	client := &fakeClient{}
	service := New(client, fakeResolver{}, &fakeInventory{})

	result, appErr := service.Distill(context.Background(), "A", "短", "zh-CN")

	if result != nil || appErr == nil {
		t.Fatalf("result = %#v, error = %#v; want validation error", result, appErr)
	}
	if client.called {
		t.Fatal("client was called for invalid input")
	}
}

func TestDistillPassesConfiguredCredentials(t *testing.T) {
	want := &distillation.Response{Name: "结构化金丹"}
	client := &fakeClient{result: want}
	resolver := fakeResolver{credentials: &credential.ModelCredentials{Model: "configured-model", APIKey: "secret"}}
	service := New(client, resolver, &fakeInventory{})

	got, appErr := service.Distill(context.Background(), "人物主题", "提炼公开资料中的决策方式", "en")

	if appErr != nil || got != want {
		t.Fatalf("got = %#v, error = %#v", got, appErr)
	}
	if !client.called {
		t.Fatal("client was not called")
	}
}

func TestDistillReturnsCredentialFailureWithoutCallingClient(t *testing.T) {
	client := &fakeClient{}
	service := New(client, fakeResolver{err: errors.New("未配置模型")}, &fakeInventory{})

	result, appErr := service.Distill(context.Background(), "人物主题", "提炼公开资料中的决策方式", "zh-CN")

	if result != nil || appErr == nil || client.called {
		t.Fatalf("result = %#v, error = %#v, called = %v", result, appErr, client.called)
	}
}

func TestDistillMapsRemoteServiceUnavailableWithCodeAndData(t *testing.T) {
	remote := &distillation.RemoteError{
		Status:    http.StatusServiceUnavailable,
		Code:      "research_search_blocked",
		Stage:     "research",
		Message:   "搜索受限",
		Retryable: true,
		Details:   map[string]any{"documents": 0},
	}
	service := New(&fakeClient{err: remote}, fakeResolver{credentials: &credential.ModelCredentials{}}, &fakeInventory{})

	_, appErr := service.Distill(context.Background(), "人物主题", "提炼公开资料中的决策方式", "zh-CN")

	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeServiceUnavailable) {
		t.Fatalf("error = %#v, want ErrorTypeServiceUnavailable", appErr)
	}
	if appErr.GetCode() != "research_search_blocked" {
		t.Fatalf("code = %q, want research_search_blocked", appErr.GetCode())
	}
	if appErr.Error() != "搜索受限" {
		t.Fatalf("message = %q, want 搜索受限", appErr.Error())
	}
	withData, ok := appErr.(appErrors.ErrorWithData)
	if !ok {
		t.Fatal("expected ErrorWithData to carry stage/retryable/details")
	}
	payload, ok := withData.GetData().(map[string]any)
	if !ok || payload["stage"] != "research" || payload["retryable"] != true {
		t.Fatalf("data = %#v, want {stage:research retryable:true}", withData.GetData())
	}
	details, ok := payload["details"].(map[string]any)
	if !ok || details["documents"] != 0 {
		t.Fatalf("details = %#v, want {documents:0} 原样透传", payload["details"])
	}
}

func TestDistillMapsRemoteInvalidRequestTo400WithPublicMessage(t *testing.T) {
	remote := &distillation.RemoteError{
		Status:  http.StatusBadRequest,
		Message: "未配置可用于智能炼制的模型，请先到设置中配置模型供应商",
	}
	service := New(&fakeClient{err: remote}, fakeResolver{credentials: &credential.ModelCredentials{}}, &fakeInventory{})

	_, appErr := service.Distill(context.Background(), "人物主题", "提炼公开资料中的决策方式", "zh-CN")

	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeInvalidRequest) {
		t.Fatalf("error = %#v, want ErrorTypeInvalidRequest", appErr)
	}
	if appErr.Error() != "未配置可用于智能炼制的模型，请先到设置中配置模型供应商" {
		t.Fatalf("message = %q, want 公开 message 保留", appErr.Error())
	}
}

func validExportSkill() *distillation.ExportableSkill {
	return &distillation.ExportableSkill{
		Name:        "结构化金丹",
		Description: "一份结构化的语言风格技能包",
		SkillSchema: map[string]interface{}{"identity_card": "我是金丹"},
		Tags:        []string{"语言"},
		GeneratedAt: "2026-08-27T10:00:00Z",
	}
}

func TestSkillExport_RejectsInvalidFormatBeforeCallingClient(t *testing.T) {
	client := &fakeClient{}
	service := New(client, fakeResolver{}, &fakeInventory{})

	_, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		Skill:  validExportSkill(),
		Format: "yaml",
	})

	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeInvalidRequest) {
		t.Fatalf("error = %#v, want ErrorTypeInvalidRequest", appErr)
	}
	if client.exportCalled {
		t.Fatal("client was called for invalid format")
	}
}

func TestSkillExport_RequiresExactlyOneTarget(t *testing.T) {
	client := &fakeClient{}
	service := New(client, fakeResolver{}, &fakeInventory{})

	for _, input := range []*distillation.SkillExportInput{
		{Format: "codex"},
		{RevisionID: "550e8400-e29b-41d4-a716-446655440002", Format: "codex"},
		{RecipeID: "550e8400-e29b-41d4-a716-446655440001", Skill: validExportSkill(), Format: "codex"},
	} {
		_, appErr := service.SkillExport(context.Background(), input)
		if appErr == nil || !appErr.IsType(appErrors.ErrorTypeInvalidRequest) {
			t.Fatalf("input = %+v, error = %#v, want ErrorTypeInvalidRequest", input, appErr)
		}
	}
	if client.exportCalled {
		t.Fatal("client was called without a valid single target")
	}
}

func TestSkillExport_RejectsInvalidStructuredContent(t *testing.T) {
	client := &fakeClient{}
	service := New(client, fakeResolver{}, &fakeInventory{})
	skill := validExportSkill()
	skill.Description = "" // 空描述: 服务端重校验拦截

	_, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		Skill:  skill,
		Format: "codex",
	})

	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeInvalidRequest) {
		t.Fatalf("error = %#v, want ErrorTypeInvalidRequest", appErr)
	}
	if appErr.GetCode() != "service.skill_export.invalid" {
		t.Fatalf("code = %q, want service.skill_export.invalid", appErr.GetCode())
	}
	if client.exportCalled {
		t.Fatal("client was called for invalid content")
	}
}

func TestSkillExport_StructuredSuccessPassesSkillAndFormat(t *testing.T) {
	client := &fakeClient{exportResult: &distillation.ExportResult{Filename: "alchemy-skill-x-codex.zip", Content: []byte("PK")}}
	service := New(client, fakeResolver{}, &fakeInventory{})

	result, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		Skill:  validExportSkill(),
		Format: "codex",
	})

	if appErr != nil || result == nil || string(result.Content) != "PK" {
		t.Fatalf("result = %#v, error = %#v", result, appErr)
	}
	if !client.exportCalled || client.exportFormat != "codex" {
		t.Fatalf("client 未携带 format=codex: called=%v format=%q", client.exportCalled, client.exportFormat)
	}
	if client.exportSkill.Name != "结构化金丹" || client.exportSkill.GeneratedAt != "2026-08-27T10:00:00Z" {
		t.Fatalf("client skill = %+v", client.exportSkill)
	}
}

func TestSkillExport_MapsRemoteUnavailableToRetryableServiceUnavailable(t *testing.T) {
	remote := &distillation.RemoteError{
		Status:    http.StatusServiceUnavailable,
		Code:      "skill_export_unavailable",
		Stage:     "export",
		Message:   "导出服务暂不可用",
		Retryable: true,
		Details:   map[string]any{"reason": "engine down"},
	}
	service := New(&fakeClient{exportErr: remote}, fakeResolver{}, &fakeInventory{})

	_, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		Skill:  validExportSkill(),
		Format: "claude",
	})

	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeServiceUnavailable) {
		t.Fatalf("error = %#v, want ErrorTypeServiceUnavailable", appErr)
	}
	if appErr.GetCode() != "skill_export_unavailable" || appErr.Error() != "导出服务暂不可用" {
		t.Fatalf("code/message = %q/%q", appErr.GetCode(), appErr.Error())
	}
	withData, ok := appErr.(appErrors.ErrorWithData)
	if !ok {
		t.Fatal("expected ErrorWithData")
	}
	payload := withData.GetData().(map[string]any)
	if payload["stage"] != "export" || payload["retryable"] != true {
		t.Fatalf("data = %#v", withData.GetData())
	}
}

func TestSkillExport_MapsRemoteInvalidTo400WithPublicCode(t *testing.T) {
	remote := &distillation.RemoteError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "skill_export_invalid",
		Stage:   "export",
		Message: "Skill 导出内容无效: name 长度不足",
	}
	service := New(&fakeClient{exportErr: remote}, fakeResolver{}, &fakeInventory{})

	_, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		Skill:  validExportSkill(),
		Format: "codex",
	})

	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeInvalidRequest) {
		t.Fatalf("error = %#v, want ErrorTypeInvalidRequest", appErr)
	}
	if appErr.GetCode() != "skill_export_invalid" {
		t.Fatalf("code = %q, want skill_export_invalid", appErr.GetCode())
	}
}

// fakeInventory 最小 stub：仅实现导出路径所需方法(GetRecipe/GetRecipeRevision)，
// 其余接口方法在本测试不触达。
type fakeInventory struct {
	recipe   *model.PillRecipe
	revision *model.PillRecipeRevision
	otherRev *model.PillRecipeRevision // 属于其他丹方的版本(归属校验用)
}

func (f *fakeInventory) GetRecipe(_ context.Context, uid uuid.UUID) (*model.PillRecipe, *model.PillRecipeRevision, appErrors.Error) {
	if f.recipe == nil || f.recipe.PillRecipeID != uid.String() {
		return nil, nil, appErrors.ErrorRecordNotFound("fake.recipe")
	}
	return f.recipe, f.revision, nil
}

func (f *fakeInventory) GetRecipeRevision(_ context.Context, recipeID, revisionID uuid.UUID) (*model.PillRecipeRevision, appErrors.Error) {
	if f.revision != nil && f.revision.PillRecipeRevisionID == revisionID.String() && recipeID.String() == f.recipe.PillRecipeID {
		return f.revision, nil
	}
	if f.otherRev != nil && f.otherRev.PillRecipeRevisionID == revisionID.String() && recipeID.String() != f.recipe.PillRecipeID {
		return nil, appErrors.ErrorRecordNotFound("fake.revision_not_of_recipe")
	}
	return nil, appErrors.ErrorRecordNotFound("fake.revision")
}

func (f *fakeInventory) SaveRecipe(context.Context, service.SaveRecipeRequest) (*service.PillOperationResult, appErrors.Error) {
	panic("unused")
}
func (f *fakeInventory) CraftOne(context.Context, service.CraftPillRequest) (*service.PillOperationResult, appErrors.Error) {
	panic("unused")
}
func (f *fakeInventory) Consume(context.Context, service.ConsumePillRequest) (*service.PillOperationResult, appErrors.Error) {
	panic("unused")
}
func (f *fakeInventory) ConfirmFusion(context.Context, service.ConfirmFusionRequest) (*service.PillOperationResult, appErrors.Error) {
	panic("unused")
}
func (f *fakeInventory) GetOperation(context.Context, uuid.UUID) (*service.PillOperationResult, appErrors.Error) {
	panic("unused")
}
func (f *fakeInventory) UpdateRecipe(context.Context, service.UpdateRecipeRequest) (*service.PillOperationResult, appErrors.Error) {
	panic("unused")
}
func (f *fakeInventory) ArchiveRecipe(context.Context, service.ArchiveRecipeRequest) appErrors.Error {
	panic("unused")
}
func (f *fakeInventory) DiscardItem(context.Context, service.DiscardItemRequest) appErrors.Error {
	panic("unused")
}
func (f *fakeInventory) ListRecipes(context.Context, int, int, string, bool) (int64, []service.RecipeListItem, map[string]int64, appErrors.Error) {
	panic("unused")
}
func (f *fakeInventory) ListItems(context.Context, int, int, *uuid.UUID) (int64, []service.ItemListItem, appErrors.Error) {
	panic("unused")
}
func (f *fakeInventory) GetItem(context.Context, uuid.UUID) (*service.ItemDetail, appErrors.Error) {
	panic("unused")
}

// recipe 模式投影必须把空来源序列化为 [] 而非 null:
// Go nil slice → JSON null → Python Pydantic sources: List 校验失败 422
// (2026-08-28 桌面验收报错 list_type@body.sources 的回归防线)
func TestSkillExport_RecipeModeNeverSendsNullSources(t *testing.T) {
	client := &fakeClient{}
	recipe, rev := fakeRecipeAndRevision()
	service := New(client, fakeResolver{}, &fakeInventory{recipe: recipe, revision: rev})

	_, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		RecipeID: recipe.PillRecipeID,
		Format:   "codex",
	})
	if appErr != nil {
		t.Fatalf("unexpected error = %#v", appErr)
	}
	if client.exportSkill == nil {
		t.Fatal("client was not called with a projected skill")
	}
	if client.exportSkill.Sources == nil {
		t.Fatal("Sources 为 nil: JSON 序列化为 null 会被 Pydantic 拒绝")
	}
	raw, err := json.Marshal(client.exportSkill)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(raw, []byte(`"sources":null`)) {
		t.Fatalf("投影包含 sources:null: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"sources":[]`)) {
		t.Fatalf("投影应含 sources:[]: %s", raw)
	}
}

// fakeRecipeAndRevision 构造测试用丹方与当前版本(不可变)
func fakeRecipeAndRevision() (*model.PillRecipe, *model.PillRecipeRevision) {
	recipe := &model.PillRecipe{PillRecipeID: "550e8400-e29b-41d4-a716-446655440001"}
	rev := &model.PillRecipeRevision{
		PillRecipeRevisionID: "550e8400-e29b-41d4-a716-446655440010",
		RecipeID:             recipe.PillRecipeID,
		Revision:             1,
		Name:                 "结构化金丹",
		Description:          "一份结构化的语言风格技能包",
		SkillSchema:          model.JSONMap{"identity_card": "我是金丹"},
		Tags:                 model.JSONList{"语言"},
		Base:                 model.Base{CreatedAt: time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)},
	}
	revUID := rev.PillRecipeRevisionID
	recipe.CurrentRevisionID = &revUID
	return recipe, rev
}

// TestSkillExport_RecipeIDExportsCurrentRevision recipe_id 单独 → 导出丹方当前版本内容
func TestSkillExport_RecipeIDExportsCurrentRevision(t *testing.T) {
	client := &fakeClient{exportResult: &distillation.ExportResult{Filename: "alchemy-recipe-codex.zip", Content: []byte("PK")}}
	recipe, rev := fakeRecipeAndRevision()
	service := New(client, fakeResolver{}, &fakeInventory{recipe: recipe, revision: rev})

	result, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		RecipeID: recipe.PillRecipeID,
		Format:   "codex",
	})
	if appErr != nil || result == nil {
		t.Fatalf("result = %#v, error = %#v", result, appErr)
	}
	if !client.exportCalled || client.exportSkill == nil {
		t.Fatal("client 未收到投影 skill")
	}
	got := client.exportSkill
	if got.Name != "结构化金丹" || got.Description != "一份结构化的语言风格技能包" {
		t.Fatalf("投影字段 = %+v", got)
	}
	if got.SkillSchema == nil || got.SkillSchema["identity_card"] != "我是金丹" {
		t.Fatalf("SkillSchema 投影 = %+v", got.SkillSchema)
	}
	if got.GeneratedAt != "2026-08-27T10:00:00Z" {
		t.Fatalf("GeneratedAt = %q, 期望版本创建时间(不可变)", got.GeneratedAt)
	}
	if got.EvidenceLevel != "limited" {
		t.Fatalf("EvidenceLevel = %q, 期望 limited", got.EvidenceLevel)
	}
}

// TestSkillExport_RevisionIDExportsSpecifiedRevision recipe_id+revision_id → 指定版本
func TestSkillExport_RevisionIDExportsSpecifiedRevision(t *testing.T) {
	client := &fakeClient{exportResult: &distillation.ExportResult{Filename: "x.zip", Content: []byte("PK")}}
	recipe, rev := fakeRecipeAndRevision()
	service := New(client, fakeResolver{}, &fakeInventory{recipe: recipe, revision: rev})

	_, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		RecipeID:   recipe.PillRecipeID,
		RevisionID: rev.PillRecipeRevisionID,
		Format:     "codex",
	})
	if appErr != nil {
		t.Fatalf("error = %#v", appErr)
	}
	if !client.exportCalled || client.exportSkill.Name != "结构化金丹" {
		t.Fatalf("client skill = %+v", client.exportSkill)
	}
}

// TestSkillExport_RevisionOfOtherRecipe404 revision 不属于 URL 的丹方 → 404,不调 client
func TestSkillExport_RevisionOfOtherRecipe404(t *testing.T) {
	client := &fakeClient{}
	recipe, rev := fakeRecipeAndRevision()
	other := &model.PillRecipeRevision{
		PillRecipeRevisionID: uuid.New().String(),
		RecipeID:             uuid.New().String(), // 另一个丹方的 UUID 文本
		Revision:             2,
		Name:                 "别人家的丹",
	}
	service := New(client, fakeResolver{}, &fakeInventory{recipe: recipe, revision: rev, otherRev: other})

	_, appErr := service.SkillExport(context.Background(), &distillation.SkillExportInput{
		RecipeID:   recipe.PillRecipeID,
		RevisionID: other.PillRecipeRevisionID,
		Format:     "codex",
	})
	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeRecordNotFound) {
		t.Fatalf("error = %#v, want ErrorTypeRecordNotFound", appErr)
	}
	if client.exportCalled {
		t.Fatal("归属不符不应调用 client")
	}
}

// TestSkillExport_InvalidRecipeIDRejected 非法 recipe_id/revision_id → 400
func TestSkillExport_InvalidRecipeIDRejected(t *testing.T) {
	client := &fakeClient{}
	service := New(client, fakeResolver{}, &fakeInventory{})

	for _, input := range []*distillation.SkillExportInput{
		{RecipeID: "not-a-uuid", Format: "codex"},
		{RecipeID: "550e8400-e29b-41d4-a716-446655440001", RevisionID: "not-a-uuid", Format: "codex"},
	} {
		_, appErr := service.SkillExport(context.Background(), input)
		if appErr == nil || !appErr.IsType(appErrors.ErrorTypeInvalidRequest) {
			t.Fatalf("input = %+v, error = %#v, want ErrorTypeInvalidRequest", input, appErr)
		}
	}
	if client.exportCalled {
		t.Fatal("非法 ID 不应调用 client")
	}
}
