// Package user 用户档案 HTTP 处理器
// 路由: /api/v1/user/profile;本地/单用户部署,无注册登录,整库固定 id=1
package user

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/internal/context/contextutil"
	ierr "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// User 用户档案处理器
type User struct {
	db *gorm.DB
}

// New 构造处理器
func New(db *gorm.DB) *User {
	return &User{db: db}
}

// Response 用户档案响应 DTO
type Response struct {
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	Avatar      string    `json:"avatar"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Get 获取当前用户档案
// GET /api/v1/user/profile
// 行为:首次调用时自动创建默认行(id=1, display_name=用户, bio=空)
func (cls *User) Get(c *gin.Context) (response.Code, any, error) {
	profile, err := cls.getOrCreateProfile(c)
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, toResponse(profile), nil
}

// UpdateRequest 更新请求(指针字段区分「未传」与「置空」)
type UpdateRequest struct {
	DisplayName *string `json:"display_name" binding:"omitempty,max=64"`
	Bio         *string `json:"bio"          binding:"omitempty,max=500"`
	Avatar      *string `json:"avatar"       binding:"omitempty,max=500"`
}

// Update 更新当前用户档案
// PUT /api/v1/user/profile
func (cls *User) Update(c *gin.Context) (response.Code, any, error) {
	ctx := contextutil.NewContextWithGin(c)
	var body UpdateRequest
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		return response.InvalidParams, nil, berr
	}

	profile, err := cls.getOrCreateProfile(c)
	if err != nil {
		return 0, nil, err
	}

	updates := map[string]any{}
	if body.DisplayName != nil {
		name := strings.TrimSpace(*body.DisplayName)
		if name == "" {
			return response.InvalidParams, nil, ierr.New(ierr.ErrorTypeInvalidRequest, "user.display_name_empty", "显示名不能为空")
		}
		if utf8.RuneCountInString(name) > 32 {
			return response.InvalidParams, nil, ierr.New(ierr.ErrorTypeInvalidRequest, "user.display_name_too_long", "显示名最多 32 个字符")
		}
		updates["display_name"] = name
	}
	if body.Bio != nil {
		bio := strings.TrimSpace(*body.Bio)
		if utf8.RuneCountInString(bio) > 500 {
			return response.InvalidParams, nil, ierr.New(ierr.ErrorTypeInvalidRequest, "user.bio_too_long", "简介最多 500 个字符")
		}
		updates["bio"] = bio
	}
	if body.Avatar != nil {
		updates["avatar"] = strings.TrimSpace(*body.Avatar)
	}

	if len(updates) == 0 {
		return response.Ok, toResponse(profile), nil
	}

	if err := cls.db.WithContext(ctx).Model(profile).Updates(updates).Error; err != nil {
		return 0, nil, ierr.New(ierr.ErrorTypeServerInternalError, "user.update", err.Error())
	}
	// 重读以返回最新值
	if err := cls.db.WithContext(ctx).First(profile).Error; err != nil {
		return 0, nil, ierr.New(ierr.ErrorTypeServerInternalError, "user.reload", err.Error())
	}
	return response.Ok, toResponse(profile), nil
}

// getOrCreateProfile 取整库唯一的用户档案;不存在则创建默认行
func (cls *User) getOrCreateProfile(c *gin.Context) (*model.UserProfile, error) {
	profile := &model.UserProfile{}
	if err := cls.db.WithContext(c.Request.Context()).First(profile, 1).Error; err == nil {
		return profile, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, ierr.New(ierr.ErrorTypeServerInternalError, "user.get", err.Error())
	}
	// 不存在:插入默认行
	profile = &model.UserProfile{ID: 1, DisplayName: "用户"}
	if err := cls.db.WithContext(c.Request.Context()).Create(profile).Error; err != nil {
		return nil, ierr.New(ierr.ErrorTypeServerInternalError, "user.create", err.Error())
	}
	return profile, nil
}

func toResponse(p *model.UserProfile) *Response {
	if p == nil {
		return &Response{DisplayName: "用户"}
	}
	return &Response{
		DisplayName: p.DisplayName,
		Bio:         p.Bio,
		Avatar:      p.Avatar,
		UpdatedAt:   p.UpdatedAt,
	}
}
