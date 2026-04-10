package model

import "time"

type Image struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	FID         string    `json:"fid"`
	FileSize    int64     `json:"file_size"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	MimeType    string    `json:"mime_type"`
	PHash       int64     `json:"phash"`
	IsAnimated  bool      `json:"is_animated"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type ImageWithTags struct {
	Image
	Tags []Tag `json:"tags"`
}

type ImageListFilter struct {
	Tags     []string
	Page     int
	PageSize int
}

type ImageListResult struct {
	Items    []ImageWithTags `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int64           `json:"total"`
}

type UploadRequest struct {
	Filename string
	Data     []byte
	TagNames []string
	Force    bool
}

type UpdateImageDescriptionRequest struct {
	Description *string `json:"description"`
}

type RenderParams struct {
	Width   int
	Height  int
	Fit     string
	Quality int
	Format  string
}

type VectorMatch struct {
	ImageID int64
	Score   float32
}
