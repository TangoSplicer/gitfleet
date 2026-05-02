package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type SwarmType int

const (
	SwarmStatus SwarmType = iota
	SwarmSync
	SwarmPrune
)

type Job struct {
	Path string
	Type SwarmType
}

type Result struct {
	Path    string
	Success bool
	Output  string
	Err     error
}

type Fleet struct {
	NumWorkers int
	Jobs       chan Job
	Results    chan Result
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewFleet(numWorkers int) *Fleet {
	ctx, cancel := context.WithCancel(context.Background())
	return &Fleet{
		NumWorkers: numWorkers,
		Jobs:       make(chan Job, 1000),
		Results:    make(chan Result, 1000),
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (f *Fleet) Start() {
	for i := 0; i < f.NumWorkers; i++ {
		f.wg.Add(1)
		go f.worker()
	}

	go func() {
		f.wg.Wait()
		close(f.Results)
	}()
}

func (f *Fleet) Stop() {
	f.cancel()
}

func (f *Fleet) worker() {
	defer f.wg.Done()
	for {
		select {
		case <-f.ctx.Done():
			return
		case job, ok := <-f.Jobs:
			if !ok {
				return
			}
			f.Results <- f.execute(job)
		}
	}
}

func (f *Fleet) execute(job Job) Result {
	var cmd *exec.Cmd
	var customOutput string

	switch job.Type {
	case SwarmStatus:
		cmd = exec.CommandContext(f.ctx, "git", "status", "--porcelain")
	case SwarmSync:
		// --autostash safely protects uncommitted work during the concurrent rebase
		cmd = exec.CommandContext(f.ctx, "git", "pull", "--rebase", "--autostash")
	case SwarmPrune:
		// 1. Fetch and prune dead tracking branches natively
		// 2. We use a bash wrap here to dynamically find and delete 'gone' local branches
		script := `git fetch -p && git branch -vv | grep ': gone]' | awk '{print $1}' | xargs -r git branch -D`
		cmd = exec.CommandContext(f.ctx, "sh", "-c", script)
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

	// Format specific outputs for the UI log
	if job.Type == SwarmStatus {
		if output != "" {
			customOutput = "Changes pending"
		} else {
			customOutput = "Clean"
		}
	} else if job.Type == SwarmPrune {
		if output != "" {
			customOutput = "Branches pruned"
		} else {
			customOutput = "Clean"
		}
	} else if err == nil {
		customOutput = "Synced"
	} else {
		customOutput = "Failed"
	}

	return Result{
		Path:    job.Path,
		Success: err == nil,
		Output:  customOutput,
		Err:     err,
	}
}
