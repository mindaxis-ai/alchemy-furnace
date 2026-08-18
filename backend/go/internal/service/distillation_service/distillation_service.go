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
		if stderrors.As(err, &remote) && remote.Status >= http.StatusBadRequest && remote.Status < http.StatusInternalServerError {
			return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.distillation.remote", remote.Message)
		}
		return nil, appErrors.New(appErrors.ErrorTypeServerInternalError, "service.distillation.call", err.Error())
	}
	return result, nil
}
