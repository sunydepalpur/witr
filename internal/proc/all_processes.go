package proc

import (
	"github.com/pranshuparmar/witr/pkg/model"
)

func GetAllProcesses() ([]model.ProcessSummary, error) {

	// On Linux, we can scan /proc
	// On Darwin, we can use ps or other tools
	// For now, let's use a simple approach that works on both if possible,
	// or split it like other functions.

	return getAllProcessesOS()
}
