package model

import "time"

type Tag struct {
	ID        int32     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateTagRequest struct {
	Name string `json:"name"`
}
