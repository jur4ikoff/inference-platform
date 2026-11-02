package infrastructure

import (
	"time"

	"github.com/google/uuid"
	"github.com/jur4ikoff/user-service/internal/domain"
)

type IDGenerator struct{}

func (IDGenerator) NewUserID() domain.UserID {
	return domain.UserID(uuid.NewString())
}

type Clock struct{}

func (Clock) Now() time.Time {
	return time.Now()
}
