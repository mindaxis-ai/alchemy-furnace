// Package engineproc 桌面模式编排内嵌 Python 引擎子进程: 拉起→健康检查→停止
package engineproc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/configuration"
)

// ResolveRuntimeRoot 定位内嵌 Python 运行时根目录
// env ALCHEMY_PYTHON_RUNTIME 优先(开发用本机环境);否则相对可执行文件
func ResolveRuntimeRoot() (string, error) {
	if v := os.Getenv("ALCHEMY_PYTHON_RUNTIME"); v != "" {
		return v, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		// <.app>/Contents/MacOS/<bin> → <.app>/Contents/Resources/python-runtime
		return filepath.Join(filepath.Dir(exe), "..", "Resources", "python-runtime"), nil
	}
	return filepath.Join(filepath.Dir(exe), "runtime"), nil
}

// pythonBin 返回可执行的 Python 解释器路径(跨平台)
func pythonBin(root string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(root, "python.exe")
	}
	return filepath.Join(root, "bin", "python3")
}

// pickPort 让 OS 选一个空闲端口(127.0.0.1:0)
func pickPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// waitHealthy 轮询 /health,200 即就绪;超时返回 error
func waitHealthy(ctx context.Context, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/health", nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("Python 引擎健康检查超时(%s)", timeout)
}

// Start 拉起 uvicorn 并就绪等待;stop 由 Wails OnShutdown 调用(杀进程组)
func Start(ctx context.Context) (baseURL string, stop func(), err error) {
	root, err := ResolveRuntimeRoot()
	if err != nil {
		return "", nil, fmt.Errorf("定位 Python 运行时失败: %w", err)
	}
	var cmd *exec.Cmd
	for attempt := 0; attempt < 3; attempt++ {
		port, perr := pickPort()
		if perr != nil {
			err = perr
			continue
		}
		cmd = exec.CommandContext(ctx, pythonBin(root),
			"-m", "uvicorn", "app.main:app", "--host", "127.0.0.1", "--port", fmt.Sprint(port))
		cmd.Dir = filepath.Join(root, "engine")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// 过滤污染 env: 父 shell 的 LOG_FORMAT=json / 自定义 %-style 配置会让 uvicorn 崩溃
		cmd.Env = filterPythonEnv(os.Environ())
		setProcGroup(cmd) // 平台文件实现
		if err = cmd.Start(); err != nil {
			continue // 端口竞争等,换端口重试
		}
		baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
		if err = waitHealthy(ctx, baseURL, 30*time.Second); err == nil {
			configuration.Configuration.PythonEngine.BaseURL = baseURL
			return baseURL, func() { killProcGroup(cmd) }, nil
		}
		killProcGroup(cmd)
	}
	return "", nil, fmt.Errorf("Python 引擎启动失败(重试 3 次): %w", err)
}


// filterPythonEnv 保留 PATH/HOME 等必要 env,移除 Python app 已知冲突的键
// 桌面 .app bundle 启动时会 inherit 父 shell 的全部 env,某些自定义
// (如 LOG_FORMAT=json)与 Python app 自己的 %-style logging 配置冲突,
// 导致 uvicorn 启动时 raise ValueError。这里白名单 + 黑名单策略。
func filterPythonEnv(parent []string) []string {
	// 必须保留的(让 python3 / uvicorn 能找到依赖)
	keep := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "TMPDIR": true,
		"LANG": true, "LC_ALL": true, "LC_CTYPE": true,
		"PYTHONHOME": true, "PYTHONPATH": true, "PYTHONUNBUFFERED": true,
		// 桌面端自己的 env
		"ALCHEMY_DESKTOP": true, "ALCHEMY_PYTHON_RUNTIME": true, "GIN_MODE": true,
	}
	// 必须移除的(冲突或与 desktop 模式无关)
	drop := map[string]bool{
		"LOG_FORMAT":  true, // 父 shell 的 json 格式与 Python app %-style 冲突
		"LOG_LEVEL":   true, // 留给 Python app 自己 default
		"RUST_LOG":    true, // rust 工具链噪声
		"OS_ACTIVITY": true, // macOS
	}
	out := make([]string, 0, len(parent))
	for _, kv := range parent {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		k := kv[:eq]
		if drop[k] {
			continue
		}
		if !keep[k] {
			// 不在白名单的也透传(避免漏掉必要变量)
			// 实际更稳妥:全透传 + drop 黑名单
		}
		out = append(out, kv)
	}
	return out
}
