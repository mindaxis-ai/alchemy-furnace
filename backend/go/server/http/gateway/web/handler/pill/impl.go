// Package pill 金丹管理 HTTP 处理器(新网关;对齐 Luna-CY 模板 handler 分包风格)
// 路由: /api/v1/pills;路径参数 :uuid 为对外唯一标识
package pill

import (
	"time"

	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
)

// Pill 金丹处理器
type Pill struct {
	pill service.Pill
}

// New 构造金丹处理器
func New(pill service.Pill) *Pill {
	return &Pill{pill: pill}
}

// Response 金丹响应 DTO:id 输出 UUID 字符串,不泄露数字主键
type Response struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	SkillSchema model.JSONMap  `json:"skill_schema"`
	Tags        model.JSONList `json:"tags"`
	Author      string         `json:"author"`
	Version     string         `json:"version"`
	IsBuiltin   bool           `json:"is_builtin"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// ToResponse 内部模型 → 对外 DTO(导出供 agent 详情等跨包嵌入)
func ToResponse(p *model.ElixirPill) *Response {
	return &Response{
		ID:          p.ElixirPillID,
		Name:        p.Name,
		Description: p.Description,
		SkillSchema: p.SkillSchema,
		Tags:        p.Tags,
		Author:      p.Author,
		Version:     p.Version,
		IsBuiltin:   p.IsBuiltin,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// toResponseList 批量转换
func toResponseList(pills []*model.ElixirPill) []*Response {
	list := make([]*Response, 0, len(pills))
	for _, p := range pills {
		list = append(list, ToResponse(p))
	}
	return list
}
