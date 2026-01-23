// Test program to verify streaming command output works outside of Bubbletea TUI
package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"time"
)

func main() {
	fmt.Println("=== Stream Test: Testing command output streaming ===")
	fmt.Println()

	// Test 1: Simple echo command
	fmt.Println("--- Test 1: Quick echo command ---")
	testCommand("echo", "Hello from streaming test!")
	fmt.Println()

	// Test 2: Long-running command with periodic output
	fmt.Println("--- Test 2: Long-running command (bash loop) ---")
	testCommand("bash", "-c", "for i in 1 2 3 4 5; do echo \"Line $i at $(date +%H:%M:%S)\"; sleep 1; done && echo 'Done!'")
	fmt.Println()

	// Test 3: Try npx cap --help if available
	fmt.Println("--- Test 3: npx cap --help (if available) ---")
	testCommand("npx", "cap", "--help")
	fmt.Println()

	fmt.Println("=== All tests completed ===")
}

// testCommand runs a command with streaming output, using the same pattern as model.go
func testCommand(cmdName string, args ...string) {
	fmt.Printf("Running: %s %v\n", cmdName, args)
	start := time.Now()

	cmd := exec.Command(cmdName, args...)

	// Create pipes for stdout and stderr (same as model.go)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("ERROR creating stdout pipe: %v\n", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("ERROR creating stderr pipe: %v\n", err)
		return
	}

	// Create output channel for streaming (same buffer size as model.go)
	outputChan := make(chan string, 100)

	if err := cmd.Start(); err != nil {
		fmt.Printf("ERROR starting command: %v\n", err)
		close(outputChan)
		return
	}

	fmt.Printf("Command started (PID: %d)\n", cmd.Process.Pid)

	// Stream output in background goroutines (same as model.go)
	go streamPipe(stdout, outputChan)
	go streamPipe(stderr, outputChan)

	// Wait for command in background and close channel when done (same as model.go)
	go func() {
		waitErr := cmd.Wait()
		if waitErr != nil {
			fmt.Printf("[wait goroutine] Command exited with error: %v\n", waitErr)
		} else {
			fmt.Printf("[wait goroutine] Command exited successfully\n")
		}
		close(outputChan)
	}()

	// Now read from the channel (simulating what Bubbletea's Update does)
	lineCount := 0
	for {
		line, ok := <-outputChan
		if !ok {
			// Channel closed = command finished
			fmt.Printf("Channel closed after %d lines\n", lineCount)
			break
		}
		lineCount++
		fmt.Printf("  [%s] OUTPUT: %s\n", time.Since(start).Round(time.Millisecond), line)
	}

	fmt.Printf("Command completed in %v\n", time.Since(start).Round(time.Millisecond))
}

// streamPipe reads from a pipe and sends lines to the output channel
// This is identical to the function in model.go
func streamPipe(r io.Reader, outputChan chan string) {
	scanner := bufio.NewScanner(r)
	// Increase buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// Non-blocking send - drop if buffer full
		select {
		case outputChan <- line:
		default:
			// Buffer full, skip this line
			fmt.Printf("  [WARNING] Buffer full, dropped line: %s\n", line)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("  [streamPipe ERROR] %v\n", err)
	}
}
