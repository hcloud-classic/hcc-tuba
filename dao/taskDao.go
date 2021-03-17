package dao

import (
	"encoding/json"
	"fmt"
	"github.com/hcloud-classic/hcc_errors"
	"github.com/hcloud-classic/pb"
	"hcc/tuba/lib/fileutil"
	"hcc/tuba/lib/syscheck"
	"hcc/tuba/model"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func getPIDList() ([]int, error) {
	var pidList []int

	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = d.Close()
	}()

	for {
		names, err := d.Readdirnames(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		var wait sync.WaitGroup
		var pidListAppendLock sync.Mutex

		wait.Add(len(names))
		for _, name := range names {
			go func(routineName string, routinePIDList *[]int) {
				var pid int64
				var err error

				// We only care if the name starts with a numeric
				if routineName[0] < '0' || routineName[0] > '9' {
					goto OUT
				}

				// From this point forward, any errors we just ignore, because
				// it might simply be that the process doesn't exist anymore.
				pid, err = strconv.ParseInt(routineName, 10, 0)
				if err != nil {
					goto OUT
				}

				pidListAppendLock.Lock()
				*routinePIDList = append(*routinePIDList, int(pid))
				pidListAppendLock.Unlock()
			OUT:
				wait.Done()
			}(name, &pidList)
		}
		wait.Wait()
	}

	return pidList, nil
}

func getProcessStartTime(pid int) string {
	cmd := exec.Command("sh", "-c", "ps -o lstart= -p "+strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	value := strings.TrimSpace(string(out))

	return value
}

func getThreadStartTime(pid int, spid int) string {
	cmd := exec.Command("sh", "-c", "ps -o spid=,lstart= -p "+strconv.Itoa(pid)+" -T | grep -w "+strconv.Itoa(spid))
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	value := strings.TrimSpace(string(out))
	value = strings.Replace(value, strconv.Itoa(spid)+" ", "", -1)

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
		taskCPUUsage := task.CPUUsage[:len(task.CPUUsage)-1]
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

func getCmdline(pid int) string {
	data, err := readProcFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return "error"
	}
	if data == nil {
		return "error"
	}

	for i := range data {
		if data[i] == '\u0000' {
			data[i] = '\u0020'
		}
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

func getTask(pid int) *model.Task {
	var emptyTaskList []model.Task

	if !isProcExist(pid) {
		fmt.Println("no pid", pid)
		return nil
	}

	task := model.Task{
		CMD:        "",
		State:      "",
		PID:        pid,
		PPID:       0,
		PGID:       0,
		SID:        0,
		Priority:   0,
		Nice:       0,
		NumThreads: 0,
		StartTime:  getProcessStartTime(pid),
		Children:   emptyTaskList,
		Threads:    emptyTaskList,
		CPUUsage:   getCPUUsage(pid),
		MemUsage:   "0KB",
		EPMType:    "NOT_SUPPORTED",
		EPMSource:  0,
		EPMTarget:  0,
		CMDLine:    getCmdline(pid),
	}
	task.MemUsage = getMemUsage(&task, false)

	err := getStatFromProc(pid, &task)
	if err != nil {
		fmt.Printf("getThread(): getStatFromProc(): PID: %d, Error: %s\n", pid, err.Error())
		return nil
	}

	if syscheck.EPMProcSupported {
		task.EPMType = getProcData(pid, "epm_type")
		src, _ := strconv.Atoi(getProcData(pid, "epm_source"))
		task.EPMSource = src
		target, _ := strconv.Atoi(getProcData(pid, "epm_target"))
		task.EPMTarget = target
	}

	return &task
}

func getThread(pid int, spid int) *model.Task {
	var emptyTaskList []model.Task

	if !isProcExist(spid) {
		fmt.Println("no thread", spid)
		return nil
	}

	task := model.Task{
		CMD:        "",
		State:      "",
		PID:        spid,
		PPID:       0,
		PGID:       0,
		SID:        0,
		Priority:   0,
		Nice:       0,
		NumThreads: 0,
		StartTime:  getThreadStartTime(pid, spid),
		Children:   emptyTaskList,
		Threads:    emptyTaskList,
		CPUUsage:   getCPUUsage(spid),
		MemUsage:   "0KB",
		EPMType:    "NOT_SUPPORTED",
		EPMSource:  0,
		EPMTarget:  0,
		CMDLine:    getCmdline(spid),
	}
	task.MemUsage = getMemUsage(&task, false)

	err := getStatFromProc(spid, &task)
	if err != nil {
		fmt.Printf("getThread(): getStatFromProc(): SPID: %d, Error: %s\n", spid, err.Error())
		return nil
	}

	if syscheck.EPMProcSupported {
		task.EPMType = getProcData(spid, "epm_type")
		src, _ := strconv.Atoi(getProcData(spid, "epm_source"))
		task.EPMSource = src
		target, _ := strconv.Atoi(getProcData(spid, "epm_target"))
		task.EPMTarget = target
	}

	return &task
}

func findTaskByPPID(tasks *[]model.Task, ppid int) *model.Task {
	for i := range *tasks {
		if (*tasks)[i].PID == ppid {
			return &(*tasks)[i]
		}

		children := &(*tasks)[i].Children
		task := findTaskByPPID(children, ppid)
		if task != nil {
			return task
		}
	}

	return nil
}

func makeTaskTree(tasks *[]model.Task) {
	var newTaskList []model.Task
	var parentIsExist []bool

	for i := range *tasks {
		if (*tasks)[i].PID == 1 {
			parentIsExist = append(parentIsExist, false)
			continue
		}

		fmt.Println("\n")
		fmt.Println("(*tasks)[i].CMD", (*tasks)[i].CMD)
		parent := findTaskByPPID(tasks, (*tasks)[i].PPID)
		if parent != nil {
			fmt.Println("parent.CMD", parent.CMD)
			parent.Children = append(parent.Children, (*tasks)[i])
			parentIsExist = append(parentIsExist, true)
		} else {
			parentIsExist = append(parentIsExist, false)
		}
	}

	for i := range parentIsExist {
		if !parentIsExist[i] {
			newTaskList = append(newTaskList, (*tasks)[i])
		}
	}

	*tasks = newTaskList
}

// ReadTaskList : Get list of task with selected infos
func ReadTaskList() (*pb.ResGetTaskList, uint64, string) {
	var resGetTaskList pb.ResGetTaskList
	var taskList model.TaskList

	var modelTaskList []model.Task

	pidList, err := getPIDList()
	if err != nil {
		return nil, hcc_errors.HccErrorTestCode, err.Error()
	}

	var wait sync.WaitGroup
	var taskListAppendLock sync.Mutex

	wait.Add(len(pidList))
	for _, p := range pidList {
		go func(routinePID int, routineModelTaskList *[]model.Task) {
			var task *model.Task
			var pid int
			var threadsPIDs []int

			task = getTask(routinePID)
			if task == nil {
				goto OUT
			}

			pid = task.PID
			threadsPIDs, err = getThreadsPIDs(pid)
			if err != nil {
				fmt.Printf("getThreadsPIDs(): PID: %d, Error: %s\n", pid, err.Error())
			}

			for _, threadPID := range threadsPIDs {
				if pid == threadPID {
					continue
				}

				thread := getThread(pid, threadPID)
				if thread == nil {
					continue
				}

				task.Threads = append(task.Threads, *thread)
			}

			taskListAppendLock.Lock()
			*routineModelTaskList = append(*routineModelTaskList, *task)
			taskListAppendLock.Unlock()
		OUT:
			wait.Done()
		}(p, &modelTaskList)
	}
	wait.Wait()

	taskList.TotalTasks = len(modelTaskList)
	var totalMemUsageKB int64 = 0
	taskList.TotalMemUsage, totalMemUsageKB = getTotalMemUsage()
	var totalMemKB int64 = 0
	taskList.TotalMem, totalMemKB = getTotalMem()
	taskList.TotalMemUsagePercent = getTotalMemUsagePercent(totalMemUsageKB, totalMemKB)
	taskList.TotalCPUUsage = getTotalCPUUsage(modelTaskList)

	makeTaskTree(&modelTaskList)
	taskList.Tasks = modelTaskList

	result, err := json.MarshalIndent(taskList, "", "    ")
	resGetTaskList.Result = string(result)
	//fmt.Println(resGetTaskList.Result)
	fmt.Println("\n\n\n")

	return &resGetTaskList, 0, ""
}
