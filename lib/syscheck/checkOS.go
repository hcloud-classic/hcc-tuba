package syscheck

import (
	"errors"
	"runtime"
)

// CheckOS : Check OS then return error if not Linux or FreeBSD
func CheckOS() error {
	if runtime.GOOS != "linux" {
		return errors.New("tuba module only can run on linux machine")
	}

	return nil
}
