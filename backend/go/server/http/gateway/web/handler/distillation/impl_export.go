// Skill 导出接口: POST /api/v1/distillation/skill-export
// 只读 RAW handler(不经 Wrapper): 成功直接写 ZIP 二进制流 + Content-Disposition;
// 失败写统一 JSON 错误信封。请求只接收丹方版本引用或结构化 skill 数据,
// 绝不接收 API Key —— 携带凭据字段一律 403(服务端权限边界)。
package distillation

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/alchemy-furnace/server/internal/context/contextutil"
	nudist "github.com/alchemy-furnace/server/internal/distillation"
	appErrors "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SkillExportRequest 导出请求: recipe_id(+revision_id) / skill 二选一,format 必填
// 任务 5 起: 导出指定丹方版本(recipe_id 为当前版本,带 revision_id 为指定版本)。
type SkillExportRequest struct {
	RecipeID   string                  `json:"recipe_id"`
	RevisionID string                  `json:"revision_id"`
	Skill      *nudist.ExportableSkill `json:"skill"`
	Format     string                  `json:"format"`
}

// SkillExport 处理导出请求并写出 ZIP 二进制响应
func (h *Handler) SkillExport(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "读取请求体失败")
		return
	}
	// 权限边界: 接口绝不接收 API Key/凭据字段(顶层键扫描,嵌套内容级密钥由字段校验拦截)
	if err := nudist.RejectCredentialFields(raw); err != nil {
		response.FailureWithErrorCode(c, http.StatusForbidden, response.InvalidParams,
			"skill_export_forbidden", "导出接口不接受任何密钥或凭据字段")
		return
	}
	// 绑定需要从 Body 读取,恢复已消费的请求体
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var body SkillExportRequest
	if err := request.ShouldBindJSON(c, &body); err != nil {
		writeExportError(c, err)
		return
	}

	result, serr := h.service.SkillExport(contextutil.NewContextWithGin(c), &nudist.SkillExportInput{
		RecipeID:   body.RecipeID,
		RevisionID: body.RevisionID,
		Skill:      body.Skill,
		Format:     body.Format,
	})
	if serr != nil {
		writeExportError(c, serr)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, result.Filename))
	c.Data(http.StatusOK, "application/zip", result.Content)
}

// writeExportError 与 router.Wrapper 同语义写出错误信封(二进制 handler 不经 Wrapper):
// 5xx 不暴露内部细节,受控 503(远端可重试)保留公开 message,ErrorWithData 附加 data。
func writeExportError(c *gin.Context, err error) {
	status := appErrors.HTTPStatus(err)
	bodyCode := status
	message := err.Error()
	errorCode := ""
	if internal, ok := err.(appErrors.Error); ok {
		errorCode = internal.GetCode()
	}
	if status >= 500 && status != http.StatusServiceUnavailable {
		zap.L().Error("[炼丹炉] 内部错误",
			zap.String("request_id", c.GetString("X-Request-ID")),
			zap.Error(err))
		message = "服务器内部错误"
	}
	if ed, ok := err.(appErrors.ErrorWithData); ok {
		response.FailureWithDataAndErrorCode(c, status, bodyCode, errorCode, message, ed.GetData())
		return
	}
	response.FailureWithErrorCode(c, status, bodyCode, errorCode, message)
}
