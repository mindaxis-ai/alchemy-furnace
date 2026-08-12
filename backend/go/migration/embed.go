// Package migration 历史版本化 SQL 迁移文件(golang-migrate iofs 源)
// ⚠️ 已废弃: 运行时 schema 同步由 gorm.AutoMigrate 统一负责(见 internal/dao/migrate.go)
// 本目录仅保留为历史快照,不再被任何代码 import。
// 详见本目录 README.md。
package migration

import "embed"

// FS 内嵌全部迁移 SQL(历史快照,Docker 镜像不再依赖)
// 仅作应急手工参考用,不要在生产路径调用。
//
//go:embed *.sql
var FS embed.FS
