package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	emailpkg "antelope/internal/modules/email"
	"antelope/pkg/response"
	"antelope/pkg/types"
	authsvc "antelope/services/auth"
	cachesvc "antelope/services/cache"
	usersvc "antelope/services/user"
)

// UserHandler handles user registration and management routes.
type UserHandler struct {
	svc      usersvc.Service
	cacheSvc cachesvc.Service
}

func NewUserHandler(svc usersvc.Service, cacheSvc cachesvc.Service) *UserHandler {
	return &UserHandler{svc: svc, cacheSvc: cacheSvc}
}

// @Summary Register user
// @Description Register user
// @Tags user
// @Produce json
// @Accept json
// @Param body body types.RegisterDto true "Registration payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/register [post]
func (h *UserHandler) Register(c *gin.Context) {
	var req types.RegisterDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !emailpkg.VerifyEmailFormat(req.Email) {
		response.CheckFail(c, nil, response.EmailFormatCheck)
		return
	}
	if len(req.Password) < 6 {
		response.CheckFail(c, nil, response.PasswordCheck)
		return
	}
	if !h.cacheSvc.VerificationCode(req.Email, req.Code) {
		response.CheckFail(c, nil, response.VerificationCodeError)
		return
	}
	response.Render(c, nil, h.svc.Register(req))
}

// @Summary Send registration code
// @Description Send registration code
// @Tags user
// @Produce json
// @Accept json
// @Param body body types.EmailDto true "Email payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/register/code [post]
func (h *UserHandler) SendRegisterCode(c *gin.Context) {
	var req types.EmailDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !emailpkg.VerifyEmailFormat(req.Email) {
		response.CheckFail(c, nil, response.EmailFormatCheck)
		return
	}
	response.Render(c, nil, h.cacheSvc.SendRegisterCode(req.Email))
}

// @Summary Reset password
// @Description Reset a local account's password after email-code verification
// @Tags user
// @Produce json
// @Accept json
// @Param body body types.ResetPasswordDto true "Reset password payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/reset [post]
func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req types.ResetPasswordDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !emailpkg.VerifyEmailFormat(req.Email) {
		response.CheckFail(c, nil, response.EmailFormatCheck)
		return
	}
	if len(req.Password) < 6 {
		response.CheckFail(c, nil, response.PasswordCheck)
		return
	}
	if !h.cacheSvc.VerificationResetCode(req.Email, req.Code) {
		response.CheckFail(c, nil, response.VerificationCodeError)
		return
	}
	response.Render(c, nil, h.svc.ResetPassword(req))
}

// @Summary Send password reset code
// @Description Send a verification code to reset a local account's password
// @Tags user
// @Produce json
// @Accept json
// @Param body body types.EmailDto true "Email payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/reset/code [post]
func (h *UserHandler) SendResetCode(c *gin.Context) {
	var req types.EmailDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !emailpkg.VerifyEmailFormat(req.Email) {
		response.CheckFail(c, nil, response.EmailFormatCheck)
		return
	}
	response.Render(c, nil, h.cacheSvc.SendResetCode(req.Email))
}

// @Summary List users
// @Description List users
// @Tags system-users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/user/list [get]
func (h *UserHandler) List(c *gin.Context) {
	data, err := h.svc.List()
	response.Render(c, data, err)
}

// @Summary Add user
// @Description Add user
// @Tags system-users
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.UserAddDto true "User payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/user/add [post]
func (h *UserHandler) Add(c *gin.Context) {
	var req types.UserAddDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.Add(req))
}

// @Summary Update user
// @Description Update user
// @Tags system-users
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.UserEditDto true "User payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/user/update [post]
func (h *UserHandler) Update(c *gin.Context) {
	var req types.UserEditDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.Update(req))
}

// @Summary Delete user
// @Description Delete user
// @Tags system-users
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/user/delete/{id} [delete]
func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.Delete(uint(id)))
}

// Profile returns the authenticated user's own profile.
// @Summary Get current user profile
// @Description Get current user profile
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/profile [get]
func (h *UserHandler) Profile(c *gin.Context) {
	data, err := h.svc.Profile(authsvc.GetUserID(c))
	response.Render(c, data, err)
}

// UpdateProfile updates the authenticated user's name and department.
// @Summary Update profile
// @Description Update profile
// @Tags user
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.UserUpdateProfileDto true "Profile payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/profile [put]
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	var req types.UserUpdateProfileDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.UpdateProfile(authsvc.GetUserID(c), req))
}

// ChangePassword changes the authenticated user's local password.
// @Summary Change password
// @Description Change password
// @Tags user
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.ChangePasswordDto true "Password payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/password [post]
func (h *UserHandler) ChangePassword(c *gin.Context) {
	var req types.ChangePasswordDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if len(req.NewPassword) < 6 {
		response.CheckFail(c, nil, response.PasswordCheck)
		return
	}
	response.Render(c, nil, h.svc.ChangePassword(authsvc.GetUserID(c), req))
}
