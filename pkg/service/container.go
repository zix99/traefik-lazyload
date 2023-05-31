package service

import (
	"time"
	"traefik-lazyload/pkg/config"
	"traefik-lazyload/pkg/containers"
)

type containerSettings struct {
	stopDelay     time.Duration
	waitForCode   int
	waitForPath   string
	waitForMethod string
	needs         []string
}

type ContainerState struct {
	name string
	containerSettings
	lastRecv, lastSend int64 // Last network traffic, used to see if idle
	lastActivity       time.Time
	started            time.Time
	pinned             bool // Don't remove, even if not started
}

func newStateFromContainer(ct *containers.Wrapper) *ContainerState {
	return &ContainerState{
		name:              ct.NameID(),
		containerSettings: extractContainerLabels(ct),
		lastActivity:      time.Now(),
		started:           time.Now(),
	}
}

func extractContainerLabels(ct *containers.Wrapper) (target containerSettings) {
	target.stopDelay, _ = ct.ConfigDuration("stopdelay", config.Model.StopDelay)
	target.waitForCode, _ = ct.ConfigInt("waitforcode", 200)
	target.waitForPath, _ = ct.ConfigOrDefault("waitforpath", "/")
	target.waitForMethod, _ = ct.ConfigOrDefault("waitformethod", "HEAD")
	target.needs, _ = ct.ConfigCSV("needs", nil)
	return
}

func (s *ContainerState) Name() string {
	return s.name
}

func (s *ContainerState) LastActive() time.Time {
	return s.lastActivity
}

func (s *ContainerState) LastActiveAge() string { // FIXME: Return duration (update UI)
	return time.Since(s.lastActivity).Round(time.Second).String()
}

func (s *ContainerState) Rx() int64 {
	return s.lastRecv
}

func (s *ContainerState) Tx() int64 {
	return s.lastSend
}

func (s *ContainerState) Started() time.Time {
	return s.started
}

func (s *containerSettings) StopDelay() string { // FIXME: Return duration (update UI)
	return s.stopDelay.String()
}

func (s *ContainerState) WaitForCode() int {
	return s.waitForCode
}

func (s *ContainerState) WaitForPath() string {
	return s.waitForPath
}

func (s *ContainerState) WaitForMethod() string {
	return s.waitForMethod
}
