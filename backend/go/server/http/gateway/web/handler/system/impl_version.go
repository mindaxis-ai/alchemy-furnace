package system

import (
	"github.com/alchemy-furnace/server/internal/buildinfo"
	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// VersionResponse 版本信息响应 DTO(全模式: serve + desktop)
type VersionResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	Mode      string `json:"mode"`
}

// GetVersion GET /api/v1/version
// 全模式可用: 读 ldflags 注入的 buildinfo + 运行时 Mode
func (cls *System) GetVersion(c *gin.Context) (response.Code, any, error) {
	return response.Ok, &VersionResponse{
		Version:   buildinfo.Version,
		Commit:    buildinfo.Commit,
		BuildDate: buildinfo.BuildDate,
		Mode:      configuration.Mode(),
	}, nil
}
