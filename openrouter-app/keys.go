package main

import (
	"fmt"
	"os"
	"strings"
)

func loadPrivateKeyPEM() (string, error) {
	if pem := os.Getenv("AUTH_PRIVATE_KEY"); strings.TrimSpace(pem) != "" {
		return pem, nil
	}

	path := strings.TrimSpace(os.Getenv("AUTH_PRIVATE_KEY_FILE"))
	if path == "" {
		return "", fmt.Errorf("AUTH_PRIVATE_KEY or AUTH_PRIVATE_KEY_FILE is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read AUTH_PRIVATE_KEY_FILE: %w", err)
	}

	pem := string(data)
	if strings.TrimSpace(pem) == "" {
		return "", fmt.Errorf("AUTH_PRIVATE_KEY_FILE is empty")
	}

	return pem, nil
}
