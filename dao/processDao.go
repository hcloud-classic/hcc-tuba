package dao

import (
	"bytes"
	"fmt"
	"github.com/hcloud-classic/hcc_errors"
	"github.com/hcloud-classic/pb"
	"github.com/mitchellh/go-ps"
	"hcc/tuba/lib/logger"
	"hcc/tuba/lib/syscheck"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func getCPUUsage(pid int) string {
	cmd := exec.Command("bash", "-c",
		"ps up "+strconv.Itoa(pid)+" | tail -n1 | grep -iv \"%CPU\" | tr -s ' ' | cut -f3 -d' '")
	out, err := cmd.Output()
	if err != nil {
		return "error"
	}

	trim := strings.TrimSpace(string(out))
	trimFloat, _ := strconv.ParseFloat(trim, 2)
	calcUsage := trimFloat / float64(runtime.NumCPU())
	value := fmt.Sprintf("%.1f", calcUsage)

	return value
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

func getStatFromProc(pid int) string {
	data, err := readProcFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "error"
	}
	if data == nil {
		return "error"
	}

	// Parse out data after (<cmd name>)
	i := bytes.LastIndex(data, []byte(")"))
	if i == -1 {
		return "error"
	}
	data = data[i+2:]

	stats := bytes.Fields(data)
	if len(stats) < 3 {
		logger.Logger.Println("wrong stat data (PID: " + strconv.Itoa(pid) + ")")
		return "error"
	}

	switch stats[0][0] {
	case 'R':
		return "running"
	case 'S':
		return "sleeping"
	case 'D':
		return "blocked"
	case 'Z':
		return "zombies"
	case 'X':
		return "dead"
	case 'T', 't':
		return "stopped"
	case 'W':
		return "paging"
	case 'I':
		return "idle"
	case 'P':
		return "parked"
	default:
		return "unknown"
	}
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

// ReadProcessList : Get list of processes with selected infos
func ReadProcessList(in *pb.ReqGetProcessList) (*pb.ResGetProcessList, uint64, string) {
	var processList pb.ResGetProcessList
	var processes []pb.Process
	var pprocesses []*pb.Process

	var pid int64
	var pidOk = false
	var ppid int64
	var ppidOk = false
	var exe string
	var exeOk = false
	var cpuUsage string
	var cpuUsageOk = false
	var stat string
	var statOk = false
	var epmType string
	var epmTypeOk = false
	var epmSource int64
	var epmSourceOk = false
	var epmTarget int64
	var epmTargetOk = false

	if in.Process != nil {
		reqProcess := in.Process

		pid = reqProcess.PID
		pidOk = pid != 0
		ppid = reqProcess.PPID
		ppidOk = ppid != 0
		exe = reqProcess.EXE
		exeOk = len(exe) != 0
		cpuUsage = reqProcess.CPUUsage
		cpuUsageOk = len(cpuUsage) != 0
		stat = reqProcess.Stat
		statOk = len(stat) != 0

		if syscheck.EPMProcSupported {
			epmType = reqProcess.EPMType
			epmTypeOk = len(epmType) != 0
			epmSource = reqProcess.EPMSource
			epmSourceOk = epmSource != 0
			epmTarget = reqProcess.EPMTarget
			epmTargetOk = epmTarget != 0
		}
	}

	pList, err := ps.Processes()
	if err != nil {
		return nil, hcc_errors.HccErrorTestCode, err.Error()
	}

	for _, process := range pList {
		processPID := process.Pid()
		p := pb.Process{
			PID:       int64(processPID),
			PPID:      int64(process.PPid()),
			EXE:       process.Executable(),
			CPUUsage:  getCPUUsage(process.Pid()),
			Stat:      getStatFromProc(process.Pid()),
			EPMType:   "NOT_SUPPORTED",
			EPMSource: 0,
			EPMTarget: 0,
		}

		if syscheck.EPMProcSupported {
			p.EPMType = getProcData(processPID, "epm_type")
			src, _ := strconv.Atoi(getProcData(processPID, "epm_source"))
			p.EPMSource = int64(src)
			target, _ := strconv.Atoi(getProcData(processPID, "epm_target"))
			p.EPMTarget = int64(target)
		}

		if pidOk && pid != p.PID {
			continue
		}
		if ppidOk && ppid != p.PPID {
			continue
		}
		if exeOk && exe != p.EXE {
			continue
		}
		if cpuUsageOk && cpuUsage != p.CPUUsage {
			continue
		}
		if statOk && stat != p.Stat {
			continue
		}
		if syscheck.EPMProcSupported {
			if epmTypeOk && epmType != p.EPMType {
				continue
			}
			if epmSourceOk && epmSource != p.EPMSource {
				continue
			}
			if epmTargetOk && epmTarget != p.EPMTarget {
				continue
			}
		}

		processes = append(processes, pb.Process{
			PID:       p.PID,
			PPID:      p.PPID,
			EXE:       p.EXE,
			CPUUsage:  p.CPUUsage,
			Stat:      p.Stat,
			EPMType:   p.EPMType,
			EPMSource: p.EPMSource,
			EPMTarget: p.EPMTarget,
		})
	}

	for i := range processes {
		pprocesses = append(pprocesses, &processes[i])
	}

	processList.Process = pprocesses

	return &processList, 0, ""
}
