// migrate 子命令: 基于 GORM AutoMigrate 的多数据库 schema 同步
//   - migrate up: 同步全部业务表(幂等,跨 PG/MySQL/SQLite)
//   - migrate down: DropTable 全部业务表(本地重建用,带确认)
package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/spf13/cobra"
)

// NewMigrateCommand migrate 命令组
func NewMigrateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:  "migrate",
		Args: cobra.NoArgs,
	}
	command.AddCommand(newMigrateUpCommand(), newMigrateDownCommand())
	return command
}

// initDBForCommand 子命令通用: 初始化数据库连接
func initDBForCommand() error {
	return dao.InitDatabase(&configuration.Configuration.Database)
}

func newMigrateUpCommand() *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:  "up",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := initDBForCommand(); err != nil {
				return err
			}
			defer dao.CloseDatabase()

			// --force:跳过 HasSchema 短路,补齐新列/索引(新老 schema 漂移修复场景)
			if !force {
				has, err := dao.HasSchema()
				if err != nil {
					return err
				}
				if has {
					fmt.Println("[炼丹炉] schema 已存在,AutoMigrate 幂等跳过(可手动 down+up 重建,或加 --force 强制补齐)")
					return nil
				}
			} else {
				fmt.Println("[炼丹炉] --force 模式:跳过 HasSchema 短路,直接 AutoMigrate 补齐新列/索引")
			}

			if err := dao.MigrateUp(); err != nil {
				return err
			}
			fmt.Println("[炼丹炉] AutoMigrate 完成,业务表已对齐最新 model")
			return nil
		},
	}
	command.Flags().BoolVarP(&force, "force", "f", false, "跳过 HasSchema 短路,强制补齐新增列/索引(用于新老 schema 漂移修复)")
	return command
}

func newMigrateDownCommand() *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:  "down",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Print("[炼丹炉] 警告: 将 DROP 全部业务表,数据不可恢复!输入 yes 确认: ")
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				if strings.TrimSpace(line) != "yes" {
					fmt.Println("[炼丹炉] 已取消")
					return nil
				}
			}
			if err := initDBForCommand(); err != nil {
				return err
			}
			defer dao.CloseDatabase()
			if err := dao.MigrateDown(); err != nil {
				return err
			}
			fmt.Println("[炼丹炉] 已 DROP 全部业务表(数据库文件保留;SQLite 用户可直接删除文件)")
			return nil
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "跳过确认(脚本用)")
	return command
}
