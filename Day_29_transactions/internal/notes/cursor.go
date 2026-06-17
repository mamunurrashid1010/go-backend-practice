package notes

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

func encodeCursor(id int64) string {
	if id <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

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
