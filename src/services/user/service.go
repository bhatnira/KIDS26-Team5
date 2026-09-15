package user

import (
	"context"
	"errors"
	"strings"

	"antelope/internal/modules/auth/session"
	"antelope/internal/modules/log"
	"antelope/internal/modules/misc"
	"antelope/models"
	"antelope/pkg/apperr"
	"antelope/pkg/response"
	"antelope/pkg/types"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service handles user management operations.
type Service interface {
	Register(dto types.RegisterDto) error
	List() (gin.H, error)
	Add(dto types.UserAddDto) error
	Update(dto types.UserEditDto) error
	Delete(userID uint) error

	// Self-service operations for the authenticated user.
	Profile(userID uint) (gin.H, error)
	UpdateProfile(userID uint, dto types.UserUpdateProfileDto) error
	ChangePassword(userID uint, dto types.ChangePasswordDto) error

	// ResetPassword sets a new password after email-code verification.
	ResetPassword(dto types.ResetPasswordDto) error
}

type userService struct {
	db    *gorm.DB
	store session.Store
}

func NewService(db *gorm.DB, store session.Store) Service {
	return &userService{db: db, store: store}
}

// bumpEpoch force-revokes the user's outstanding access tokens by advancing
// their token epoch (see session.Store / AuthMiddleware). Best-effort: a failure
// only means the change takes effect at natural token expiry instead of
// immediately.
func (s *userService) bumpEpoch(userID uint) {
	if s.store == nil || userID == 0 {
		return
	}
	if err := s.store.BumpUserEpoch(context.Background(), userID); err != nil {
		log.L().Warn("failed to bump user token epoch", zap.Uint("id", userID), zap.Error(err))
	}
}

func (s *userService) Register(dto types.RegisterDto) error {
	if checkEmailExist(s.db, dto.Email) {
		return apperr.CheckFail(response.CheckFailCode, response.EmailRegistered)
	}
	newUser := models.User{
		Name:     strings.Split(dto.Email, "@")[0],
		Email:    dto.Email,
		Password: misc.BcryptHash(dto.Password),
	}
	if err := s.db.Create(&newUser).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// Lost a concurrent-registration race; the unique index protected
			// integrity — return the friendly "already registered", not a 500.
			return apperr.CheckFail(response.CheckFailCode, response.EmailRegistered)
		}
		log.L().Error("create user failed", zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	return nil
}

func (s *userService) List() (gin.H, error) {
	var users []types.UserListDto
	if err := s.db.Model(&models.User{}).
		Select(`id,name,email,department,"group",role,updated_at,status,auth_source,auth_provider`).
		Scan(&users).Error; err != nil {
		log.L().Error("get user list failed", zap.Error(err))
		return nil, apperr.ServerError(err.Error())
	}
	return gin.H{"users": users}, nil
}

func (s *userService) Add(dto types.UserAddDto) error {
	if checkEmailExist(s.db, dto.Email) {
		return apperr.CheckFail(response.CheckFailCode, response.EmailRegistered)
	}
	dto.Password = misc.BcryptHash(dto.Password)
	newUser := dto.ToUser()
	if err := s.db.Create(&newUser).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apperr.CheckFail(response.CheckFailCode, response.EmailRegistered)
		}
		log.L().Error("create user failed", zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	return nil
}

func (s *userService) Update(dto types.UserEditDto) error {
	// Capture the pre-update role/status so we can force-revoke the user's
	// sessions if an admin changes their privileges or disables them.
	var existing models.User
	_ = s.db.Where("email = ?", dto.Email).First(&existing).Error // best-effort

	updates := map[string]any{
		"name":       dto.Name,
		"email":      dto.Email,
		"department": dto.Department,
		"group":      dto.Group,
		"role":       dto.Role,
		"status":     dto.Status,
	}
	if err := s.db.Model(&models.User{}).Where("email = ?", dto.Email).Updates(updates).Error; err != nil {
		log.L().Error("user update failed", zap.String("email", dto.Email), zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}

	if existing.ID != 0 && (existing.Role != dto.Role || existing.Status != dto.Status) {
		// Role demotion or account disable must take effect immediately, not at
		// the 24h access-token expiry.
		s.bumpEpoch(existing.ID)
	}
	return nil
}

func (s *userService) Delete(userID uint) error {
	result := s.db.Unscoped().Delete(&models.User{}, userID)
	if result.Error != nil {
		return apperr.ServerError(response.SystemError)
	}
	if result.RowsAffected == 0 {
		return apperr.CheckFail(response.CheckFailCode, response.UserNotExist)
	}
	// Invalidate any outstanding tokens of the deleted user immediately.
	s.bumpEpoch(userID)
	return nil
}

func (s *userService) Profile(userID uint) (gin.H, error) {
	var profile types.UserProfileDto
	if err := s.db.Model(&models.User{}).
		Select(`id,name,email,department,"group",role,auth_source,auth_provider`).
		Where("id = ?", userID).
		First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.CheckFail(response.CheckFailCode, response.UserNotExist)
		}
		log.L().Error("get user profile failed", zap.Uint("id", userID), zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	return gin.H{"profile": profile}, nil
}

func (s *userService) UpdateProfile(userID uint, dto types.UserUpdateProfileDto) error {
	updates := map[string]any{
		"name":       dto.Name,
		"department": dto.Department,
	}
	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		log.L().Error("update user profile failed", zap.Uint("id", userID), zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	return nil
}

func (s *userService) ChangePassword(userID uint, dto types.ChangePasswordDto) error {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.CheckFail(response.CheckFailCode, response.UserNotExist)
		}
		log.L().Error("load user for password change failed", zap.Uint("id", userID), zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	// Externally authenticated users have no local password to change.
	if user.AuthSource != models.AuthSourceLocal {
		return apperr.Forbidden(response.PasswordChangeDenied)
	}
	if !misc.BcryptCheck(dto.OldPassword, user.Password) {
		return apperr.CheckFail(response.CheckFailCode, response.OldPasswordError)
	}
	if err := s.db.Model(&user).Update("password", misc.BcryptHash(dto.NewPassword)).Error; err != nil {
		log.L().Error("update password failed", zap.Uint("id", userID), zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	// A password change invalidates all existing sessions for this user.
	s.bumpEpoch(userID)
	return nil
}

// ResetPassword sets a new password for a locally authenticated user. The
// verification code is checked by the handler before this is called; here we
// re-assert that the account is local since LDAP/OIDC users have no local
// password to reset.
func (s *userService) ResetPassword(dto types.ResetPasswordDto) error {
	var user models.User
	if err := s.db.Where("email = ?", dto.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.CheckFail(response.CheckFailCode, response.UserNotExist)
		}
		log.L().Error("load user for password reset failed", zap.String("email", dto.Email), zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	if user.AuthSource != models.AuthSourceLocal {
		return apperr.Forbidden(response.PasswordResetDenied)
	}
	if err := s.db.Model(&user).Update("password", misc.BcryptHash(dto.Password)).Error; err != nil {
		log.L().Error("reset password failed", zap.Uint("id", user.ID), zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	// A password reset invalidates all existing sessions for this user.
	s.bumpEpoch(user.ID)
	return nil
}

func checkEmailExist(db *gorm.DB, email string) bool {
	var user models.User
	db.Where("email = ?", email).First(&user)
	return user.ID != 0
}
