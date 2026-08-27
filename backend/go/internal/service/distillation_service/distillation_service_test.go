package distillation_service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/alchemy-furnace/server/internal/distillation"
	appErrors "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/service/credential"
)

type fakeClient struct {
	called bool
	result *distillation.Response
	err    error
}

func (f *fakeClient) Distill(_ context.Context, _, _, _ string, _ *credential.ModelCredentials) (*distillation.Response, error) {
	f.called = true
	return f.result, f.err
}

type fakeResolver struct {
	credentials *credential.ModelCredentials
	err         error
}

func (f fakeResolver) ResolveCredentials(context.Context, string) (*credential.ModelCredentials, error) {
	return f.credentials, f.err
}

func (f fakeResolver) ResolveSynthesisCredentials(context.Context) (*credential.ModelCredentials, error) {
	return f.credentials, f.err
}

func (f fakeResolver) ResolveFusionCredentials(context.Context) (*credential.ModelCredentials, error) {
	return f.credentials, f.err
}

func TestDistillValidatesBeforeCallingDependencies(t *testing.T) {
	client := &fakeClient{}
	service := New(client, fakeResolver{})

	result, appErr := service.Distill(context.Background(), "A", "短", "zh-CN")

	if result != nil || appErr == nil {
		t.Fatalf("result = %#v, error = %#v; want validation error", result, appErr)
	}
	if client.called {
		t.Fatal("client was called for invalid input")
	}
}

func TestDistillPassesConfiguredCredentials(t *testing.T) {
	want := &distillation.Response{Name: "结构化金丹"}
	client := &fakeClient{result: want}
	resolver := fakeResolver{credentials: &credential.ModelCredentials{Model: "configured-model", APIKey: "secret"}}
	service := New(client, resolver)

	got, appErr := service.Distill(context.Background(), "人物主题", "提炼公开资料中的决策方式", "en")

	if appErr != nil || got != want {
		t.Fatalf("got = %#v, error = %#v", got, appErr)
	}
	if !client.called {
		t.Fatal("client was not called")
	}
}

func TestDistillReturnsCredentialFailureWithoutCallingClient(t *testing.T) {
	client := &fakeClient{}
	service := New(client, fakeResolver{err: errors.New("未配置模型")})

	result, appErr := service.Distill(context.Background(), "人物主题", "提炼公开资料中的决策方式", "zh-CN")

	if result != nil || appErr == nil || client.called {
		t.Fatalf("result = %#v, error = %#v, called = %v", result, appErr, client.called)
	}
}

func TestDistillMapsRemoteServiceUnavailableWithCodeAndData(t *testing.T) {
	remote := &distillation.RemoteError{
		Status:    http.StatusServiceUnavailable,
		Code:      "research_search_blocked",
		Stage:     "research",
		Message:   "搜索受限",
		Retryable: true,
		Details:   map[string]any{"documents": 0},
	}
	service := New(&fakeClient{err: remote}, fakeResolver{credentials: &credential.ModelCredentials{}})

	_, appErr := service.Distill(context.Background(), "人物主题", "提炼公开资料中的决策方式", "zh-CN")

	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeServiceUnavailable) {
		t.Fatalf("error = %#v, want ErrorTypeServiceUnavailable", appErr)
	}
	if appErr.GetCode() != "research_search_blocked" {
		t.Fatalf("code = %q, want research_search_blocked", appErr.GetCode())
	}
	if appErr.Error() != "搜索受限" {
		t.Fatalf("message = %q, want 搜索受限", appErr.Error())
	}
	withData, ok := appErr.(appErrors.ErrorWithData)
	if !ok {
		t.Fatal("expected ErrorWithData to carry stage/retryable/details")
	}
	payload, ok := withData.GetData().(map[string]any)
	if !ok || payload["stage"] != "research" || payload["retryable"] != true {
		t.Fatalf("data = %#v, want {stage:research retryable:true}", withData.GetData())
	}
}

func TestDistillMapsRemoteInvalidRequestTo400WithPublicMessage(t *testing.T) {
	remote := &distillation.RemoteError{
		Status:  http.StatusBadRequest,
		Message: "未配置可用于智能炼制的模型，请先到设置中配置模型供应商",
	}
	service := New(&fakeClient{err: remote}, fakeResolver{credentials: &credential.ModelCredentials{}})

	_, appErr := service.Distill(context.Background(), "人物主题", "提炼公开资料中的决策方式", "zh-CN")

	if appErr == nil || !appErr.IsType(appErrors.ErrorTypeInvalidRequest) {
		t.Fatalf("error = %#v, want ErrorTypeInvalidRequest", appErr)
	}
	if appErr.Error() != "未配置可用于智能炼制的模型，请先到设置中配置模型供应商" {
		t.Fatalf("message = %q, want 公开 message 保留", appErr.Error())
	}
}
