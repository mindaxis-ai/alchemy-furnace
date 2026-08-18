package distillation

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service service.Distillation }

func New(service service.Distillation) *Handler { return &Handler{service: service} }

type Request struct {
	Subject string `json:"subject" binding:"required,max=120"`
	Brief   string `json:"brief" binding:"required,max=1000"`
	Locale  string `json:"locale"`
}

func (h *Handler) Nuwa(c *gin.Context) (response.Code, any, error) {
	var body Request
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}
	result, err := h.service.Distill(contextutil.NewContextWithGin(c), body.Subject, body.Brief, body.Locale)
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, result, nil
}
