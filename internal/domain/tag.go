package domain

import (
	"time"

	"github.com/google/uuid"
)

type Tag struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	CreatedAt time.Time
}