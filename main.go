package main

import (
	"fmt"
	"hcc/tuba/action/grpc/server"
	"hcc/tuba/dao"
	"hcc/tuba/lib/config"
	"hcc/tuba/lib/logger"
	"hcc/tuba/lib/pid"
	"hcc/tuba/lib/syscheck"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"innogrid.com/hcloud-classic/hcc_errors"
)

func init() {
	tubaRunning, tubaPID, err := pid.IsTubaRunning()
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	if tubaRunning {
		fmt.Println("tuba is already running. (PID: " + strconv.Itoa(tubaPID) + ")")
		os.Exit(1)
	}
	err = pid.WriteTubaPID()
	if err != nil {
		_ = pid.DeleteTubaPID()
		fmt.Println(err)
		panic(err)
	}

	err = syscheck.CheckOS()
	if err != nil {
		fmt.Println("Please run tuba module on the Linux machine.")
		_ = pid.DeleteTubaPID()
		panic(err)
	}

	err = syscheck.GetPageShift()
	if err != nil {
		fmt.Println(err.Error())
		_ = pid.DeleteTubaPID()
		panic(err)
	}

	err = syscheck.IncreaseRLimitToMax()
	if err != nil {
		fmt.Println(err.Error())
		_ = pid.DeleteTubaPID()
		panic(err)
	}

	syscheck.CheckGPMProc()
	if syscheck.GPMProcSupported {
		fmt.Println("GPMProc is supported by the kernel")
	} else {
		fmt.Println("GPMProc is not supported in this system")
	}

	err = syscheck.CheckRoot()
	if err != nil {
		_ = pid.DeleteTubaPID()
		panic(err)
	}

	err = dao.GetTotalMem()
	if err != nil {
		fmt.Println("Failed to get total size of memory.")
		_ = pid.DeleteTubaPID()
		panic(err)
	}

	err = logger.Init()
	if err != nil {
		hcc_errors.SetErrLogger(logger.Logger)
		hcc_errors.NewHccError(hcc_errors.HarpInternalInitFail, "logger.Init(): "+err.Error()).Fatal()
		_ = pid.DeleteTubaPID()
	}
	hcc_errors.SetErrLogger(logger.Logger)

	config.Init()
}

func end() {
	logger.End()
	_ = pid.DeleteTubaPID()
}

func main() {
	// Catch the exit signal
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		end()
		fmt.Println("Exiting tuba module...")
		os.Exit(0)
	}()

	server.Init()
}
