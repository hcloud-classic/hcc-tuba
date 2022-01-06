package dao

import (
	"encoding/json"
	"errors"
	"fmt"
	"hcc/tuba/lib/fileutil"
	"hcc/tuba/lib/syscheck"
	"hcc/tuba/model"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"innogrid.com/hcloud-classic/hcc_errors"
	"innogrid.com/hcloud-classic/pb"
)

var modelTaskList []model.Task

// TotalMem : Total size of memory
var TotalMem int64

// GetTotalMem : Return total size memory by KB int64
func GetTotalMem() error {
	cmd := exec.Command("sh", "-c",
		"cat /proc/meminfo | grep -iw MemTotal | tr -s ' ' | cut -f2 -d' '")
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	val := strings.TrimSpace(string(out))
	memKB, _ := strconv.ParseInt(val, 0, 64)

	TotalMem = memKB

	return nil
}

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

	names, err := d.Readdirnames(0)
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
		"ps up "+strconv.Itoa(pid)+" | tail -n1 | tr -s ' ' | cut -f3 -d' '")
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

func getSize(kbValue int) string {
	if kbValue > 999 {
		dot := kbValue % 1024
		resultMB := float32(kbValue) / 1024
		if resultMB > 999 {
			resultGB := resultMB / 1024
			if resultGB > 999 {
				resultTB := resultGB / 1024
				if dot != 0 {
					return fmt.Sprintf("%.2f", resultTB) + "TB"
				}

				return strconv.Itoa(int(resultTB)) + "TB"
			}
			if dot != 0 {
				return fmt.Sprintf("%.2f", resultGB) + "GB"
			}

			return strconv.Itoa(int(resultGB)) + "GB"
		}
		if dot != 0 {
			return fmt.Sprintf("%.2f", resultMB) + "MB"
		}

		return strconv.Itoa(int(resultMB)) + "MB"
	}

	return strconv.Itoa(kbValue) + "KB"
}

func getMemUsagePercent(memUsage int64) string {
	calcUsage := float32(memUsage) / float32(TotalMem) * 100
	value := fmt.Sprintf("%.2f", calcUsage)

	return value + "%"
}

func getProcMemory(task *model.Task) {
	out := getProcData(task.PID, "status")

	out = strings.Replace(out, "\t", " ", -1)
	out = strings.Replace(out, " ", "", -1)
	lines := strings.Split(out, "\n")

	for _, line := range lines {
		if strings.Contains(line, "VmRSS") {
			val := strings.Split(line, ":")
			if len(val) == 2 {
				memRes, _ := strconv.ParseInt(val[1][:len(val[1])-2], 0, 64)
				task.MemRes = getSize(int(memRes))
				task.MemPercent = getMemUsagePercent(memRes)
			}
		} else if strings.Contains(line, "VmSize") {
			val := strings.Split(line, ":")
			if len(val) == 2 {
				memVirt, _ := strconv.ParseInt(val[1][:len(val[1])-2], 0, 64)
				task.MemVirt = getSize(int(memVirt))
			}
		}
	}

	out = getProcData(task.PID, "statm")
	val := strings.Split(out, " ")
	if len(val) >= 3 {
		memShr, _ := strconv.ParseInt(val[2], 0, 64)
		memShr = memShr << (syscheck.PageShift - 10)
		task.MemShr = getSize(int(memShr))
	}
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

	var dummy = 0

	data = data[binStart+binEnd+2:]
	_, err = fmt.Sscanf(data,
		"%s %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d",
		&task.State,
		&task.PPID,
		&dummy, // pgid
		&dummy, // sid
		&dummy, // tty_nr
		&dummy, // tty_pgrp
		&dummy, // flags
		&dummy, // min_flt
		&dummy, // cmin_flt
		&dummy, // maj_flt
		&dummy, // cmaj_flt
		&dummy, // utime
		&dummy, // stime
		&dummy, // cutime
		&dummy, // cstime
		&task.Priority,
		&task.Nice)

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

var maxTaskReadCount int64 = 10
var taskReadCounter int64

func incTaskReadCounter() {
	atomic.AddInt64(&taskReadCounter, 1)
}

func decTaskReadCounter() {
	atomic.AddInt64(&taskReadCounter, -1)
}

func getTask(pid pid) *model.Task {
	for true {
		if taskReadCounter >= maxTaskReadCount {
			time.Sleep(100 * time.Millisecond)
		} else {
			break
		}
	}

	incTaskReadCounter()
	defer func() {
		decTaskReadCounter()
	}()

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
		Priority:   0,
		Nice:       0,
		Children:   emptyTaskList,
		Threads:    emptyTaskList,
		CPUUsage:   getCPUUsage(_pid),
		MemVirt:    "error",
		MemRes:     "error",
		MemShr:     "error",
		MemPercent: "error",
		GPMTarget:  0,
		IsThread:   false,
	}

	getProcMemory(&task)

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

	err := getStatFromProc(_pid, &task)
	if err != nil {
		fmt.Printf("getTask(): getStatFromProc(): PID: %d, Error: %s\n", _pid, err.Error())
		return nil
	}

	if syscheck.GPMProcSupported {
		target, _ := strconv.Atoi(getProcData(_pid, "gpm_target"))
		task.GPMTarget = target
	}

	return &task
}

