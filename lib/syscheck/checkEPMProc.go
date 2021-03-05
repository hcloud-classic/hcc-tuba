package syscheck

import (
	"hcc/tuba/lib/fileutil"
)

// EPMProcSupported : Has value of EPMProc is supported by the kernel
var EPMProcSupported = false

func isEPMProcSupported() bool {
	if fileutil.IsFileOrDirectoryExist("/proc/1/epm_type") {
		return true
	}

	return false
}

// CheckEPMProc : Check if EPMProc is supported by the kernel
func CheckEPMProc() {
	if isEPMProcSupported() {
		EPMProcSupported = true
	}
}
