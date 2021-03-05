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
func IsTubaRunning() (err error, running bool, pid int) {
	if _, err := os.Stat(tubaPIDFile); os.IsNotExist(err) {
		return nil, false, 0
	}

	pidStr, err := ioutil.ReadFile(tubaPIDFile)
	if err != nil {
		return err, false, 0
	}

	tubaPID, _ := strconv.Atoi(string(pidStr))

	proc, err := os.FindProcess(tubaPID)
	if err != nil {
		return err, false, 0
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return nil, true, tubaPID
	}

	return nil, false, 0
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
