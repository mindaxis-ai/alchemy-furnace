# 历史迁移文件(已废弃)

⚠️ **本目录不再被运行时引用,仅作历史快照保留。**

## 当前迁移方案

- 运行时 schema 同步: **`gorm.AutoMigrate` 驱动(由 `internal/dao/migrate.go` 实现)**
- 单一事实来源: `model/*.go` 中的 GORM 标签
- 多数据库: 自动适配 **PostgreSQL / MySQL / SQLite**
- 启动行为: `serve` 在空库上自动跑 AutoMigrate(SKIP_AUTO_MIGRATE=1 可关)
- 子命令:
  - `migrate up`   → AutoMigrate 全部业务表(幂等,跨驱动)
  - `migrate down` → DropTable 全部业务表(本地重建)

## 本目录内文件状态

| 文件 | 状态 |
|------|------|
| `000001_init_schema.up.sql` / `.down.sql` | 初始 8 表 schema,已被 GORM 模型替代 |
| `000002_widen_source_fingerprint.up.sql` / `.down.sql` | 列宽 64→80; GORM 模型已是 80,该变更融入 `LanguagePattern.SourceFingerprint` 的 `size:80` |

## 为什么保留

1. **考古价值**: 字段变更/列宽调整的历史可追溯
2. **对比参考**: 帮新手理解「裸 SQL 迁移 → ORM 模型」的演进
3. **应急回退**: 极端情况下 DBA 仍可手工执行原始 SQL(不推荐,无 ORM 维护)

## 删除计划

在以下里程碑后清理:
- 至少 2 个生产环境已稳定运行 AutoMigrate 模式
- CI 覆盖 PG/MySQL/SQLite 三驱动的 AutoMigrate 验证
- 文档说明到位(README + 部署指南)

## 不再 import 的原因

旧 `internal/dao/migrate.go` 通过 `embed.FS` 加载本目录 SQL,经 `golang-migrate` 执行。
新实现直接走 `db.AutoMigrate(allMigratableModels...)`,**不读本目录任何文件**。
