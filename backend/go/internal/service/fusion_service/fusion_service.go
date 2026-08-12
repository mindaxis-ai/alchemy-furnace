// Package fusion_service 金丹融合业务逻辑(新架构 internal 分层)
// 加载金丹 -> 解析凭证 -> 调 Python 融合引擎;不落库。
// 对应 RESTful API: /api/v1/fusion/fuse
package fusion_service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	idao "github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Fusion service.Fusion 接口实现
type Fusion struct {
	pill       idao.Pill
	fusion     synthesis.FusionClient // 接口,便于单测 mock
	credential credential.Resolver
}

// New 构造金丹融合业务实例
func New(pill idao.Pill, fusionClient synthesis.FusionClient, credential credential.Resolver) *Fusion {
	return &Fusion{
		pill:       pill,
		fusion:     fusionClient,
		credential: credential,
	}
}

// Fuse 融合预览:按 UUID 批量加载金丹,转发 Python 融合引擎
// 凭证解析失败时返回 400 硬性错误,引导用户去设置中配置融合专用模型(不静默回退道人默认)
func (s *Fusion) Fuse(ctx context.Context, pillUUIDs []uuid.UUID, excludeOperatorID string) (*synthesis.FuseResponse, errors.Error) {
	if len(pillUUIDs) < 2 {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.fusion.too_few", "融合至少需要 2 枚金丹")
	}

	pills, err := s.pill.FindPillsByUUIDs(ctx, pillUUIDs)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.fusion.load_pills"))
	}
	pillMap := make(map[string]*model.ElixirPill, len(pills))
	for _, p := range pills {
		pillMap[p.UUID.String()] = p
	}

	// 按请求 UUID 顺序组装输入,任一缺失即 404(与 trial.loadTrialPills 对齐)
	inputs := make([]synthesis.PillInput, 0, len(pillUUIDs))
	for _, uid := range pillUUIDs {
		p, ok := pillMap[uid.String()]
		if !ok {
			return nil, errors.New(errors.ErrorTypeRecordNotFound, "service.fusion.pill_missing", "金丹(id=%s)不存在", uid.String())
		}
		inputs = append(inputs, synthesis.PillInput{
			ID:          p.UUID.String(),
			Name:        p.Name,
			SkillSchema: p.SkillSchema,
		})
	}

	// 解析融合专用模型凭证:未配置 is_fusion 时硬性报错(不静默回退道人默认模型)
	// 硬错误比静默 fallback 更安全——避免「道人大模型被融合 prompt 撑爆」类隐式 bug
	var creds *credential.ModelCredentials
	if s.credential != nil {
		c, e := s.credential.ResolveFusionCredentials(ctx)
		if e != nil {
			// credential.Resolver 返回 stdlib error;包装为 errors.Error 以携带中文消息
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.fusion.resolve_fusion", e.Error())
		}
		creds = c
	}

	resp, e := s.fusion.Fuse(ctx, inputs, excludeOperatorID, creds)
	if e != nil {
		return nil, errors.New(errors.ErrorTypeServerInternalError, "service.fusion.fuse", e.Error())
	}
	return resp, nil
}
