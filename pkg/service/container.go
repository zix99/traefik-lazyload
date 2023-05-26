package service

import (
	"strconv"
	"strings"
	"time"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types"
	"github.com/sirupsen/logrus"
)

type containerSettings struct {
	stopDelay   time.Duration
	waitForCode int
	waitForPath string
}

type ContainerState struct {
	name string
	containerSettings
	lastRecv, lastSend int64 // Last network traffic, used to see if idle
	lastActivity       time.Time
	started            time.Time
}

func newStateFromContainer(ct *types.Container) *ContainerState {
	return &ContainerState{
		name:              containerShort(ct),
		containerSettings: extractContainerLabels(ct),
		lastActivity:      time.Now(),
		started:           time.Now(),
	}
}

func extractContainerLabels(ct *types.Container) (target containerSettings) {
	{ // Parse stop delay
		stopDelay, _ := labelOrDefault(ct, "stopdelay", config.Model.StopDelay.String())
		if dur, stopErr := time.ParseDuration(stopDelay); stopErr != nil {
			target.stopDelay = config.Model.StopDelay
			logrus.Warnf("Unable to parse stopdelay for %s of %s, defaulting to %s", containerShort(ct), stopDelay, target.stopDelay.String())
		} else {
			target.stopDelay = dur
		}
	}
	{ // WaitForCode
		codeStr, _ := labelOrDefault(ct, "waitforcode", "200")
		if code, err := strconv.Atoi(codeStr); err != nil {
			target.waitForCode = 200
			logrus.Warnf("Unable to parse WaitForCode of %s, defaulting to %d", containerShort(ct), target.waitForCode)
		} else {
			target.waitForCode = code
		}
	}

	target.waitForPath, _ = labelOrDefault(ct, "waitforpath", "/")
	return
}

func (s *ContainerState) Name() string {
	return s.name
}

func (s *ContainerState) LastActive() time.Time {
	return s.lastActivity
}

func (s *ContainerState) LastActiveAge() string {
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

func (s *containerSettings) StopDelay() string {
	return s.stopDelay.String()
}

type ContainerWrapper struct {
	types.Container
}

func (s *ContainerWrapper) NameID() string {
	return containerShort(&s.Container)
}

func (s *ContainerWrapper) ConfigLabels() map[string]string {
	var matchString = config.Model.LabelPrefix + "."

	ret := make(map[string]string)
	for k, v := range s.Labels {
		if strings.HasPrefix(k, matchString) {
			ret[k[len(matchString):]] = v
		}
	}
	return ret
}
