package main

import (
	"errors"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

var wg sync.WaitGroup

// stop specified proc.
func stopProc(proc string, quit bool) error {
	p, ok := procs[proc]
	if !ok {
		return errors.New("Unknown proc: " + proc)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil {
		return nil
	}

	p.quit = quit
	err := terminateProc(proc)
	if err != nil {
		return err
	}

	timeout := time.AfterFunc(10*time.Second, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p, ok := procs[proc]; ok && p.cmd != nil {
			err = p.cmd.Process.Kill()
		}
	})
	p.cond.Wait()
	timeout.Stop()
	return err
}

// start specified proc. if proc is started already, return nil.
func startProc(proc string) error {
	p, ok := procs[proc]
	if !ok {
		return errors.New("Unknown proc: " + proc)
	}

	p.mu.Lock()
	if procs[proc].cmd != nil {
		p.mu.Unlock()
		return nil
	}

	wg.Add(1)
	go func() {
		spawnProc(proc)
		wg.Done()
		p.mu.Unlock()
	}()
	return nil
}

// restart specified proc.
func restartProc(proc string) error {
	if _, ok := procs[proc]; !ok {
		return errors.New("Unknown proc: " + proc)
	}
	stopProc(proc, false)
	return startProc(proc)
}

// spawn all procs.
func startProcs() error {
	// sort of priority
	sortedProcs := []*procInfo{}
	for _, proc := range procs {
		sortedProcs = append(sortedProcs, proc)
	}
	sort.Slice(sortedProcs, func(i, j int) bool {
		return sortedProcs[i].ext.priority < sortedProcs[j].ext.priority
	})

	for _, proc := range sortedProcs {
		startProc(proc.proc)
	}
	sc := make(chan os.Signal, 10)
	go func() {
		wg.Wait()
		sc <- syscall.SIGINT
	}()
	signal.Notify(sc, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	<-sc
	for proc := range procs {
		stopProc(proc, true)
	}
	return nil
}
