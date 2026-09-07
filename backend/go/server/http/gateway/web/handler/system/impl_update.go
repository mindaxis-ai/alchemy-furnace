// impl_update.go - desktop 专用更新端点(任务 11)
// 三个端点: GET /api/v1/update/check / POST /api/v1/update/apply / GET /api/v1/update/progress
// 防: dev 构建(UpdateRepo 空) → ErrUpdateDisabled 信封返回;进度用 atomic.Int32 跨请求共享
package system

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/alchemy-furnace/server/internal/buildinfo"
	internalerrors "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/updater"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// updateProgress 包内全局进度(0..100 下载中;110 待重启;负数错误)
// atomic.Int32 保证跨请求并发安全
var updateProgress atomic.Int32

// CheckUpdateResponse 检查更新响应 DTO
type CheckUpdateResponse struct {
	Enabled       bool   `json:"enabled"`
	HasUpdate     bool   `json:"has_update"`
	LatestVersion string `json:"latest_version"`
	CurrentVer    string `json:"current_version"`
	Notes         string `json:"notes"`
	PageURL       string `json:"page_url"`
	AssetName     string `json:"asset_name"`
	AssetSize     int64  `json:"asset_size"`
}

// CheckUpdate GET /api/v1/update/check
// 拉最新 release → semver 比对 → 返回 has_update 与 asset 元数据
func (cls *System) CheckUpdate(c *gin.Context) (response.Code, any, error) {
	rel, err := updater.CheckLatest(c.Request.Context())
	if err != nil {
		if errors.Is(err, updater.ErrUpdateDisabled) {
			return response.Ok, &CheckUpdateResponse{
				Enabled:   false,
				HasUpdate: false,
				Notes:     "开发构建未启用更新",
			}, nil
		}
		return response.ServerInternalError, nil, err
	}

	current := buildinfo.Version
	has := updater.IsNewer(rel.Version, current)
	asset := updater.SelectAsset(rel, runtime.GOOS, runtime.GOARCH)
	resp := &CheckUpdateResponse{
		Enabled:       true,
		HasUpdate:     has,
		LatestVersion: rel.Version,
		CurrentVer:    current,
		Notes:         rel.Notes,
		PageURL:       rel.PageURL,
	}
	if asset != nil {
		resp.AssetName = asset.Name
		resp.AssetSize = asset.Size
	}
	return response.Ok, resp, nil
}

// ApplyUpdate POST /api/v1/update/apply
// 流程: CheckLatest → SelectAsset → Download(进度写 atomic) → 下载 SHA256 → VerifyChecksums → ApplyAndRestart
// 每步失败清理临时文件并回错误信封
func (cls *System) ApplyUpdate(c *gin.Context) (response.Code, any, error) {
	rel, err := updater.CheckLatest(c.Request.Context())
	if err != nil {
		return response.ServerInternalError, nil, err
	}
	asset := updater.SelectAsset(rel, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		return response.ServerInternalError, nil, errors.New("无匹配当前平台的资产")
	}
	current := buildinfo.Version
	if !updater.IsNewer(rel.Version, current) {
		return response.Ok, gin.H{"message": "已是最新版本,无需更新"}, nil
	}
	// macOS App Translocation 等安装位置问题必须在下载前拦截；否则数十 MB
	// 资源下载完成后仍会在交换应用时失败，前端过去只能看到“内部错误”。
	if err := updater.ValidateUpdateTarget(); err != nil {
		return updateFailure("preflight", err)
	}

	// 临时目录: os.TempDir()/alchemy-update-<ts>
	tmpDir, err := os.MkdirTemp("", "alchemy-update-*")
	if err != nil {
		return updateFailure("create_temp_dir", err)
	}
	assetPath := filepath.Join(tmpDir, asset.Name)
	updateProgress.Store(0)
	progressFn := func(pct int) {
		updateProgress.Store(int32(pct))
	}
	defer func() {
		// 失败清理临时目录(成功路径不清理,apply 进程要读)
		if updateProgress.Load() < 100 {
			os.RemoveAll(tmpDir)
		}
	}()

	// 1. 下载资产
	if err := updater.Download(c.Request.Context(), *asset, assetPath, progressFn); err != nil {
		return updateFailure("download", err)
	}

	// 2. SHA256 校验(GitHub release 资产旁通常有 <name>.sha256 单行)
	shaURL := asset.URL[:len(asset.URL)-len(filepath.Ext(asset.Name))] + ".sha256"
	if body, err := fetchOptional(c.Request.Context(), shaURL); err == nil && len(body) > 0 {
		// 容忍两种格式: <hex> <filename>(单行) 或多行 sha256sum
		if err := updater.VerifyChecksums(assetPath, asset.Name, bytes.NewReader(body)); err != nil {
			return updateFailure("verify_checksum", err)
		}
	}

	updateProgress.Store(110) // 110 = 待重启

	// 3. Apply: 主进程会 os.Exit(0),后续不返回
	if err := updater.ApplyAndRestart(c.Request.Context(), assetPath); err != nil {
		return updateFailure("apply", err)
	}
	return response.Ok, gin.H{"message": "更新已启动,进程即将退出"}, nil
}

// updateFailure 统一记录更新失败的阶段。安装位置错误属于用户可修复的 4xx，
// 其他错误仍按 5xx 隐藏内部细节，但可从桌面诊断日志定位阶段和原始原因。
func updateFailure(stage string, err error) (response.Code, any, error) {
	updateProgress.Store(-1)
	zap.L().Warn("[炼丹炉] 自动更新失败", zap.String("stage", stage), zap.Error(err))
	if errors.Is(err, updater.ErrAppTranslocated) {
		return response.InvalidParams, nil, internalerrors.New(
			internalerrors.ErrorTypeInvalidRequest,
			"handler.system.update.app_translocated",
			"当前炼丹炉未安装到“应用程序”目录，无法自动更新。请退出应用，将“炼丹炉.app”拖入“应用程序”后重新打开，再检查更新。",
		)
	}
	return response.ServerInternalError, nil, err
}

// ProgressUpdate GET /api/v1/update/progress
// 0..100 下载中; 110 待重启; 负数错误
func (cls *System) ProgressUpdate(c *gin.Context) (response.Code, any, error) {
	p := updateProgress.Load()
	return response.Ok, gin.H{"progress": p}, nil
}

// fetchOptional GET 一个 URL,失败返回空 body(用于 sha256 旁路文件)
func fetchOptional(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("not found")
	}
	return io.ReadAll(resp.Body)
}
