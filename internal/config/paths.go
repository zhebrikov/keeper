package config

import (
	"os"
	"path/filepath"
)

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gophkeeper"
	}
	return filepath.Join(home, ".gophkeeper")
}
