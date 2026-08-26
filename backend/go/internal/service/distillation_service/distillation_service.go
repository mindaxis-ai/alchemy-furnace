package distillation_service

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"

	"github.com/alchemy-furnace/server/internal/distillation"
	appErrors "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/service/credential"
)

type Service struct {
	client     distillation.Client
	credential credential.Resolver
}

func New(client distillation.Client, resolver credential.Resolver) *Service {
	return &Service{client: client, credential: resolver}
}

func (s *Service) Distill(ctx context.Context, subject, brief, locale string) (*distillation.Response, appErrors.Error) {
	subject, brief = strings.TrimSpace(subject), strings.TrimSpace(brief)
	if len([]rune(subject)) < 2 || len([]rune(brief)) < 4 {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.distillation.input", "请填写人物或主题，以及至少一句炼制目标")
	}
	if locale != "en" {
		locale = "zh-CN"
	}
	creds, err := s.credential.ResolveSynthesisCredentials(ctx)
	if err != nil {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.distillation.credentials", err.Error())
	}
	result, err := s.client.Distill(ctx, subject, brief, locale, creds)
	if err != nil {
		var remote *distillation.RemoteError
		if stderrors.As(err, &remote) {
			// 透传远端语义: error_code=远端 code,data={stage,retryable,details}。
			// 可重试的远端 503 映射为 ErrorTypeServiceUnavailable(HTTP 503),
			// 不被通用 Wrapper 隐藏成"服务器内部错误"。
			errorType := appErrors.ErrorTypeInvalidRequest
			switch {
			case remote.Status == http.StatusServiceUnavailable:
				errorType = appErrors.ErrorTypeServiceUnavailable
			case remote.Status >= http.StatusInternalServerError:
				errorType = appErrors.ErrorTypeServerInternalError
			}
			return nil, appErrors.NewWithData(errorType, remote.Code, map[string]any{
				"stage":     remote.Stage,
				"retryable": remote.Retryable,
				"details":   remote.Details,
			}, remote.Message)
		}
		return nil, appErrors.New(appErrors.ErrorTypeServerInternalError, "service.distillation.call", err.Error())
	}
	return result, nil
}
