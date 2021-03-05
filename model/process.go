package model

import (
	"github.com/hcloud-classic/hcc_errors"
)

// Process : Struct of process
type Process struct {
	PID       int    `json:"pid"`
	PPID      int    `json:"ppid"`
	EXE       string `json:"exe"`
	CPUUsage  string `json:"cpu_usage"`
	Stat      string `json:"stat"`
	EPMType   string `json:"epm_type"`
	EPMSource int    `json:"epm_source"`
	EPMTarget int    `json:"epm_target"`
}

// Processes : Array struct of processes
type Processes struct {
	Processes []Process                `json:"process"`
	Errors    hcc_errors.HccErrorStack `json:"errors"`
}
