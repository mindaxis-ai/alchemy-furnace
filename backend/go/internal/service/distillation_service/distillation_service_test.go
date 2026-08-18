package distillation_service

import (
	"context"
	"errors"
	"testing"

	"github.com/alchemy-furnace/server/internal/distillation"
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
