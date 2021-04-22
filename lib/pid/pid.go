package pid

import (
	"hcc/tuba/lib/fileutil"
	"io/ioutil"
	"os"
	"strconv"
	"syscall"
)

var tubaPIDFileLocation = "/var/run"
var tubaPIDFile = "/var/run/tuba.pid"

// IsTubaRunning : Check if tuba is running
func IsTubaRunning() (running bool, pid int, err error) {
	if _, err := os.Stat(tubaPIDFile); os.IsNotExist(err) {
		return false, 0, nil
	}

	pidStr, err := ioutil.ReadFile(tubaPIDFile)
	if err != nil {
		return false, 0, err
	}

	tubaPID, _ := strconv.Atoi(string(pidStr))

	proc, err := os.FindProcess(tubaPID)
	if err != nil {
		return false, 0, err
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true, tubaPID, nil
	}

	return false, 0, nil
}

// WriteTubaPID : Write tuba PID to "/var/run/tuba.pid"
func WriteTubaPID() error {
	pid := os.Getpid()

	err := fileutil.CreateDirIfNotExist(tubaPIDFileLocation)
	if err != nil {
		return err
	}

	err = fileutil.WriteFile(tubaPIDFile, strconv.Itoa(pid))
	if err != nil {
		return err
	}

	return nil
}

// DeleteTubaPID : Delete the tuba PID file
func DeleteTubaPID() error {
	err := fileutil.DeleteFile(tubaPIDFile)
	if err != nil {
		return err
	}

	return nil
}
