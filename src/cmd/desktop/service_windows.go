//go:build windows

package main

import (
	"errors"
	"sync"
)

// @sk-task kvn-desktop#T1.1: windows service stubs — server is embedded (AC-003)

var (
	serverRestart func() error
	serverStop    func() error
	mu            sync.Mutex
)

func SetServerRestart(fn func() error) {
	mu.Lock()
	defer mu.Unlock()
	serverRestart = fn
}

func SetServerStop(fn func() error) {
	mu.Lock()
	defer mu.Unlock()
	serverStop = fn
}

func (s *ServiceManager) Start() error {
	return errors.New("server is managed by the application process")
}

func (s *ServiceManager) Stop() error {
	mu.Lock()
	fn := serverStop
	mu.Unlock()
	if fn != nil {
		return fn()
	}
	return errors.New("server stop not available")
}

func (s *ServiceManager) Restart() error {
	mu.Lock()
	fn := serverRestart
	mu.Unlock()
	if fn != nil {
		return fn()
	}
	return errors.New("server restart not available")
}
