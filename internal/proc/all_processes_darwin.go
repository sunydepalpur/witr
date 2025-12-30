//go:build darwin

package proc

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/pranshuparmar/witr/pkg/model"
)

func getAllProcessesOS() ([]model.ProcessSummary, error) {
	var processes []model.ProcessSummary

	// Use ps to get all processes
	// -e = all processes
	// -o = output format
	out, err := exec.Command("ps", "-e", "-o", "pid,ppid,user,comm").Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 || len(line) == 0 {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		user := fields[2]
		cmd := strings.Join(fields[3:], " ")

		processes = append(processes, model.ProcessSummary{
			PID:     pid,
			PPID:    ppid,
			User:    user,
			Command: cmd,
		})
	}

	return processes, nil
}
