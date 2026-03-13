// Package applog writes diagnostic messages to %APPDATA%\mdview\mdview.log.
package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func logPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "mdview", "mdview.log")
}

// Write appends a timestamped line to the log file. Silent on error.
func Write(format string, args ...any) {
	path := logPath()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
}
