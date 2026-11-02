package service

// PasswordHasher keeps password-algorithm details out of use cases.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}
