package fusion_service

import (
	"context"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// --- fakes ---

// fakePillDao 实现 idao.Pill 全部方法,仅 FindPillsByUUIDs 有实际行为
type fakePillDao struct{ pills map[string]*model.ElixirPill }

func (f *fakePillDao) FindPillsByUUIDs(ctx context.Context, uids []uuid.UUID) ([]*model.ElixirPill, errors.Error) {
	out := make([]*model.ElixirPill, 0, len(uids))
	for _, u := range uids {
		if p, ok := f.pills[u.String()]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakePillDao) TakePillByUUID(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error) {
	return nil, errors.ErrorRecordNotFound("test.fake.take_pill")
}

func (f *fakePillDao) FindPills(ctx context.Context, page int, size int, keyword string, isBuiltin *bool) (int64, []*model.ElixirPill, errors.Error) {
	return 0, nil, nil
}

func (f *fakePillDao) SavePill(ctx context.Context, pill *model.ElixirPill) errors.Error {
	return nil
}

func (f *fakePillDao) UpdatePill(ctx context.Context, pill *model.ElixirPill, updates map[string]any) errors.Error {
	return nil
}

func (f *fakePillDao) DeletePill(ctx context.Context, pill *model.ElixirPill) errors.Error {
	return nil
}

func (f *fakePillDao) FindAgentIDsByPillID(ctx context.Context, pillID uint) ([]uint, errors.Error) {
	return nil, nil
}

func (f *fakePillDao) InvalidateLanguagePatternsByAgentIDs(ctx context.Context, agentIDs []uint) errors.Error {
	return nil
}

// fakeFusionClient 实现 synthesis.FusionClient,记录调用入参
type fakeFusionClient struct {
	called bool
	req    []synthesis.PillInput
}

func (f *fakeFusionClient) Fuse(ctx context.Context, pills []synthesis.PillInput, excludeOperatorID string, creds *credential.ModelCredentials) (*synthesis.FuseResponse, error) {
	f.called = true
	f.req = pills
	return &synthesis.FuseResponse{
		Name: "麻辣禅师", Description: "辣",
		SkillSchema: model.JSONMap{"identity_card": "..."},
		Operator:    synthesis.FuseOperator{ID: "dialectic", Name: "对立调和"},
	}, nil
}

func TestFuseLoadsPillsAndForwards(t *testing.T) {
	uid1, uid2 := uuid.New(), uuid.New()
	d := &fakePillDao{pills: map[string]*model.ElixirPill{
		uid1.String(): {UUID: uid1, Name: "鲁迅风金丹", SkillSchema: model.JSONMap{"identity_card": "医师"}},
		uid2.String(): {UUID: uid2, Name: "禅师金丹", SkillSchema: model.JSONMap{"identity_card": "蒲团"}},
	}}
	client := &fakeFusionClient{}
	svc := New(d, client, nil)

	resp, err := svc.Fuse(context.Background(), []uuid.UUID{uid1, uid2}, "")
	if err != nil {
		t.Fatalf("Fuse failed: %v", err)
	}
	if !client.called || len(client.req) != 2 {
		t.Fatalf("expected client called with 2 pills, got %+v", client.req)
	}
	if resp.Name != "麻辣禅师" || resp.Operator.ID != "dialectic" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
}

func TestFuseRejectsSinglePill(t *testing.T) {
	svc := New(&fakePillDao{}, &fakeFusionClient{}, nil)
	_, err := svc.Fuse(context.Background(), []uuid.UUID{uuid.New()}, "")
	if err == nil {
		t.Fatal("expected error for single pill")
	}
	if !err.IsType(errors.ErrorTypeInvalidRequest) {
		t.Fatalf("expected invalid-request error, got %v", err)
	}
}

func TestFuseMissingPillReturnsNotFound(t *testing.T) {
	svc := New(&fakePillDao{pills: map[string]*model.ElixirPill{}}, &fakeFusionClient{}, nil)
	_, err := svc.Fuse(context.Background(), []uuid.UUID{uuid.New(), uuid.New()}, "")
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !err.IsType(errors.ErrorTypeRecordNotFound) {
		t.Fatalf("expected record-not-found error, got %v", err)
	}
}
