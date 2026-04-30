package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// SwarmType defines the operation to be executed across the fleet.
type SwarmType int

const (
	SwarmStatus SwarmType = iota
	SwarmSync
	SwarmPrune
)

// Job represents a single git repository operation.
type Job struct {
	Path string
	Type SwarmType
}

// Result represents the outcome of a Job.
type Result struct {
	Path    string
	Success bool
	Output  string
	Err     error
}

// Fleet manages the bounded worker pool and channels.
type Fleet struct {
	NumWorkers int
	Jobs       chan Job
	Results    chan Result
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewFleet initializes a new concurrent fleet manager.
func NewFleet(numWorkers int) *Fleet {
	ctx, cancel := context.WithCancel(context.Background())
	return &Fleet{
		NumWorkers: numWorkers,
		Jobs:       make(chan Job, 1000), // Buffered to prevent blocking the scanner
		Results:    make(chan Result, 1000),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start boots up the worker pool.
func (f *Fleet) Start() {
	for i := 0; i < f.NumWorkers; i++ {
		f.wg.Add(1)
		go f.worker()
	}

	// Watcher to close the results channel once all jobs are processed
	go func() {
		f.wg.Wait()
		close(f.Results)
	}()
}

// Stop gracefully cancels all running workers.
func (f *Fleet) Stop() {
	f.cancel()
}

// worker listens for jobs and executes them until the channel is closed or context cancelled.
func (f *Fleet) worker() {
	defer f.wg.Done()
	for {
		select {
		case <-f.ctx.Done():
			return
		case job, ok := <-f.Jobs:
			if !ok {
				return // Jobs channel closed, exit worker
			}
			f.Results <- f.execute(job)
		}
	}
}

// execute runs the actual Git commands based on the SwarmType.
func (f *Fleet) execute(job Job) Result {
	var cmd *exec.Cmd
	
	switch job.Type {
	case SwarmStatus:
		// Check for uncommitted changes
		cmd = exec.CommandContext(f.ctx, "git", "status", "--porcelain")
	case SwarmSync:
		// Fetch and pull changes
		cmd = exec.CommandContext(f.ctx, "git", "pull", "--rebase")
	case SwarmPrune:
		// Fetch and prune dead tracking branches
		cmd = exec.CommandContext(f.ctx, "git", "fetch", "-p")
	default:
		return Result{Path: job.Path, Success: false, Err: fmt.Errorf("unknown swarm type")}
	}

	cmd.Dir = job.Path
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	output := strings.TrimSpace(outBuf.String())
	if output == "" {
		output = strings.TrimSpace(errBuf.String())
	}

	// Specialized output formatting for Status
	if job.Type == SwarmStatus && output != "" {
		output = "Changes pending"
	} else if job.Type == SwarmStatus && output == "" {
		output = "Clean"
	}

	return Result{
		Path:    job.Path,
		Success: err == nil,
		Output:  output,
		Err:     err,
	}
}
