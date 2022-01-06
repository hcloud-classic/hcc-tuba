package syscheck

import (
	"hcc/tuba/lib/fileutil"
)

// GPMProcSupported : Has value of GPMProc is supported by the kernel
var GPMProcSupported = false

func isGPMProcSupported() bool {
	if fileutil.IsFileOrDirectoryExist("/proc/1/gpm_type") {
		return true
	}

	return false
}

// CheckGPMProc : Check if GPMProc is supported by the kernel
func CheckGPMProc() {
	if isGPMProcSupported() {
		GPMProcSupported = true
	}
}
