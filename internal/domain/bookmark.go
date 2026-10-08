package domain

import (
	"time"

	"github.com/google/uuid"
)

type Bookmark struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	URL       string
	Title     string
	Tags      []Tag
	CreatedAt time.Time
	UpdatedAt time.Time
}