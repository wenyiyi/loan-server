package model

// Identity is a trusted identity resolved from a configured bearer token.
type Identity struct {
	ID    string `json:"id" yaml:"id"`
	Role  string `json:"role" yaml:"role"`
	Email string `json:"email" yaml:"email"`
}
