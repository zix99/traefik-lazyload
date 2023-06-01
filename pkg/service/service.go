package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
	"traefik-lazyload/pkg/config"
	"traefik-lazyload/pkg/containers"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

type Core struct {
	mux  sync.Mutex
	term chan bool

	client    *client.Client
	discovery *containers.Discovery

	active map[string]*ContainerState // cid -> state
}

func New(client *client.Client, discovery *containers.Discovery, pollRate time.Duration) (*Core, error) {
	// Test client and report
	if info, err := client.Info(context.Background()); err != nil {
		return nil, err
	} else {
		logrus.Infof("Connected docker to %s (v%s)", info.Name, info.ServerVersion)
	}

	// Make core
	ret := &Core{
		client:    client,
		discovery: discovery,
		active:    make(map[string]*ContainerState),
		term:      make(chan bool),
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

func (s *Core) StartHost(hostname string) (*ContainerState, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), config.Model.Timeout)

	ct, err := s.discovery.FindContainerByHostname(ctx, hostname)
	if err != nil {
		logrus.Warnf("Unable to find container for host %s: %s", hostname, err)
		cancel()
		return nil, err
	}

	if ets, exists := s.active[ct.ID]; exists {
		logrus.Debugf("Asked to start host, but we already think it's started: %s", ets.name)
		cancel()
		return ets, nil
	}

	// add to active pool
	logrus.Infof("Starting container for %s...", hostname)
	ets := newStateFromContainer(ct)
	s.active[ct.ID] = ets
	ets.pinned = true // pin while starting

	go func() {
		defer func() {
			cancel()
			s.mux.Lock()
			ets.pinned = false
			ets.lastActivity = time.Now()
			s.mux.Unlock()
		}()
		s.startDependencyFor(ctx, ets.needs, ct.NameID())
		s.startContainerSync(ctx, ct)
	}()

	return ets, nil
}

// Stop all running containers pined with the configured label
func (s *Core) StopAll() {
	s.mux.Lock()
	defer s.mux.Unlock()

	ctx := context.Background()

	logrus.Info("Stopping all containers...")
	for cid, ct := range s.active {
		logrus.Infof("Stopping %s...", ct.name)
		if err := s.client.ContainerStop(ctx, cid, container.StopOptions{}); err != nil {
			logrus.Warnf("Error stopping %s: %v", ct.name, err)
		} else {
			delete(s.active, cid)
		}
	}
}

// Returns all actively managed containers
func (s *Core) ActiveContainers() []*ContainerState {
	s.mux.Lock()
	defer s.mux.Unlock()

	ret := make([]*ContainerState, 0, len(s.active))
	for _, item := range s.active {
		ret = append(ret, item)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].name < ret[j].name
	})
	return ret
}

func (s *Core) startContainerSync(ctx context.Context, ct *containers.Wrapper) error {
	if ct.IsRunning() {
		return nil
	}

	if err := s.client.ContainerStart(ctx, ct.ID, types.ContainerStartOptions{}); err != nil {
		logrus.Warnf("Error starting container %s: %s", ct.NameID(), err)
		return err
	} else {
		logrus.Infof("Started container %s", ct.NameID())
	}
	return nil
}

func (s *Core) startDependencyFor(ctx context.Context, needs []string, forContainer string) error {
	for _, dep := range needs {
		providers, err := s.discovery.FindDepProvider(ctx, dep)

		if err != nil {
			logrus.Errorf("Error finding dependency provider for %s: %v", dep, err)
			return err
		} else if len(providers) == 0 {
			logrus.Warnf("Unable to find any container that provides %s for %s", dep, forContainer)
			return ErrProviderNotFound
		} else {
			for _, provider := range providers {
				if !provider.IsRunning() {
					logrus.Infof("Starting dependency for %s: %s", forContainer, provider.NameID())

					if err := s.startContainerSync(ctx, &provider); err != nil {
						return err
					}

					delay, _ := provider.ConfigDuration("provides.delay", 2*time.Second)
					logrus.Debugf("Delaying %s to start %s", delay.String(), dep)
					time.Sleep(delay)
				}
			}
		}
	}

	return nil
}

