package db

import (
	"os"

	"github.com/google/uuid"
)

func generateID() string {
	return uuid.New().String()
}

// readFileIfExists reads a file, returning (nil, nil) when it does not exist so
// callers can treat "no snapshot yet" as a non-error condition.
func readFileIfExists(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}
