package dao

import (
	"encoding/json"
	"errors"
	"fmt"
	"hcc/tuba/lib/fileutil"
	"hcc/tuba/lib/syscheck"
	"hcc/tuba/model"
	"innogrid.com/hcloud-classic/hcc_errors"
	"innogrid.com/hcloud-classic/pb"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var modelTaskList []model.Task

type pid struct {
	pid   int
	isNew bool
}

func getOldPIDList(taskList *[]model.Task) []int {
	var pidList []int

	for i := range *taskList {
		pidList = append(pidList, (*taskList)[i].PID)
		pidList = append(pidList, getOldPIDList(&(*taskList)[i].Children)...)
	}

	return pidList
}

func isPIDExist(oldPIDList *[]int, pid int) bool {
	for _, _pid := range *oldPIDList {
		if _pid == pid {
			return true
		}
	}

	return false
}

func getPIDList() ([]pid, error) {
	var pidList []pid
	var oldPIDList = getOldPIDList(&modelTaskList)

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
			go func(routineName string) {
				var _pid int64
				var pidExist bool
				var err error

				// We only care if the name starts with a numeric
				if routineName[0] < '0' || routineName[0] > '9' {
					goto OUT
				}

				// From this point forward, any errors we just ignore, because
				// it might simply be that the process doesn't exist anymore.
				_pid, err = strconv.ParseInt(routineName, 10, 0)
				if err != nil {
					goto OUT
				}

				pidListAppendLock.Lock()
				pidExist = isPIDExist(&oldPIDList, int(_pid))
				pidList = append(pidList, pid{
					pid:   int(_pid),
					isNew: pidExist,
				})
				pidListAppendLock.Unlock()
			OUT:
				wait.Done()
			}(name)
		}
		wait.Wait()
	}

	return pidList, nil
}

func findTaskFromModelTaskList(pid int) *model.Task {
	for _, modelTask := range modelTaskList {
		if modelTask.PID == pid {
			return &modelTask
		}
	}

	return nil
}

func findThreadFromModelTaskList(pid int) *model.Task {
	for _, modelTask := range modelTaskList {
		for _, thread := range modelTask.Threads {
			if thread.PID == pid {
				return &thread
			}
		}
	}

	return nil
}

