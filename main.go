package main

import (
	"context"
	"embed"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

//go:embed assets/*
var httpAssets embed.FS

const httpAssetPrefix = "/__llassets/"

var dockerClient *client.Client

type containerState struct {
	IsRunning bool
	LastWork  time.Time
	StopDelay time.Duration

	lastRecv, lastSend int64 // Last network traffic, used to see if idle
}

// containerID -> State
var containerStateCache map[string]*containerState = make(map[string]*containerState)

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	dockerClient = cli

	if Config.StopAtBoot {
		stopAllLazyContainers()
	}

	go watchForInactive()

	subFs, _ := fs.Sub(httpAssets, "assets")
	http.Handle(httpAssetPrefix, http.StripPrefix(httpAssetPrefix, http.FileServer(http.FS(subFs))))
	http.HandleFunc("/", ContainerHandler)

	logrus.Infof("Listening on %s...", Config.Listen)
	http.ListenAndServe(Config.Listen, nil)
}

func stopAllLazyContainers() {
	filter := filters.NewArgs()
	filter.Add("label", "lazyloader")

	containers, _ := dockerClient.ContainerList(context.Background(), types.ContainerListOptions{Filters: filter, All: true})

	for _, c := range containers {
		logrus.Infof("Stopping %s: %s", c.ID[:8], c.Names[0])
		dockerClient.ContainerStop(context.Background(), c.ID, container.StopOptions{})
	}
}

func watchForInactive() {
	// TODO: Thread safety
	for {
		for cid, ct := range containerStateCache {
			if !ct.IsRunning {
				continue
			}

			statsStream, err := dockerClient.ContainerStatsOneShot(context.Background(), cid)
			if err != nil {
				logrus.Warn(err)
				continue
			}

			var stats types.StatsJSON
			if err := json.NewDecoder(statsStream.Body).Decode(&stats); err != nil {
				logrus.Warn(err)
				continue
			}

			if stats.PidsStats.Current == 0 {
				// Probably stopped
				*ct = containerState{} // Reset
				continue
			}

			// Check for network activity
			rx, tx := sumNetworkBytes(stats.Networks)
			if rx > ct.lastRecv || tx > ct.lastSend {
				ct.lastRecv = rx
				ct.lastSend = tx
				ct.LastWork = time.Now()
				continue
			}

			// No network activity for a while, stop?
			if time.Now().After(ct.LastWork.Add(ct.StopDelay)) {
				logrus.Infof("Stopping idle container %s...", short(cid))
				err := dockerClient.ContainerStop(context.Background(), cid, container.StopOptions{})
				if err != nil {
					logrus.Warnf("Error stopping container: %s", err)
				} else {
					delete(containerStateCache, cid)
				}
			}
		}

		time.Sleep(5 * time.Second) // TODO Increase/use-config
	}
}

func ContainerHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Not Found")
		return
	}

	ct, _ := findContainerWithRoute(r.Context(), host) // TODO: Use cache rather than query
	if ct != nil {
		// TODO: Send response before querying anything about the container (the slow bit)
		splash, _ := httpAssets.Open("assets/splash.html")
		io.Copy(w, splash)

		logrus.Infof("Found container %s for host %s, checking state...", containerShort(ct), host)
		state := getOrCreateCache(ct.ID)

		if !state.IsRunning {
			details, _ := dockerClient.ContainerInspect(r.Context(), ct.ID)

			if !details.State.Running {
				logrus.Infof("Container %s not running, starting...", containerShort(ct))
				dockerClient.ContainerStart(r.Context(), ct.ID, types.ContainerStartOptions{})
			}

			state.IsRunning = true
			state.LastWork = time.Now()

			var stopErr error
			stopDelay, _ := labelOrDefault(ct, "stopdelay", "10s")
			state.StopDelay, stopErr = time.ParseDuration(stopDelay)
			if stopErr != nil {
				state.StopDelay = 30 * time.Second
				logrus.Warnf("Unable to parse stopdelay of %s, defaulting to %s", stopDelay, state.StopDelay.String())
			}
		}
	} else {
		logrus.Warnf("Unable to find container for host %s", host)
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Not Found")
	}
}

func getOrCreateCache(cid string) (ret *containerState) {
	var ok bool
	if ret, ok = containerStateCache[cid]; !ok {
		ret = &containerState{}
		containerStateCache[cid] = ret
	}
	return
}

func findContainerWithRoute(ctx context.Context, route string) (*types.Container, error) {
	containers, err := findAllLazyloadContainers(ctx, true)
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		for k, v := range c.Labels {
			if strings.Contains(k, "traefik.http.routers.") && strings.Contains(v, route) { // TODO: More complex, and self-ignore
				return &c, nil
			}
		}
	}

	return nil, errors.New("not found")
}

func findAllLazyloadContainers(ctx context.Context, includeStopped bool) ([]types.Container, error) {
	filters := filters.NewArgs()
	filters.Add("label", Config.Labels.Prefix)

	return dockerClient.ContainerList(ctx, types.ContainerListOptions{
		All:     includeStopped,
		Filters: filters,
	})
}
