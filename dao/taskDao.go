package dao

import (
	"fmt"
	"github.com/hcloud-classic/hcc_errors"
	"github.com/hcloud-classic/pb"
	"github.com/mitchellh/go-ps"
	"hcc/tuba/lib/fileutil"
	"hcc/tuba/lib/syscheck"
	"hcc/tuba/model"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func modelTaskToPbTask(task *model.Task, skipRecursive bool) *pb.Task {
	var pChildren []*pb.Task
	var pThreads []*pb.Task

	modelProcess := &pb.Task{
		CMD:        task.CMD,
		State:      task.State,
		PID:        int64(task.PID),
		PPID:       int64(task.PPID),
		PGID:       int64(task.PGID),
		SID:        int64(task.SID),
		Priority:   int64(task.Priority),
		Nice:       int64(task.Nice),
		NumThreads: int64(task.NumThreads),
		StartTime:  task.StartTime,
		Children:   nil,
		Threads:    nil,
		CPUUsage:   task.CPUUsage,
		MemUsage:   task.MemUsage,
		EPMType:    task.EPMType,
		EPMSource:  int64(task.EPMSource),
		EPMTarget:  int64(task.EPMTarget),
	}

	if !skipRecursive {
		for i := range task.Children {
			pChildren = append(pChildren, modelTaskToPbTask(&task.Children[i], true))
		}
		modelProcess.Children = pChildren

		for i := range task.Threads {
			pThreads = append(pThreads, modelTaskToPbTask(&task.Threads[i], true))
		}
		modelProcess.Threads = pThreads
	}

	return modelProcess
}

func getStartTime(pid int) string {
	cmd := exec.Command("sh", "-c", "ps -o lstart= -p "+strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	value := strings.TrimSpace(string(out))

	return value
}

func getCPUUsage(pid int) string {
	cmd := exec.Command("sh", "-c",
		"ps up "+strconv.Itoa(pid)+" | tail -n1 | grep -iv \"%CPU\" | tr -s ' ' | cut -f3 -d' '")
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	trim := strings.TrimSpace(string(out))
	trimFloat, _ := strconv.ParseFloat(trim, 2)
	calcUsage := trimFloat / float64(runtime.NumCPU())
	value := fmt.Sprintf("%.1f", calcUsage)

	return value + "%"
}

func getTotalCPUUsage(tasks []model.Task) string {
	var usage float64 = 0

	for _, task := range tasks {
		taskCPUUsage := task.CPUUsage[:len(task.CPUUsage) - 1]
		usageFloat, _ := strconv.ParseFloat(taskCPUUsage, 2)

		usage += usageFloat
	}

	value := fmt.Sprintf("%.1f", usage)

	return value + "%"
}

func getMemUsage(task *model.Task, getByKB bool) string {
	cmd := exec.Command("sh", "-c",
		"cat /proc/"+strconv.Itoa(task.PID)+"/status | grep -iw VmRSS | tr -d '\\t' | tr -s ' ' | cut -f2 -d' '")
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	val := strings.TrimSpace(string(out))
	vmRssKB, _ := strconv.ParseInt(val, 0, 64)
	if !getByKB && vmRssKB > 999 {
		dot := vmRssKB % 1024
		vmRssMB := float32(vmRssKB) / 1024
		if vmRssMB > 999 {
			vmRssGB := vmRssMB / 1024
			if vmRssGB > 999 {
				vmRssTB := vmRssGB / 1024
				if dot != 0 {
					return fmt.Sprintf("%.2f", vmRssTB) + "TB"
				}

				return strconv.Itoa(int(vmRssTB)) + "TB"
			}
			if dot != 0 {
				return fmt.Sprintf("%.2f", vmRssGB) + "GB"
			}

			return strconv.Itoa(int(vmRssGB)) + "GB"
		}
		if dot != 0 {
			return fmt.Sprintf("%.2f", vmRssMB) + "MB"
		}

		return strconv.Itoa(int(vmRssMB)) + "MB"
	}

	return strconv.Itoa(int(vmRssKB)) + "KB"
}

func getMemInfo(content string) (int64, error) {
	cmd := exec.Command("sh", "-c",
		"cat /proc/meminfo | grep -iw "+content+" | tr -s ' ' | cut -f2 -d' '")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	val, err := strconv.ParseInt(strings.TrimSpace(string(out)), 0, 64)
	if err != nil {
		return 0, err
	}

	return val, nil
}

// getTotalMemUsage : Return total memory usage by parsed string and KB int
func getTotalMemUsage() (string, int64) {
	memTotal, err := getMemInfo("MemTotal")
	if err != nil {
		return "error", 0
	}
	memFree, err := getMemInfo("MemFree")
	if err != nil {
		return "error", 0
	}
	buffers, err := getMemInfo("Buffers")
	if err != nil {
		return "error", 0
	}
	cached, err := getMemInfo("Cached")
	if err != nil {
		return "error", 0
	}

	totalMemUsageKB := memTotal - (memFree + buffers + cached)

	if totalMemUsageKB > 999 {
		dot := totalMemUsageKB % 1024
		memTotalMB := float32(totalMemUsageKB) / 1024
		if memTotalMB > 999 {
			memTotalGB := memTotalMB / 1024
			if memTotalGB > 999 {
				memTotalTB := memTotalGB / 1024
				if dot != 0 {
					return fmt.Sprintf("%.2f", memTotalTB) + "TB", totalMemUsageKB
				}

				return strconv.Itoa(int(memTotalTB)) + "TB", totalMemUsageKB
			}
			if dot != 0 {
				return fmt.Sprintf("%.2f", memTotalGB) + "GB", totalMemUsageKB
			}

			return strconv.Itoa(int(memTotalGB)) + "GB", totalMemUsageKB
		}
		if dot != 0 {
			return fmt.Sprintf("%.2f", memTotalMB) + "MB", totalMemUsageKB
		}

		return strconv.Itoa(int(memTotalMB)) + "MB", totalMemUsageKB
	}

	return strconv.Itoa(int(totalMemUsageKB)) + "KB", totalMemUsageKB
}

// getTotalMem : Return total memory by parsed string and KB int
func getTotalMem() (string, int64) {
	cmd := exec.Command("sh", "-c",
		"cat /proc/meminfo | grep -iw MemTotal | tr -s ' ' | cut -f2 -d' '")
	out, err := cmd.Output()
	if err != nil {
		return "error", 0
	}

	val := strings.TrimSpace(string(out))
	memKB, _ := strconv.ParseInt(val, 0, 64)
	if memKB > 999 {
		memMB := memKB / 1000
		if memMB > 999 {
			memGB := memMB / 1000
			if memGB > 999 {
				memTB := memGB / 1000

				return strconv.Itoa(int(memTB)) + "TB", memKB
			}

			return strconv.Itoa(int(memGB)) + "GB", memKB
		}

		return strconv.Itoa(int(memMB)) + "MB", memKB
	}

	return strconv.Itoa(int(memKB)) + "KB", memKB
}

func getTotalMemUsagePercent(totalMemUsage int64, memTotal int64) string {
	calcUsage := float32(totalMemUsage) / float32(memTotal) * 100
	value := fmt.Sprintf("%.2f", calcUsage)

	return value + "%"
}

func readProcFile(filename string) ([]byte, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		// Reading from /proc/<PID> fails with ESRCH if the process has
		// been terminated between open() and read().
		if perr, ok := err.(*os.PathError); ok && perr.Err == syscall.ESRCH {
			return nil, nil
		}

		return nil, err
	}

	return data, nil
}

func getStatFromProc(pid int, task *model.Task) error {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	dataBytes, err := ioutil.ReadFile(statPath)
	if err != nil {
		return err
	}

	data := string(dataBytes)
	binStart := strings.IndexRune(data, '(') + 1
	binEnd := strings.IndexRune(data[binStart:], ')')
	task.CMD = data[binStart : binStart+binEnd]

	var ttyNr = 0
	var ttyPgrp = 0
	var flags = 0
	var minFlt = 0
	var cminFlt = 0
	var majFlt = 0
	var cmajFlt = 0
	var utime = 0
	var stime = 0
	var cutime = 0
	var cstime = 0

	data = data[binStart+binEnd+2:]
	_, err = fmt.Sscanf(data,
		"%s %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d",
		&task.State,
		&task.PPID,
		&task.PGID,
		&task.SID,
		&ttyNr,
		&ttyPgrp,
		&flags,
		&minFlt,
		&cminFlt,
		&majFlt,
		&cmajFlt,
		&utime,
		&stime,
		&cutime,
		&cstime,
		&task.Priority,
		&task.Nice,
		&task.NumThreads)

	return nil
}

func isProcExist(pid int) bool {
	return fileutil.IsFileOrDirectoryExist("/proc/" + strconv.Itoa(pid))
}

func getProcData(pid int, procData string) string {
	data, err := readProcFile("/proc/" + strconv.Itoa(pid) + "/" + procData)
	if err != nil {
		return "error"
	}
	if data == nil {
		return "error"
	}

	value := strings.TrimSpace(string(data))

	return value
}

func getThreadsPIDs(pid int) ([]int, error) {
	var threadsPIDs []int

	dirs, err := ioutil.ReadDir("/proc/" + strconv.Itoa(pid) + "/task")
	if err != nil {
		return nil, err
	}

	for i := range dirs {
		threadPID, _ := strconv.Atoi(dirs[i].Name())
		threadsPIDs = append(threadsPIDs, threadPID)
	}

	return threadsPIDs, nil
}

func findTaskByPID(tasks []model.Task, pid int) *model.Task {
	for i := range tasks {
		if tasks[i].PID == pid {
			return &tasks[i]
		}
	}

	return nil
}

func findTaskByPPID(tasks []model.Task, ppid int) *model.Task {
	for i := range tasks {
		if tasks[i].PPID == ppid {
			return &tasks[i]
		}
	}

	return nil
}

// ReadTaskList : Get list of task with selected infos
func ReadTaskList(in *pb.ReqGetTaskList) (*pb.ResGetTaskList, uint64, string) {
	var taskList pb.ResGetTaskList
	var pTasks []*pb.Task

	var modelTaskList []model.Task
	var emptyTaskList []model.Task

	var cmd string
	var cmdOk = false
	var state string
	var stateOk = false
	var pid int
	var pidOk = false
	var ppid int
	var ppidOk = false
	var pgid int
	var pgidOk = false
	var sid int
	var sidOk = false
	var priority int
	var priorityOk = false
	var nice int
	var niceOk = false
	var numThreads int
	var numThreadsOk = false
	var epmType string
	var epmTypeOk = false
	var epmSource int
	var epmSourceOk = false
	var epmTarget int
	var epmTargetOk = false

	if in.Task != nil {
		reqProcess := in.Task

		cmd = reqProcess.CMD
		cmdOk = len(cmd) != 0
		state = reqProcess.State
		stateOk = len(state) != 0
		pid = int(reqProcess.PID)
		pidOk = pid != 0
		ppid = int(reqProcess.PPID)
		ppidOk = ppid != 0
		pgid = int(reqProcess.PGID)
		pgidOk = pgid != 0
		sid = int(reqProcess.SID)
		sidOk = sid != 0
		priority = int(reqProcess.Priority)
		priorityOk = priority != 0
		nice = int(reqProcess.Nice)
		niceOk = nice != 0
		numThreads = int(reqProcess.NumThreads)
		numThreadsOk = numThreads != 0

		if syscheck.EPMProcSupported {
			epmType = reqProcess.EPMType
			epmTypeOk = len(epmType) != 0
			epmSource = int(reqProcess.EPMSource)
			epmSourceOk = epmSource != 0
			epmTarget = int(reqProcess.EPMTarget)
			epmTargetOk = epmTarget != 0
		}
	}

	pList, err := ps.Processes()
	if err != nil {
		return nil, hcc_errors.HccErrorTestCode, err.Error()
	}

	var wait sync.WaitGroup

	wait.Add(len(pList))
	for _, process := range pList {
		go func(routineProcess ps.Process, routineModelTaskList *[]model.Task) {
			var task model.Task
			var err error

			processPID := routineProcess.Pid()

			if !isProcExist(processPID) {
				goto OUT
			}

			task = model.Task{
				CMD:        "",
				State:      "",
				PID:        processPID,
				PPID:       routineProcess.PPid(),
				PGID:       0,
				SID:        0,
				Priority:   0,
				Nice:       0,
				NumThreads: 0,
				StartTime:  getStartTime(processPID),
				Children:   emptyTaskList,
				Threads:    emptyTaskList,
				CPUUsage:   getCPUUsage(processPID),
				MemUsage:   "0KB",
				EPMType:    "NOT_SUPPORTED",
				EPMSource:  0,
				EPMTarget:  0,
			}
			task.MemUsage = getMemUsage(&task, false)

			err = getStatFromProc(processPID, &task)
			if err != nil {
				fmt.Printf("getStatFromProc(): PID: %d, Error: %s\n", processPID, err.Error())
				goto OUT
			}

			if syscheck.EPMProcSupported {
				task.EPMType = getProcData(processPID, "epm_type")
				src, _ := strconv.Atoi(getProcData(processPID, "epm_source"))
				task.EPMSource = src
				target, _ := strconv.Atoi(getProcData(processPID, "epm_target"))
				task.EPMTarget = target
			}

			if cmdOk && cmd != task.CMD {
				goto OUT
			}
			if stateOk && state != task.State {
				goto OUT
			}
			if pidOk && pid != task.PID {
				goto OUT
			}
			if ppidOk && ppid != task.PPID {
				goto OUT
			}
			if pgidOk && pgid != task.PGID {
				goto OUT
			}
			if sidOk && sid != task.SID {
				goto OUT
			}
			if priorityOk && priority != task.Priority {
				goto OUT
			}
			if niceOk && nice != task.Nice {
				goto OUT
			}
			if numThreadsOk && numThreads != task.NumThreads {
				goto OUT
			}

			if syscheck.EPMProcSupported {
				if epmTypeOk && epmType != task.EPMType {
					goto OUT
				}
				if epmSourceOk && epmSource != task.EPMSource {
					goto OUT
				}
				if epmTargetOk && epmTarget != task.EPMTarget {
					goto OUT
				}
			}

			*routineModelTaskList = append(*routineModelTaskList, task)
		OUT:
			wait.Done()
		}(process, &modelTaskList)
	}
	wait.Wait()

	wait.Add(len(modelTaskList))
	for i := range modelTaskList {
		go func(routineModelTaskList []model.Task, routineI int) {
			var threadsPIDs []int

			parent := findTaskByPPID(routineModelTaskList, routineModelTaskList[routineI].PPID)

			if parent == nil {
				goto OUT
			}

			for i := range routineModelTaskList {
				if routineModelTaskList[i].PPID == routineModelTaskList[i].PID {
					continue
				}

				if parent.PID == routineModelTaskList[i].PPID {
					parent.Children = append(parent.Children, routineModelTaskList[i])
				}
			}

			threadsPIDs, err = getThreadsPIDs(routineModelTaskList[routineI].PID)
			if err != nil {
				fmt.Printf("getThreadsPIDs(): PID: %d, Error: %s\n", routineModelTaskList[routineI].PID, err.Error())
			}

			for _, threadPID := range threadsPIDs {
				thread := findTaskByPID(routineModelTaskList, threadPID)
				if thread == nil {
					continue
				}

				routineModelTaskList[routineI].Threads = append(routineModelTaskList[routineI].Threads, *thread)
			}

			pTasks = append(pTasks, modelTaskToPbTask(&routineModelTaskList[routineI], false))
		OUT:
			wait.Done()
		}(modelTaskList, i)
	}
	wait.Wait()

	taskList.Tasks = pTasks
	taskList.TotalTasks = int64(len(modelTaskList))
	var totalMemUsageKB int64 = 0
	taskList.TotalMemUsage, totalMemUsageKB = getTotalMemUsage()
	var totalMemKB int64 = 0
	taskList.TotalMem, totalMemKB = getTotalMem()
	taskList.TotalMemUsagePercent = getTotalMemUsagePercent(totalMemUsageKB, totalMemKB)
	taskList.TotalCPUUsage = getTotalCPUUsage(modelTaskList)

	return &taskList, 0, ""
}
