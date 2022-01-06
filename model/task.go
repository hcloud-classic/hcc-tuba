package model

import (
	"innogrid.com/hcloud-classic/hcc_errors"
)

// Task : Struct of task
type Task struct {
	CMD        string `json:"cmd"`
	State      string `json:"state"`
	PID        int    `json:"pid"`
	User       string `json:"user"`
	PPID       int    `json:"ppid"`
	Priority   int    `json:"priority"`
	Nice       int    `json:"nice"`
	Time       string `json:"time"`
	Children   []Task `json:"children"`
	Threads    []Task `json:"threads"`
	CPUUsage   string `json:"cpu_usage"`
	MemVirt    string `json:"mem_virt"`
	MemRes     string `json:"mem_res"`
	MemShr     string `json:"mem_shr"`
	MemPercent string `json:"mem_percent"`
	GPMTarget  int    `json:"gpm_target"`
	CMDLine    string `json:"cmdline"`
	IsThread   bool   `json:"is_thread"`
}

// TaskList : Array struct of tasks
type TaskList struct {
	Tasks        []Task `json:"task_list"`
	TotalTasks   int    `json:"total_tasks"`
	TotalThreads int    `json:"total_threads"`
}

// TaskListResult : Array struct of taskListResult
type TaskListResult struct {
	Result string                   `json:"result"`
	Errors hcc_errors.HccErrorStack `json:"errors"`
}
