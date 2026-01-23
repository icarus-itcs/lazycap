package ui

import (
	"os/exec"
	"time"
)

// ProcessStatus represents the state of a process
type ProcessStatus int

const (
	ProcessRunning ProcessStatus = iota
	ProcessSuccess
	ProcessFailed
	ProcessCancelled
)

// Process represents a running or completed command
type Process struct {
	ID         string
	Name       string
	Command    string
	Status     ProcessStatus
	StartTime  time.Time
	EndTime    time.Time
	Logs       []string
	Cmd        *exec.Cmd
	OutputChan chan string
	Error      error
}

// Duration returns how long the process has been running or ran
func (p *Process) Duration() time.Duration {
	if p.Status == ProcessRunning {
		return time.Since(p.StartTime)
	}
	return p.EndTime.Sub(p.StartTime)
}

// StatusIcon returns an icon representing the process status
func (p *Process) StatusIcon() string {
	switch p.Status {
	case ProcessRunning:
		return "◐" // Will be replaced with spinner
	case ProcessSuccess:
		return "✓"
	case ProcessFailed:
		return "✗"
	case ProcessCancelled:
		return "○"
	default:
		return "?"
	}
}

// AddLog adds a log line to the process
func (p *Process) AddLog(line string) {
	p.Logs = append(p.Logs, line)
	// Keep max 5000 lines per process
	if len(p.Logs) > 5000 {
		p.Logs = p.Logs[len(p.Logs)-5000:]
	}
}
