package response

const (
	OK                     = "ok"
	ParameterError         = "parameter error"
	RequestError           = "request error"
	ForbiddenError         = "forbidden error"
	Forbidden              = "access denied: insufficient permissions"
	SystemError            = "system error"
	OperationTooFrequently = "operation too frequently"
	PageOrSizeError        = "page or request size error"
	TooManyRequests        = "too many requests"

	// Token-related Errors
	TokenExpired          = "token expired"
	Unauthorized          = "permission denied"
	TokenUnknownError     = "token unknown error"
	TokenNotValidYet      = "token not valid yet"
	TokenMalformed        = "token malformed"
	TokenSignatureInvalid = "token signature invalid"
	TokenInvalid          = "token invalid"

	// User Errors
	EmailRegistered       = "email is already registered"
	EmailFormatCheck      = "email format error"
	SendFail              = "email send fail"
	PasswordCheck         = "password cannot be less than 6 characters"
	VerificationCodeError = "verification code error"
	UserNotExist          = "user not exist"
	EmailOrPasswordError  = "email or password error"
	OldPasswordError      = "old password is incorrect"
	PasswordChangeDenied  = "password change is not available for externally authenticated accounts"
	PasswordResetDenied   = "password reset is not available for externally authenticated accounts"

	// Pipeline Errors
	PipelineRegistered       = "pipeline is already registered"
	PipelineNotExist         = "pipeline not exist"
	PipelineDeletionConflict = "pipeline deletion already in progress"
	PipelineUpdateError      = "pipeline update error"
	SchemaInvalidRepo        = "invalid pipeline repository url"
	SchemaInvalidVersion     = "invalid pipeline version"
	SchemaNotFound           = "nextflow_schema.json not found for this pipeline version"
	SchemaFetchError         = "failed to fetch pipeline schema"

	// Job Errors
	JobDispatchError = "nomad job dispatch failed"
	JobNotFound      = "job not found"
	JobCreateError   = "job creation failed in database"
	JobDeletionError = "job deletion failed in database"
	JobNotStoppable  = "job is not running on nomad and cannot be stopped"
	JobStopError     = "failed to stop job on nomad"

	// OSS Errors
	BucketNotFound       = "bucket not found"
	ObjectDeleteError    = "object deletion failed"
	StorageNotConfigured = "no storage configured, please configure your storage in User Settings first"
)