func getThread(parent *model.Task, spid int, isNew bool) *model.Task {
	for true {
		if taskReadCounter >= maxTaskReadCount {
			time.Sleep(100 * time.Millisecond)
		} else {
			break
		}
	}

	incTaskReadCounter()
	defer func() {
		decTaskReadCounter()
	}()

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
		Priority:   0,
		Nice:       0,
		Children:   emptyTaskList,
		Threads:    emptyTaskList,
		CPUUsage:   getCPUUsage(spid),
		MemVirt:    parent.MemVirt,
		MemRes:     parent.MemRes,
		MemShr:     parent.MemShr,
		MemPercent: parent.MemPercent,
		GPMTarget:  0,
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

	if syscheck.GPMProcSupported {
		target, _ := strconv.Atoi(getProcData(spid, "gpm_target"))
		task.GPMTarget = target
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
	case "cmd", "state", "pid", "user", "ppid", "priority", "nice", "time", "cpu_usage", "mem_virt", "mem_res", "mem_shr", "mem_percent", "gpm_target", "cmdline":
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
	case "mem_virt":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].MemVirt > (*taskList)[j].MemVirt
			}
			return (*taskList)[i].MemVirt < (*taskList)[j].MemVirt
		})
	case "mem_res":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].MemRes > (*taskList)[j].MemRes
			}
			return (*taskList)[i].MemRes < (*taskList)[j].MemRes
		})
	case "mem_shr":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].MemShr > (*taskList)[j].MemShr
			}
			return (*taskList)[i].MemShr < (*taskList)[j].MemShr
		})
	case "mem_percent":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].MemPercent > (*taskList)[j].MemPercent
			}
			return (*taskList)[i].MemPercent < (*taskList)[j].MemPercent
		})
	case "gpm_target":
		sort.Slice(*taskList, func(i, j int) bool {
			if reverse {
				return (*taskList)[i].GPMTarget > (*taskList)[j].GPMTarget
			}
			return (*taskList)[i].GPMTarget < (*taskList)[j].GPMTarget
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

var readTaskListLock sync.Mutex

// ReadTaskList : Get list of task with selected infos
func ReadTaskList(reqGetTaskList *pb.ReqGetTaskList) (*pb.ResGetTaskList, uint64, string) {
	var newModelTaskList []model.Task
	var threads = 0
	var resGetTaskList pb.ResGetTaskList
	var taskList model.TaskList
	var sortingMethod = reqGetTaskList.GetSortBy()
	var needSorting = false
	var hideThreads = reqGetTaskList.GetHideThreads()

	var pidList []pid
	var pidListLen int
	var result []byte
	var err error

	var wait sync.WaitGroup
	var taskListAppendLock sync.Mutex
	var taskThreadListAppendLock sync.Mutex
	var totalThreadsLock sync.Mutex

	readTaskListLock.Lock()

	if sortingMethod != "" {
		err = checkSortingMethod(sortingMethod)
		if err != nil {
			goto ERROR
		}
		needSorting = true
	}

	pidList, err = getPIDList()
	if err != nil {
		goto ERROR
	}
	pidListLen = len(pidList)

	wait.Add(pidListLen)
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

			if !hideThreads {
				pid = task.PID
				threadsPIDs, err = getThreadsPIDs(pid)
				if err != nil {
					fmt.Printf("getThreadsPIDs(): PID: %d, Error: %s\n", pid, err.Error())
				}

				var wait2 sync.WaitGroup

				wait2.Add(len(threadsPIDs))
				for _, threadPID := range threadsPIDs {
					go func(routinePID int, routineThreadPID int) {
						var isNew bool
						var thread *model.Task

						if routinePID == routineThreadPID {
							goto OUT2
						}

						isNew = isThreadExist(&oldThreadsPIDs, routineThreadPID)
						thread = getThread(task, routineThreadPID, isNew)
						if thread == nil {
							goto OUT2
						}

						totalThreadsLock.Lock()
						threads++
						totalThreadsLock.Unlock()

						if needSorting {
							taskListAppendLock.Lock()
							newModelTaskList = append(newModelTaskList, *thread)
							taskListAppendLock.Unlock()
						} else {
							taskThreadListAppendLock.Lock()
							task.Threads = append(task.Threads, *thread)
							taskThreadListAppendLock.Unlock()
						}

					OUT2:
						wait2.Done()
						return
					}(pid, threadPID)
				}
				wait2.Wait()
			}

			if !needSorting {
				_ = sortTaskList(&task.Threads, "pid", false)
			}

			taskListAppendLock.Lock()
			newModelTaskList = append(newModelTaskList, *task)
			taskListAppendLock.Unlock()

		OUT:
			wait.Done()
			return
		}(p)
	}
	wait.Wait()

	modelTaskList = newModelTaskList

	if needSorting {
		taskList.TotalTasks = len(modelTaskList) - threads
	} else {
		taskList.TotalTasks = len(modelTaskList)
	}
	taskList.TotalThreads = threads

	if needSorting {
		err = sortTaskList(&modelTaskList, sortingMethod, reqGetTaskList.GetReverseSorting())
		if err != nil {
			goto ERROR
		}
		taskList.Tasks = modelTaskList
	} else {
		_ = sortTaskList(&modelTaskList, "pid", false)
		taskList.Tasks = makeTaskTree(&modelTaskList)
	}

	result, err = json.Marshal(taskList)
	resGetTaskList.Result = result

	readTaskListLock.Unlock()
	return &resGetTaskList, 0, ""
ERROR:
	readTaskListLock.Unlock()
	return nil, hcc_errors.HccErrorTestCode, err.Error()
}
