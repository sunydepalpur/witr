//go:build linux

package proc

import (
	"os"
	"strconv"

	"github.com/pranshuparmar/witr/pkg/model"
)

func getAllProcessesOS() ([]model.ProcessSummary, error) {
	var processes []model.ProcessSummary

	files, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(f.Name())
		if err != nil {
			continue
		}

		user := GetUser(pid)
		ppid := GetPPID(pid)
		cmdline := GetCmdline(pid)

		processes = append(processes, model.ProcessSummary{
			PID:     pid,
			PPID:    ppid,
			User:    user,
			Command: cmdline,
		})
	}

	return processes, nil
}
