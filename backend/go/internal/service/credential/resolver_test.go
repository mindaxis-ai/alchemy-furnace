package credential

// resolver 测试(Task 11):provider 非敏感元数据填充链的行为锚点。

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newResolverTestDB 临时 sqlite 库并注入 dao 全局 DB,测试结束还原。
func newResolverTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "credential-resolver.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.LLMProvider{}, &model.LLMModel{}); err != nil {
		t.Fatalf("AutoMigrate llm models: %v", err)
	}
	previous := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previous })
}

func TestResolveCredentialsPopulatesProviderMetadata(t *testing.T) {
	newResolverTestDB(t)
	provider := &model.LLMProvider{
		Name: "deepseek", DisplayName: "DeepSeek 官方", Protocol: "deepseek",
		BaseURL: "https://api.deepseek.com", IsEnabled: true,
	}
	if err := dao.DB.Create(provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	mdl := &model.LLMModel{
		Name: "deepseek-chat", DisplayName: "DeepSeek Chat",
		ProviderID: provider.UUID.String(), IsEnabled: true,
	}
	if err := dao.DB.Create(mdl).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	got, err := NewResolver().ResolveCredentials(context.Background(), "deepseek-chat")
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if got.Model != "deepseek-chat" {
		t.Fatalf("model = %q, want deepseek-chat", got.Model)
	}
	if got.ProviderName != "deepseek" {
		t.Fatalf("provider_name = %q, want deepseek (LLMProvider.Name)", got.ProviderName)
	}
	if got.ProviderType != "deepseek" {
		t.Fatalf("provider_type = %q, want deepseek (LLMProvider.Protocol)", got.ProviderType)
	}
	if got.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("base_url = %q, want provider base_url", got.BaseURL)
	}
}

func TestResolveCredentialsUnregisteredModelKeepsEmptyProvider(t *testing.T) {
	newResolverTestDB(t)

	got, err := NewResolver().ResolveCredentials(context.Background(), "never-registered")
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if got.Model != "never-registered" {
		t.Fatalf("model = %q, want passthrough name", got.Model)
	}
	if got.ProviderName != "" || got.ProviderType != "" {
		t.Fatalf("unregistered model must keep empty provider metadata, got %+v", got)
	}
}
