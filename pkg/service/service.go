package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

type containerSettings struct {
	stopDelay   time.Duration
	waitForCode int
	waitForPath string
}

type containerState struct {
	Name string
	containerSettings
	lastRecv, lastSend int64 // Last network traffic, used to see if idle
	lastActivity       time.Time
}

type Core struct {
	mux  sync.Mutex
	term chan bool

	client *client.Client

	active map[string]*containerState // cid -> state
}

func New(client *client.Client, pollRate time.Duration) (*Core, error) {
	// Test client and report
	if info, err := client.Info(context.Background()); err != nil {
		return nil, err
	} else {
		logrus.Infof("Connected docker to %s (v%s)", info.Name, info.ServerVersion)
	}

	// Make core
	ret := &Core{
		client: client,
		active: make(map[string]*containerState),
		term:   make(chan bool),
	}

	ret.Poll() // initial force-poll to update
	go ret.pollThread(pollRate)

	return ret, nil
}

func (s *Core) Close() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.term <- true
	return s.client.Close()
}

type StartResult struct {
	WaitForCode int
	WaitForPath string
}

func (s *Core) StartHost(hostname string) (*StartResult, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	ctx := context.Background()

	ct, err := s.findContainerByHostname(ctx, hostname)
	if err != nil {
		logrus.Warnf("Unable to find container for host %s: %s", hostname, err)
		return nil, err
	}

	if ets, exists := s.active[ct.ID]; exists {
		// TODO: Handle case we think it's active, but not? (eg. crash? slow boot?)
		logrus.Debugf("Asked to start host, but we already think it's started: %s", ets.Name)
		return &StartResult{
			WaitForCode: ets.waitForCode,
			WaitForPath: ets.waitForPath,
		}, nil
	}

	go s.startContainer(ctx, ct)

	// add to active pool
	ets := newStateFromContainer(ct)
	s.active[ct.ID] = ets

	return &StartResult{
		WaitForCode: ets.waitForCode,
		WaitForPath: ets.waitForPath,
	}, nil
}

func (s *Core) startContainer(ctx context.Context, ct *types.Container) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if err := s.client.ContainerStart(ctx, ct.ID, types.ContainerStartOptions{}); err != nil {
		logrus.Warnf("Error starting container %s: %s", containerShort(ct), err)
	} else {
		logrus.Infof("Starting container %s", containerShort(ct))
	}
}

func (s *Core) pollThread(rate time.Duration) {
	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	for {
		select {
		case <-s.term:
			return
		case <-ticker.C:
			s.Poll()
		}
	}
}

// Initiate a thread-safe state-update, adding containers to the system, or
// stopping idle containers
// Will normally happen in the background with the pollThread
func (s *Core) Poll() {
	s.mux.Lock()
	defer s.mux.Unlock()

	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	s.checkForNewContainers(ctx)
	s.watchForInactivity(ctx)
}

func (s *Core) checkForNewContainers(ctx context.Context) {
	containers, err := s.findAllLazyloadContainers(ctx, false)
	if err != nil {
		logrus.Warnf("Error checking for new containers: %v", err)
		return
	}

	runningContainers := make(map[string]*types.Container)
	for i, ct := range containers {
		if isRunning(&ct) {
			runningContainers[ct.ID] = &containers[i]
		}
	}

	// check for containers we think are running, but aren't (destroyed, error'd, stop'd via another process, etc)
	for cid, cts := range s.active {
		if _, ok := runningContainers[cid]; !ok {
			logrus.Infof("Discover container had stopped, removing %s", cts.Name)
			delete(s.active, cid)
		}
	}

	// now, look for containers that are running, but aren't in our active inventory
	for _, ct := range runningContainers {
		if _, ok := s.active[ct.ID]; !ok {
			logrus.Infof("Discovered running container %s", containerShort(ct))
			s.active[ct.ID] = newStateFromContainer(ct)
		}
	}
}

func (s *Core) watchForInactivity(ctx context.Context) {
	for cid, cts := range s.active {
		shouldStop, err := s.checkContainerForInactivity(ctx, cid, cts)
		if err != nil {
			logrus.Warnf("error checking container state for %s: %s", cts.Name, err)
		}
		if shouldStop {
			if err := s.client.ContainerStop(ctx, cid, container.StopOptions{}); err != nil {
				logrus.Errorf("Error stopping container %s: %s", cts.Name, err)
			} else {
				logrus.Infof("Stopped container %s", cts.Name)
				delete(s.active, cid)
			}
		}
	}
}

func (s *Core) checkContainerForInactivity(ctx context.Context, cid string, ct *containerState) (shouldStop bool, retErr error) {
	statsStream, err := s.client.ContainerStatsOneShot(ctx, cid)
	if err != nil {
		return false, err
	}

	var stats types.StatsJSON
	if err := json.NewDecoder(statsStream.Body).Decode(&stats); err != nil {
		return false, err
	}

	if stats.PidsStats.Current == 0 {
		// Probably stopped. Will let next poll update container
		return true, errors.New("container not running")
	}

	// check for network activity
	rx, tx := sumNetworkBytes(stats.Networks)
	if rx > ct.lastRecv || tx > ct.lastSend {
		ct.lastRecv = rx
		ct.lastSend = tx
		ct.lastActivity = time.Now()
		return false, nil
	}

	// No activity, stop?
	if time.Now().After(ct.lastActivity.Add(ct.stopDelay)) {
		logrus.Infof("Found idle container %s...", ct.Name)
		return true, nil
	}

	return false, nil
}

func newStateFromContainer(ct *types.Container) *containerState {
	return &containerState{
		Name:              containerShort(ct),
		containerSettings: extractContainerLabels(ct),
		lastActivity:      time.Now(),
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

func (s *Core) findContainerByHostname(ctx context.Context, hostname string) (*types.Container, error) {
	containers, err := s.findAllLazyloadContainers(ctx, true)
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		if hostStr, ok := labelOrDefault(&c, "hosts", ""); ok {
			hosts := strings.Split(hostStr, ",")
			if strSliceContains(hosts, hostname) {
				return &c, nil
			}
		} else {
			// If not defined explicitely, infer from traefik route
			for k, v := range c.Labels {
				if strings.Contains(k, "traefik.http.routers.") && strings.Contains(v, hostname) { // TODO: More complex
					return &c, nil
				}
			}
		}
	}

	return nil, ErrNotFound
}

// Finds all containers on node that are labeled with lazyloader config
func (s *Core) findAllLazyloadContainers(ctx context.Context, includeStopped bool) ([]types.Container, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.Model.Labels.Prefix)

	return s.client.ContainerList(ctx, types.ContainerListOptions{
		All:     includeStopped,
		Filters: filters,
	})
}
