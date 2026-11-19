package contentref

import "time"

type Ref struct {
	ID        string
	Digest    string
	Bytes     int
	ExpiresAt time.Time
}
