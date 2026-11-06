package domain

import "fmt"

// PlatformRole determines platform-wide capabilities independently of project membership.
type PlatformRole string

const (
	PlatformRoleUser    PlatformRole = "user"
	PlatformRoleAnalyst PlatformRole = "analyst"
	PlatformRoleAdmin   PlatformRole = "admin"
)

// Validate rejects unknown roles instead of granting them implicit capabilities.
func (r PlatformRole) Validate() error {
	switch r {
	case PlatformRoleUser, PlatformRoleAnalyst, PlatformRoleAdmin:
		return nil
	default:
		return fmt.Errorf("invalid platform role %q", r)
	}
}