func getUserID(pid int) string {
	cmd := exec.Command("sh", "-c", "stat -c %u /proc/"+strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	value := strings.TrimSpace(string(out))

	return value
}

func getUserName(pid int) string {
	cmd := exec.Command("sh", "-c", "stat -c %U /proc/"+strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	value := strings.TrimSpace(string(out))
	if value == "UNKNOWN" {
		return getUserID(pid)
	}

	return value
}

func getProcessTime(pid int) string {
	cmd := exec.Command("sh", "-c", "ps -o time= -p "+strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	value := strings.TrimSpace(string(out))

	return value
}

func getThreadTime(pid int, spid int) string {
	cmd := exec.Command("sh", "-c", "ps -o spid=,time= -p "+strconv.Itoa(pid)+" -T | grep -w "+strconv.Itoa(spid))
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

	if procData == "cmdline" {
		for i := range data {
			if data[i] == '\u0000' {
				data[i] = '\u0020'
			}
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

func getTask(pid pid) *model.Task {
	var emptyTaskList []model.Task

	_pid := pid.pid

	if !isProcExist(_pid) {
		fmt.Println("no pid", _pid)
		return nil
	}

	task := model.Task{
		CMD:        "",
		State:      "",
		PID:        _pid,
		User:       getUserName(_pid),
		PPID:       0,
		PGID:       0,
		SID:        0,
		Priority:   0,
		Nice:       0,
		NumThreads: 0,
		Children:   emptyTaskList,
		Threads:    emptyTaskList,
		CPUUsage:   getCPUUsage(_pid),
		MemUsage:   "0KB",
		EPMType:    "NOT_SUPPORTED",
		EPMSource:  0,
		EPMTarget:  0,
		IsThread:   false,
	}

	if pid.isNew {
		task.Time = getProcessTime(_pid)
		task.CMDLine = getProcData(_pid, "cmdline")
	} else {
		tsk := findTaskFromModelTaskList(_pid)
		if tsk != nil {
			task.Time = tsk.Time
			task.CMDLine = tsk.CMDLine
		} else {
			task.Time = getProcessTime(_pid)
			task.CMDLine = getProcData(_pid, "cmdline")
		}
	}

	task.MemUsage = getMemUsage(&task, false)

	err := getStatFromProc(_pid, &task)
	if err != nil {
		fmt.Printf("getThread(): getStatFromProc(): PID: %d, Error: %s\n", pid, err.Error())
		return nil
	}

	if syscheck.EPMProcSupported {
		task.EPMType = getProcData(_pid, "epm_type")
		src, _ := strconv.Atoi(getProcData(_pid, "epm_source"))
		task.EPMSource = src
		if task.EPMType == "EPM_NO_ACTION" {
			task.EPMTarget = src
		} else {
			target, _ := strconv.Atoi(getProcData(_pid, "epm_target"))
			task.EPMTarget = target
		}
	}

	return &task
}

func getThread(parent *model.Task, spid int, isNew bool) *model.Task {
	var emptyTaskList []model.Task

	if !isProcExist(spid) {
		fmt.Println("no thread", spid)
		return nil
	}

	task := model.Task{
		CMD:        "",
		State:      "",
		PID:        spid,
		User:       parent.User,
		PPID:       0,
		PGID:       0,
		SID:        0,
		Priority:   0,
		Nice:       0,
		NumThreads: 0,
		Children:   emptyTaskList,
		Threads:    emptyTaskList,
		CPUUsage:   getCPUUsage(spid),
		MemUsage:   parent.MemUsage,
		EPMType:    "NOT_SUPPORTED",
		EPMSource:  0,
		EPMTarget:  0,
		IsThread:   true,
	}

	if isNew {
		task.Time = getThreadTime(parent.PID, spid)
		task.CMDLine = getProcData(spid, "cmdline")
	} else {
		tsk := findThreadFromModelTaskList(spid)
		if tsk != nil {
			task.Time = tsk.Time
			task.CMDLine = tsk.CMDLine
		} else {
			task.Time = getThreadTime(parent.PID, spid)
			task.CMDLine = getProcData(spid, "cmdline")
		}
	}

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

func isThreadExist(oldThreadPIDs *[]int, threadPID int) bool {
	for _, pid := range *oldThreadPIDs {
		if pid == threadPID {
			return true
		}
	}

	return false
}

func deleteTaskFromTaskList(tasks *[]model.Task, pid int) {
	var newTaskList []model.Task

	for i := range *tasks {
		if (*tasks)[i].PID == pid {
			continue
		}

		newTaskList = append(newTaskList, (*tasks)[i])
	}

	*tasks = newTaskList
}

func attachChildToParent(tasks *[]model.Task, child *model.Task, ppid int) (attached bool) {
	for i := range *tasks {
		children := &(*tasks)[i].Children

		if (*tasks)[i].PID == ppid {
			*children = append(*children, *child)

			return true
		}

		if attachChildToParent(children, child, ppid) {
			return true
		}
	}

	return false
}

func checkSortingMethod(sortBy string) error {
	sortBy = strings.ToLower(sortBy)

	switch sortBy {
	case "cmd", "state", "pid", "user", "ppid", "pgid", "sid", "priority", "nice", "time", "cpu_usage", "mem_usage", "emp_type", "epm_target", "epm_source", "cmdline":
		goto OUT
	default:
		return errors.New("unknown sorting method")
	}

OUT:
	return nil
}

func sortTaskList(taskList *[]model.Task, sortBy string, reverse bool) error {
	sortBy = strings.ToLower(sortBy)

	switch sortBy {
	case "cmd":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return strings.ToLower((*taskList)[i].CMD) > strings.ToLower((*taskList)[j].CMD)
			}
			return strings.ToLower((*taskList)[i].CMD) < strings.ToLower((*taskList)[j].CMD)
		})
	case "state":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].State > (*taskList)[j].State
			}
			return (*taskList)[i].State < (*taskList)[j].State
		})
	case "pid":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].PID > (*taskList)[j].PID
			}
			return (*taskList)[i].PID < (*taskList)[j].PID
		})
	case "user":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].User > (*taskList)[j].User
			}
			return (*taskList)[i].User < (*taskList)[j].User
		})
	case "ppid":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].PPID > (*taskList)[j].PPID
			}
			return (*taskList)[i].PPID < (*taskList)[j].PPID
		})
	case "pgid":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].PGID > (*taskList)[j].PGID
			}
			return (*taskList)[i].PGID < (*taskList)[j].PGID
		})
	case "sid":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].SID > (*taskList)[j].SID
			}
			return (*taskList)[i].SID < (*taskList)[j].SID
		})
	case "priority":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].Priority > (*taskList)[j].Priority
			}
			return (*taskList)[i].Priority < (*taskList)[j].Priority
		})
	case "nice":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].Nice > (*taskList)[j].Nice
			}
			return (*taskList)[i].Nice < (*taskList)[j].Nice
		})
	case "time":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].Time > (*taskList)[j].Time
			}
			return (*taskList)[i].Time < (*taskList)[j].Time
		})
	case "cpu_usage":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].CPUUsage > (*taskList)[j].CPUUsage
			}
			return (*taskList)[i].CPUUsage < (*taskList)[j].CPUUsage
		})
	case "mem_usage":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].MemUsage > (*taskList)[j].MemUsage
			}
			return (*taskList)[i].MemUsage < (*taskList)[j].MemUsage
		})
	case "epm_type":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].EPMType > (*taskList)[j].EPMType
			}
			return (*taskList)[i].EPMType < (*taskList)[j].EPMType
		})
	case "epm_target":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].EPMTarget > (*taskList)[j].EPMTarget
			}
			return (*taskList)[i].EPMTarget < (*taskList)[j].EPMTarget
		})
	case "epm_source":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].EPMSource > (*taskList)[j].EPMSource
			}
			return (*taskList)[i].EPMSource < (*taskList)[j].EPMSource
		})
	case "cmdline":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return strings.ToLower((*taskList)[i].CMDLine) > strings.ToLower((*taskList)[j].CMDLine)
			}
			return strings.ToLower((*taskList)[i].CMDLine) < strings.ToLower((*taskList)[j].CMDLine)
		})
	default:
		return errors.New("unknown sorting method")
	}

	return nil
}

