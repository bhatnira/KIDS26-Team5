package response

import (
	"errors"
	"net/http"

	"antelope/pkg/apperr"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Response(c *gin.Context, httpStatus, code int, data gin.H, msg string) {
	c.JSON(httpStatus, gin.H{"code": code, "data": data, "msg": msg})
}

func Success(c *gin.Context, data gin.H, msg string) {
	Response(c, http.StatusOK, SuccessCode, data, msg)
}

func Fail(c *gin.Context, data gin.H, msg string) {
	Response(c, http.StatusBadRequest, FailCode, data, msg)
}

func CheckFail(c *gin.Context, data gin.H, msg string) {
	Response(c, http.StatusUnprocessableEntity, CheckFailCode, data, msg)
}

func ServerError(c *gin.Context, data gin.H, msg string) {
	Response(c, http.StatusInternalServerError, ServerErrorCode, data, msg)
}

// Render is the single exit point for all handler responses.
// Services return (data, error); handlers call response.Render(c, data, err).
// On success (err == nil) it writes 200 + data. On *apperr.AppError it maps to
// the structured status/code/msg. Unknown errors produce a generic 500.
func Render(c *gin.Context, data gin.H, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"code": SuccessCode, "data": data, "msg": OK})
		return
	}
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, gin.H{"code": appErr.Code, "data": nil, "msg": appErr.Msg})
		return
	}
	zap.L().Error("unhandled service error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"code": ServerErrorCode, "data": nil, "msg": SystemError})
}
