package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

type logResources struct {
	traceLogger *log.Logger
	cleanup     func()
}

func setupLogResources(enableTrace bool, traceDir string) (logResources, error) {
	if !enableTrace {
		return logResources{}, nil
	}

	if traceDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return logResources{}, fmt.Errorf("resolve cwd: %w", err)
		}
		traceDir = cwd
	}

	if err := os.MkdirAll(traceDir, 0755); err != nil {
		return logResources{}, fmt.Errorf("create trace dir: %w", err)
	}

	logPath := filepath.Join(traceDir, "proxy-privacy.log")
	tracePath := filepath.Join(traceDir, "proxy-privacy.trace.log")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return logResources{}, fmt.Errorf("open log file: %w", err)
	}

	traceFile, err := os.OpenFile(tracePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		logFile.Close()
		return logResources{}, fmt.Errorf("open trace file: %w", err)
	}

	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	traceLogger := log.New(io.MultiWriter(os.Stderr, traceFile), "", log.Ldate|log.Ltime|log.Lmicroseconds)

	cleanup := func() {
		_ = logFile.Close()
		_ = traceFile.Close()
	}

	log.Printf("logging enabled log_file=%s trace_file=%s", logPath, tracePath)

	return logResources{
		traceLogger: traceLogger,
		cleanup:     cleanup,
	}, nil
}
