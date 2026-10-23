package main

import (
	"log"
	"os"
)

// logf writes to the file named by MTX_LOG (append). Without it, logging is off.
var logf = func(format string, args ...any) {}

// openLog routes logf to $MTX_LOG. A path that cannot be opened is an error, not a silent drop.
func openLog() (func(), error) {
	path := os.Getenv("MTX_LOG")
	if path == "" {
		return func() {}, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	l := log.New(f, "", log.Ltime|log.Lmicroseconds)
	logf = func(format string, args ...any) { l.Printf(format, args...) }
	return func() { _ = f.Close() }, nil
}
