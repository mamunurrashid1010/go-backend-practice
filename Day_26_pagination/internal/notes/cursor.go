package notes

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

// encodeCursor turns an int64 id into an opaque base64 string for the wire.
// It is NOT encryption — clients can decode it. The point is to discourage
// hand-built URLs that depend on the format, so we can change the cursor
// shape later (e.g. add a created_at component) without breaking clients.
func encodeCursor(id int64) string {
	if id <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

// decodeCursor returns the int64 inside an opaque cursor string. Empty
// string -> 0 (caller treats as "first page"). Garbage -> ErrInvalidCursor.
func decodeCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	id, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	if id <= 0 {
		return 0, ErrInvalidCursor
	}
	return id, nil
}
