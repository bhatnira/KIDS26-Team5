package validation

import (
	"os"
	"strings"

	"antelope/internal/modules/log"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

type ReqValidator struct{}

func (rv *ReqValidator) RegisterCustomizedValidator(tag string, vFunc validator.Func) (err error) {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		err = v.RegisterValidation(tag, vFunc)
	}
	return err
}

func NonBlankValidator(fl validator.FieldLevel) bool {
	return strings.TrimSpace(fl.Field().String()) != ""
}

func RegisterValidators() {
	var rv ReqValidator
	// register customized validators
	err := rv.RegisterCustomizedValidator("nonblank", NonBlankValidator)
	// add more validators here
	if err != nil {
		log.L().Error("register validator failed", zap.Error(err))
		os.Exit(0)
	}

	log.L().Info("register validator success")
}
