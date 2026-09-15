package types

import "time"

type BucketInfo struct {
	Name         string    `json:"name"`
	CreationDate time.Time `json:"creationDate"`
}

type ObjectInfo struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
	IsFolder     bool      `json:"isFolder"`
	ContentType  string    `json:"contentType,omitempty"`
}

type CreateBucketReqDto struct {
	Name string `json:"name" binding:"required,nonblank"`
}

type PresignedURLReqDto struct {
	Bucket string `json:"bucket" binding:"required,nonblank"`
	Key    string `json:"key" binding:"required,nonblank"`
}

type PresignedURLRespDto struct {
	UploadURL   string `json:"uploadUrl,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
}
