// Package updater: GitHub Releases 自动更新核心
//
// 流程: CheckLatest → IsNewer(过滤 prerelease) → SelectAsset → Download + VerifyChecksums
// 防: SHA256 不匹配 / dev 构建(UpdateRepo 空) / 非法 semver / 缺 asset
// 入口: 任务 11 的 HTTP 端点(/api/v1/update/...) 消费本包
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/alchemy-furnace/server/internal/buildinfo"
)

// ErrUpdateDisabled dev 构建(无 UpdateRepo)不启用更新
var ErrUpdateDisabled = errors.New("updater: 构建未配置 UpdateRepo,更新已禁用")

// ReleaseInfo GitHub Release 简化模型(仅保留任务 11 需要的字段)
type ReleaseInfo struct {
	Version string  `json:"version"`  // e.g. "v0.2.0"
	Notes   string  `json:"notes"`    // release body(markdown)
	PageURL string  `json:"page_url"` // html_url
	Assets  []Asset `json:"assets"`
}

// Asset 单个下载资产
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// CheckLatest 拉取最新 release
//
// https://api.github.com/repos/<UpdateRepo>/releases/latest
// 字段映射: tag_name→Version, body→Notes, html_url→PageURL, assets[]→Assets
func CheckLatest(ctx context.Context) (*ReleaseInfo, error) {
	repo := buildinfo.UpdateRepo
	if repo == "" {
		return nil, ErrUpdateDisabled
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "AlchemyFurnace-Desktop/"+buildinfo.Version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("updater: 请求 GitHub 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("updater: 仓库 %s 尚无 release", repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updater: GitHub 返回 HTTP %d", resp.StatusCode)
	}

	// GitHub 原生字段名(避免依赖 1:1 转换)
	var raw struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("updater: 解析 release JSON 失败: %w", err)
	}

	out := &ReleaseInfo{Version: raw.TagName, Notes: raw.Body, PageURL: raw.HTMLURL}
	for _, a := range raw.Assets {
		out.Assets = append(out.Assets, Asset{Name: a.Name, URL: a.BrowserDownloadURL, Size: a.Size})
	}
	return out, nil
}

// IsNewer latest 是否比 current 新(过滤 prerelease / 非法版本 / 空)
//
// semver 要求版本前有 v(自动补);含 pre/- 的版本视为 prerelease,不推
func IsNewer(latest, current string) bool {
	normalize := func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		if !strings.HasPrefix(v, "v") {
			v = "v" + v
		}
		if !semver.IsValid(v) {
			return ""
		}
		return v
	}
	l, c := normalize(latest), normalize(current)
	if l == "" || c == "" {
		return false
	}
	// prerelease 不推(版本号中带 -)
	if semver.Prerelease(l) != "" {
		return false
	}
	return semver.Compare(l, c) > 0
}

// SelectAsset 根据平台选资产
//
// 命名约定(任务 12 CI 产出):
//   - darwin/arm64 → AlchemyFurnace-mac-arm64.zip
//   - darwin/amd64 → AlchemyFurnace-mac-x64.zip
//   - windows/amd64 → AlchemyFurnace-Setup.exe
//
// 不支持平台 → nil
func SelectAsset(r *ReleaseInfo, goos, goarch string) *Asset {
	if r == nil {
		return nil
	}
	want := ""
	switch {
	case goos == "darwin" && goarch == "arm64":
		want = "AlchemyFurnace-mac-arm64.zip"
	case goos == "darwin" && (goarch == "amd64" || goarch == "x86_64"):
		want = "AlchemyFurnace-mac-x64.zip"
	case goos == "windows" && (goarch == "amd64" || goarch == "x86_64"):
		want = "AlchemyFurnace-Setup.exe"
	}
	if want == "" {
		return nil
	}
	for i := range r.Assets {
		if r.Assets[i].Name == want {
			return &r.Assets[i]
		}
	}
	return nil
}

// Download 流式下载到 dst(边下边落盘,避免大文件占内存)
//
// progress: 0..100 的回调(已下载字节 / 总字节),可为 nil
// 支持 ctx 取消;出错自动清理半成品
func Download(ctx context.Context, a Asset, dst string, progress func(pct int)) error {
	if a.URL == "" {
		return fmt.Errorf("updater: 资产 URL 为空")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("updater: 下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("updater: 下载返回 HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	// 失败时清理半成品
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(dst)
		}
	}()

	total := resp.ContentLength
	if total < 0 {
		total = a.Size
	}
	var written int64
	buf := make([]byte, 64*1024)
	lastPct := -1
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("updater: 写文件失败: %w", werr)
			}
			written += int64(n)
			if progress != nil && total > 0 {
				pct := int(written * 100 / total)
				if pct != lastPct {
					lastPct = pct
					progress(pct)
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("updater: 读取响应失败: %w", rerr)
		}
	}
	if progress != nil && total > 0 && lastPct < 100 {
		progress(100)
	}
	success = true
	return nil
}

// VerifyChecksums 校验 assetPath 的 SHA256 是否与 checksumsBody 中 name 行匹配
//
// 格式(sha256sum 标准):
//
//	<hex64>  <name>
//	<hex64> *<name>      # 二进制模式标记,容忍
//
// name 可包含 ./ 前缀(去除后比较)
func VerifyChecksums(assetPath, assetName string, checksumsBody io.Reader) error {
	f, err := os.Open(assetPath)
	if err != nil {
		return fmt.Errorf("updater: 打开下载文件失败: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("updater: 读下载文件失败: %w", err)
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))

	body, err := io.ReadAll(checksumsBody)
	if err != nil {
		return fmt.Errorf("updater: 读 checksums 失败: %w", err)
	}

	wantName := filepath.Base(assetName)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 容忍二进制模式前缀 "*"
		line = strings.TrimPrefix(line, "*")
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		sum, name := fields[0], fields[1]
		if filepath.Base(name) != wantName {
			continue
		}
		if !strings.EqualFold(sum, actual) {
			return fmt.Errorf("updater: SHA256 不匹配: want=%s got=%s", sum, actual)
		}
		return nil
	}
	return fmt.Errorf("updater: checksums 中未找到 %s", wantName)
}
