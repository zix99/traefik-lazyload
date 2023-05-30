package service

import (
	"sort"
	"strings"
	"time"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types"
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

func newStateFromContainer(ct *types.Container) *ContainerState {
	return &ContainerState{
		name:              containerShort(ct),
		containerSettings: extractContainerLabels(ct),
		lastActivity:      time.Now(),
		started:           time.Now(),
	}
}

func extractContainerLabels(ct *types.Container) (target containerSettings) {
	target.stopDelay, _ = labelOrDefaultDuration(ct, "stopdelay", config.Model.StopDelay)
	target.waitForCode, _ = labelOrDefaultInt(ct, "waitforcode", 200)
	target.waitForPath, _ = labelOrDefault(ct, "waitforpath", "/")
	target.waitForMethod, _ = labelOrDefault(ct, "waitformethod", "HEAD")
	target.needs, _ = labelOrDefaultArr(ct, "needs")
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

func (s *ContainerState) WaitForCode() int {
	return s.waitForCode
}

func (s *ContainerState) WaitForPath() string {
	return s.waitForPath
}

func (s *ContainerState) WaitForMethod() string {
	return s.waitForMethod
}

// Wrapper for container results that opaques and adds some methods to that data
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

func wrapContainers(cts ...types.Container) []ContainerWrapper {
	ret := make([]ContainerWrapper, len(cts))
	for i, c := range cts {
		ret[i] = ContainerWrapper{c}
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].NameID() < ret[j].NameID()
	})
	return ret
}
