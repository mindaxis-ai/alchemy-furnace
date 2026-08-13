//go:build darwin

// apply_zip.go - darwin 更新用的 zip 解压辅助
// (windows 走 NSIS Setup.exe 静默安装,无需内置解压)
package updater

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// unzipToDir 将 zip 流式解压到 dst
// 防御: 路径穿越(条目名含 ../) → 跳过
func unzipToDir(ctx context.Context, zipPath, dst string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		// ctx 取消检测
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// 防路径穿越
		target, err := filepath.Abs(filepath.Join(dst, f.Name))
		if err != nil {
			return err
		}
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) && target != filepath.Clean(dst) {
			return fmt.Errorf("updater: zip 条目含非法路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		zr, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			zr.Close()
			return err
		}
		if _, err := io.Copy(out, zr); err != nil {
			zr.Close()
			out.Close()
			return err
		}
		zr.Close()
		out.Close()
	}
	return nil
}
