package response

import "github.com/gin-gonic/gin"

const (
	SuccessCode        = 2000
	FailCode           = 4000
	CheckFailCode      = 4220
	ServerErrorCode    = 5000
	UnauthorizedCode   = 4010
	ForbiddenCode      = 4030
	TokenExpiredCode   = 4011
	ResultNotFoundCode = 4040
)

// ResponseStruct is kept for backward compatibility with middleware and inline
// error responses while the migration is in progress.
type ResponseStruct struct {
	HttpStatus int
	Code       int
	Data       gin.H
	Msg        string
}

// HandleResponse writes a ResponseStruct to the gin context.
// Prefer response.Render for new code; this exists for migration compatibility.
func HandleResponse(c *gin.Context, res ResponseStruct) {
	c.JSON(res.HttpStatus, gin.H{"code": res.Code, "data": res.Data, "msg": res.Msg})
}
