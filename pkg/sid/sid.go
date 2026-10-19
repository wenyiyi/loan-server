package sid

import (
	"encoding/base64"

	"github.com/google/uuid"
)

// GenUUID generates a 22-character URL-safe unique ID.
func GenUUID() string {
	id := uuid.New()
	return base64.RawURLEncoding.EncodeToString(id[:])
}
