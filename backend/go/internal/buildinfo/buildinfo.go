// Package buildinfo 构建期注入的版本信息(ldflags -X)
//
// 由 go build 时注入:
//   go build -ldflags "-X github.com/alchemy-furnace/server/internal/buildinfo.Version=v0.1.0 \
//                      -X .../buildinfo.Commit=$(git rev-parse --short HEAD) \
//                      -X .../buildinfo.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
//                      -X .../buildinfo.UpdateRepo=owner/repo"
//
// 默认值("dev" / "")用于 go run 与本地开发,updater 走 UpdateRepo 指定的 GitHub Releases
package buildinfo

// Version 语义化版本号(空时显示 "dev")
var Version = "dev"

// Commit git short SHA(空时显示 "unknown")
var Commit = ""

// BuildDate ISO8601 格式构建时间(空时显示 "unknown")
var BuildDate = ""

// UpdateRepo updater 唯一更新源(owner/repo 格式,空则禁用更新)
var UpdateRepo = ""
