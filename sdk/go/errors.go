package sdk

import (
	"fmt"
)

type APIError struct {
	Status     int
	StatusText string
	Body       []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.Status, e.StatusText, string(e.Body))
}
