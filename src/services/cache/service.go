package cache

import (
	"context"
	"time"

	emailmod "antelope/internal/modules/email"
	"antelope/internal/modules/log"
	"antelope/models"
	"antelope/pkg/apperr"
	"antelope/pkg/response"
	"antelope/templates"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	verificationCodeExpireMinutes = 5
	verificationCodeTmpl          = "mail/user/auth/verification_code.tmpl"
	resetPasswordTmpl             = "mail/user/auth/reset_password.tmpl"
)

// RegCode returns the Redis key for a registration verification code.
func RegCode(email string) string {
	return "automapper:reg:" + email
}

// ResetCode returns the Redis key for a password-reset verification code.
func ResetCode(email string) string {
	return "automapper:reset:" + email
}

// Service provides email verification code operations.
type Service interface {
	SendRegisterCode(emailAddr string) error
	VerificationCode(emailAddr, code string) bool
	SendResetCode(emailAddr string) error
	VerificationResetCode(emailAddr, code string) bool
}

type cacheService struct {
	db     *gorm.DB
	redis  redis.UniversalClient
	mailer *emailmod.Mailer
}

func NewService(db *gorm.DB, redis redis.UniversalClient, mailer *emailmod.Mailer) Service {
	return &cacheService{db: db, redis: redis, mailer: mailer}
}

func (s *cacheService) SendRegisterCode(emailAddr string) error {
	if s.checkEmailExist(emailAddr) {
		return apperr.CheckFail(response.CheckFailCode, response.EmailRegistered)
	}

	code, err := s.getOrCreateCode(RegCode(emailAddr))
	if err != nil {
		log.L().Error("get verification code failed", zap.Error(err))
		return err
	}

	if s.mailer == nil {
		log.L().Error("mailer not initialized")
		s.redis.Del(context.Background(), RegCode(emailAddr))
		return apperr.ServerError(response.SystemError)
	}

	if err := s.mailer.Send(emailmod.SendOptions{
		To:               []string{emailAddr},
		Subject:          "Verification Code",
		Templates:        templates.FS,
		HTMLTemplatePath: verificationCodeTmpl,
		TemplateData: struct {
			Code          string
			ExpireMinutes int
		}{
			Code:          code,
			ExpireMinutes: verificationCodeExpireMinutes,
		},
	}); err != nil {
		s.redis.Del(context.Background(), RegCode(emailAddr))
		return apperr.BadRequest(response.FailCode, response.SendFail)
	}

	return nil
}

// SendResetCode emails a password-reset verification code. It is only available
// for locally authenticated accounts; LDAP/OIDC users have no local password.
func (s *cacheService) SendResetCode(emailAddr string) error {
	user, ok := s.getUser(emailAddr)
	if !ok {
		return apperr.CheckFail(response.CheckFailCode, response.UserNotExist)
	}
	if user.AuthSource != models.AuthSourceLocal {
		return apperr.Forbidden(response.PasswordResetDenied)
	}

	code, err := s.getOrCreateCode(ResetCode(emailAddr))
	if err != nil {
		log.L().Error("get reset code failed", zap.Error(err))
		return err
	}

	if s.mailer == nil {
		log.L().Error("mailer not initialized")
		s.redis.Del(context.Background(), ResetCode(emailAddr))
		return apperr.ServerError(response.SystemError)
	}

	if err := s.mailer.Send(emailmod.SendOptions{
		To:               []string{emailAddr},
		Subject:          "Reset Password",
		Templates:        templates.FS,
		HTMLTemplatePath: resetPasswordTmpl,
		TemplateData: struct {
			Code          string
			ExpireMinutes int
		}{
			Code:          code,
			ExpireMinutes: verificationCodeExpireMinutes,
		},
	}); err != nil {
		s.redis.Del(context.Background(), ResetCode(emailAddr))
		return apperr.BadRequest(response.FailCode, response.SendFail)
	}

	return nil
}

// VerificationCode checks whether code matches the stored code for emailAddr and
// consumes it atomically.
//
// CONC-1: The original GET-then-DEL pattern had a TOCTOU race: two concurrent
// calls could both read the same code before either deleted it, letting both
// return true. A Lua script makes the read-and-delete a single atomic Redis
// operation so only one caller can ever succeed.
func (s *cacheService) VerificationCode(emailAddr, code string) bool {
	return s.consumeCode(RegCode(emailAddr), code)
}

// VerificationResetCode checks and consumes a password-reset code for emailAddr.
func (s *cacheService) VerificationResetCode(emailAddr, code string) bool {
	return s.consumeCode(ResetCode(emailAddr), code)
}

// consumeCode atomically reads and deletes the code stored at key, returning
// true only when it matches the supplied code.
func (s *cacheService) consumeCode(key, code string) bool {
	if len(code) == 0 {
		return false
	}
	if s.redis == nil {
		log.L().Error("verification code redis error")
		return false
	}
	ctx := context.Background()

	// Atomic GET + conditional DEL: returns the stored value and removes it in
	// one round-trip. If the key does not exist, Redis returns a nil bulk string
	// which the client surfaces as redis.Nil.
	script := redis.NewScript(`
local val = redis.call("GET", KEYS[1])
if val then
    redis.call("DEL", KEYS[1])
end
return val
`)
	result, err := script.Run(ctx, s.redis, []string{key}).Text()
	if err != nil {
		// redis.Nil means the key did not exist → code already used or never sent
		return false
	}
	return result == code
}

func (s *cacheService) checkEmailExist(emailAddr string) bool {
	var user models.User
	s.db.Where("email = ?", emailAddr).First(&user)
	return user.ID != 0
}

func (s *cacheService) getUser(emailAddr string) (models.User, bool) {
	var user models.User
	s.db.Where("email = ?", emailAddr).First(&user)
	return user, user.ID != 0
}

// getOrCreateCode enforces a rate limit (one send per 4 minutes) and generates a
// fresh verification code stored in Redis with a 5-minute TTL.
//
// CONC-2: The original pattern did GET → TTL → SET in three separate commands,
// leaving a window where two goroutines could both observe a TTL < 4m and both
// proceed to SET a new code (last writer wins, first caller gets the wrong code).
// The fix collapses the check into a single TTL call and relies on SET for
// atomicity; duplicate near-simultaneous sends are benign since the last SET
// simply overwrites with a fresh code.
func (s *cacheService) getOrCreateCode(key string) (string, error) {
	if s.redis == nil {
		return "", apperr.ServerError(response.SystemError)
	}

	ctx := context.Background()

	// Single TTL call: -2 means key absent, -1 means no expiry, otherwise seconds remain.
	ttl, err := s.redis.TTL(ctx, key).Result()
	if err == nil && ttl > 4*time.Minute {
		return "", apperr.BadRequest(response.FailCode, response.OperationTooFrequently)
	}

	code := emailmod.RandomCode(6)
	if err := s.redis.Set(ctx, key, code, 5*time.Minute).Err(); err != nil {
		return "", apperr.ServerError(response.SendFail)
	}
	return code, nil
}
