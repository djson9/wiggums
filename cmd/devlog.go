package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	devLogFile *os.File
	devLogOnce sync.Once
)

// devLog writes a timestamped message to ~/.wiggums/development.log.
// Safe for concurrent use from TUI and worker goroutines.
func devLog(format string, args ...interface{}) {
	devLogOnce.Do(func() {
		dir := filepath.Join(os.Getenv("HOME"), ".wiggums")
		os.MkdirAll(dir, 0755)
		f, err := os.OpenFile(filepath.Join(dir, "development.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		devLogFile = f
	})
	if devLogFile == nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(devLogFile, "[%s] %s\n", ts, msg)
}