func makeTaskTree(tasks *[]model.Task) []model.Task {
	var newTasks = *tasks

	for {
		var listChanged = false

		for i := range newTasks {
			if newTasks[i].PID == 1 {
				continue
			}

			attached := attachChildToParent(&newTasks, &newTasks[i], newTasks[i].PPID)
			if attached {
				deleteTaskFromTaskList(&newTasks, newTasks[i].PID)
				listChanged = true
				break
			}
		}

		if !listChanged {
			break
		}
	}

	return newTasks
}

// ReadTaskList : Get list of task with selected infos
func ReadTaskList(reqGetTaskList *pb.ReqGetTaskList) (*pb.ResGetTaskList, uint64, string) {
	var newModelTaskList []model.Task
	var resGetTaskList pb.ResGetTaskList
	var taskList model.TaskList
	var sortingMethod = reqGetTaskList.GetSortBy()
	var needSorting = false

	if sortingMethod != "" {
		err := checkSortingMethod(sortingMethod)
		if err != nil {
			return nil, hcc_errors.HccErrorTestCode, err.Error()
		}
		needSorting = true
	}

	pidList, err := getPIDList()
	if err != nil {
		return nil, hcc_errors.HccErrorTestCode, err.Error()
	}

	var wait sync.WaitGroup
	var taskListAppendLock sync.Mutex

	wait.Add(len(pidList))
	for _, p := range pidList {
		go func(routinePID pid) {
			var task *model.Task
			var pid int
			var threadsPIDs []int
			var oldThreadsPIDs []int

			task = getTask(routinePID)
			if task == nil {
				goto OUT
			}

			if !routinePID.isNew {
				oldTask := findTaskFromModelTaskList(task.PID)
				if oldTask != nil {
					for _, thread := range oldTask.Threads {
						oldThreadsPIDs = append(oldThreadsPIDs, thread.PID)
					}
				}
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

				isNew := isThreadExist(&oldThreadsPIDs, threadPID)
				thread := getThread(task, threadPID, isNew)
				if thread == nil {
					continue
				}

				if needSorting {
					taskListAppendLock.Lock()
					newModelTaskList = append(newModelTaskList, *thread)
					taskListAppendLock.Unlock()
				} else {
					task.Threads = append(task.Threads, *thread)
				}
			}

			taskListAppendLock.Lock()
			newModelTaskList = append(newModelTaskList, *task)
			taskListAppendLock.Unlock()
		OUT:
			wait.Done()
		}(p)
	}
	wait.Wait()

	modelTaskList = newModelTaskList

	taskList.TotalTasks = len(modelTaskList)
	var totalMemUsageKB int64 = 0
	taskList.TotalMemUsage, totalMemUsageKB = getTotalMemUsage()
	var totalMemKB int64 = 0
	taskList.TotalMem, totalMemKB = getTotalMem()
	taskList.TotalMemUsagePercent = getTotalMemUsagePercent(totalMemUsageKB, totalMemKB)
	taskList.TotalCPUUsage = getTotalCPUUsage(modelTaskList)

	if needSorting {
		err := sortTaskList(&modelTaskList, sortingMethod, reqGetTaskList.GetReverseSorting())
		if err != nil {
			return nil, hcc_errors.HccErrorTestCode, err.Error()
		}
		taskList.Tasks = modelTaskList
	} else {
		taskList.Tasks = makeTaskTree(&modelTaskList)
	}

	result, err := json.Marshal(taskList)
	resGetTaskList.Result = result

	return &resGetTaskList, 0, ""
}