func (s *Core) stopDependenciesFor(ctx context.Context, cid string, cts *ContainerState) []error {
	// Look at our needs, and see if anything else needs them; if not, shut down
	var errs []error

	deps := make(map[string]bool) // dep -> needed
	for _, dep := range cts.needs {
		deps[dep] = false
	}

	for activeId, active := range s.active {
		if activeId != cid { // ignore self
			for _, need := range active.needs {
				deps[need] = true
			}
		}
	}

	for dep, needed := range deps {
		if !needed {
			logrus.Infof("Stopping dependency %s...", dep)
			containers, err := s.discovery.FindDepProvider(ctx, dep)
			if err != nil {
				logrus.Errorf("Unable to find dependency provider containers for %s: %v", dep, err)
				errs = append(errs, err)
			} else if len(containers) == 0 {
				logrus.Warnf("Unable to find any containers for dependency %s", dep)
			} else {
				for _, ct := range containers {
					if ct.IsRunning() {
						logrus.Infof("Stopping %s...", ct.NameID())
						go s.client.ContainerStop(ctx, ct.ID, container.StopOptions{})
					}
				}
			}
		}
	}

	return errs
}

// Ticker loop that will check internal state against docker state (Call Poll)
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
	ctx, cancel := context.WithTimeout(context.Background(), config.Model.Timeout)
	defer cancel()

	s.checkForNewContainersSync(ctx)
	s.watchForInactivitySync(ctx)
}

func (s *Core) checkForNewContainersSync(ctx context.Context) {
	cts, err := s.discovery.FindAllLazyload(ctx, false)
	if err != nil {
		logrus.Warnf("Error checking for new containers: %v", err)
		return
	}

	runningContainers := make(map[string]*containers.Wrapper)
	for i, ct := range cts {
		if ct.IsRunning() {
			runningContainers[ct.ID] = &cts[i]
		}
	}

	// Lock
	s.mux.Lock()
	defer s.mux.Unlock()

	// check for containers we think are running, but aren't (destroyed, error'd, stop'd via another process, etc)
	for cid, cts := range s.active {
		if _, ok := runningContainers[cid]; !ok && !cts.pinned {
			logrus.Infof("Discover container had stopped, removing %s", cts.name)
			delete(s.active, cid)
			s.stopDependenciesFor(ctx, cid, cts)
		}
	}

	// now, look for containers that are running, but aren't in our active inventory
	for _, ct := range runningContainers {
		if _, ok := s.active[ct.ID]; !ok {
			logrus.Infof("Discovered running container %s", ct.NameID())
			s.active[ct.ID] = newStateFromContainer(ct)
		}
	}
}

func (s *Core) watchForInactivitySync(ctx context.Context) {
	s.mux.Lock()
	defer s.mux.Unlock()

	for cid, cts := range s.active {
		shouldStop, err := s.checkContainerForInactivity(ctx, cid, cts)
		if err != nil {
			logrus.Warnf("error checking container state for %s: %s", cts.name, err)
		}
		if shouldStop {
			s.stopContainerAndDependencies(ctx, cid, cts)
		}
	}
}

func (s *Core) stopContainerAndDependencies(ctx context.Context, cid string, cts *ContainerState) {
	// First, stop the host container
	if err := s.client.ContainerStop(ctx, cid, container.StopOptions{}); err != nil {
		logrus.Errorf("Error stopping container %s: %s", cts.name, err)
	} else {
		logrus.Infof("Stopped container %s", cts.name)
		delete(s.active, cid)
		s.stopDependenciesFor(ctx, cid, cts)
	}
}

func (s *Core) checkContainerForInactivity(ctx context.Context, cid string, ct *ContainerState) (shouldStop bool, retErr error) {
	if ct.pinned {
		return false, nil
	}

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
		logrus.Infof("Found idle container %s...", ct.name)
		return true, nil
	}

	return false, nil
}
