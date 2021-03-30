package syscheck

import (
	"errors"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

// PageShift : Page shift value of currently running linux machine
var PageShift int

// GetPageShift : Get page shift value of currently running linux machine
func GetPageShift() error {
	cmd := exec.Command("getconf", "PAGESIZE")
	out, err := cmd.Output()
	if err != nil {
		return errors.New("failed to determine page size")
	}

	value := strings.TrimSpace(string(out))
	pageSize, _ := strconv.ParseInt(value, 0, 64)
	PageShift = int(math.Log2(float64(pageSize)))

	return nil
}
