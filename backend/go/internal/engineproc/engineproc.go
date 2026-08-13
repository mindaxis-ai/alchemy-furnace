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
