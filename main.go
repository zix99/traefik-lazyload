package main

import (
	"context"
	"embed"
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"path"
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

type SplashModel struct {
	Name        string
	WaitForCode int
	WaitForPath string
}

var splashTemplate = template.Must(template.ParseFS(httpAssets, "assets/splash.html"))

var dockerClient *client.Client

type containerState struct {
	Name, ID  string
	IsRunning bool
	LastWork  time.Time
	StopDelay time.Duration

	lastRecv, lastSend int64 // Last network traffic, used to see if idle
}

// containerID -> State
var managedContainers = make(map[string]*containerState)

func main() {

	// Connect to docker
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	dockerClient = cli

	// Test
	if info, err := cli.Info(context.Background()); err != nil {
		logrus.Fatal(err)
	} else {
		logrus.Infof("Connected docker to %s", info.Name)
	}

	if splash, err := httpAssets.ReadFile(path.Join("assets", Config.Splash)); err != nil || len(splash) == 0 {
		logrus.Fatal("Unable to open splash file %s", Config.Splash)
	}

	// Initial state
	if Config.StopAtBoot {
		stopAllLazyContainers()
	} else {
		//TODO: Inventory currently running containers
	}

	go watchForInactive()

	subFs, _ := fs.Sub(httpAssets, "assets")
	http.Handle(httpAssetPrefix, http.StripPrefix(httpAssetPrefix, http.FileServer(http.FS(subFs))))
	http.HandleFunc("/", ContainerHandler)

	logrus.Infof("Listening on %s...", Config.Listen)
	http.ListenAndServe(Config.Listen, nil)
}

func stopAllLazyContainers() error {
	filter := filters.NewArgs()
	filter.Add("label", "lazyloader")

	containers, err := dockerClient.ContainerList(context.Background(), types.ContainerListOptions{Filters: filter, All: true})
	if err != nil {
		return err
	}

	ctx, _ := context.WithTimeout(context.Background(), 1*time.Minute)

	for _, c := range containers {
		logrus.Infof("Stopping %s: %s", c.ID[:8], c.Names[0])
		dockerClient.ContainerStop(ctx, c.ID, container.StopOptions{})
	}

	return nil
}

func watchForInactive() {
	// TODO: Thread safety
	for {
		for cid, ct := range managedContainers {
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
				logrus.Infof("Stopping idle container %s...", ct.Name)
				err := dockerClient.ContainerStop(context.Background(), cid, container.StopOptions{})
				if err != nil {
					logrus.Warnf("Error stopping container: %s", err)
				} else {
					delete(managedContainers, cid)
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

	ct, _ := findContainerByHostname(r.Context(), host)
	if ct != nil || true {
		// TODO: Send response before querying anything about the container (the slow bit)
		w.WriteHeader(http.StatusAccepted)
		renderErr := splashTemplate.Execute(w, SplashModel{
			Name:        host,
			WaitForCode: 200, // TODO Config-based
			WaitForPath: "/",
		})
		if renderErr != nil {
			logrus.Error(renderErr)
		}

		// Look to start the container
		state := getOrCreateState(ct.ID)
		logrus.Infof("Found container %s for host %s, checking state...", containerShort(ct), host)

		if !state.IsRunning { // cache doesn't think it's running
			if ct.State != "running" {
				logrus.Infof("Container %s not running (is %s), starting...", state.Name, ct.State)
				go dockerClient.ContainerStart(context.Background(), ct.ID, types.ContainerStartOptions{}) // TODO: Check error
			}

			state.IsRunning = true
			state.Name = containerShort(ct)
			state.ID = ct.ID
			state.LastWork = time.Now()
			parseContainerSettings(state, ct)
		} // TODO: What if container crahsed but we think it's started?
	} else {
		logrus.Warnf("Unable to find container for host %s", host)
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Not Found")
	}
}

func getOrCreateState(cid string) (ret *containerState) {
	var ok bool
	if ret, ok = managedContainers[cid]; !ok {
		ret = &containerState{}
		managedContainers[cid] = ret
	}
	return
}

func parseContainerSettings(target *containerState, ct *types.Container) {
	{ // Parse stop delay
		var stopErr error
		stopDelay, _ := labelOrDefault(ct, "stopdelay", "10s")
		target.StopDelay, stopErr = time.ParseDuration(stopDelay)
		if stopErr != nil {
			target.StopDelay = 30 * time.Second
			logrus.Warnf("Unable to parse stopdelay of %s, defaulting to %s", stopDelay, target.StopDelay.String())
		}
	}
}

func findContainerByHostname(ctx context.Context, hostname string) (*types.Container, error) {
	containers, err := findAllLazyloadContainers(ctx, true)
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		for k, v := range c.Labels {
			if strings.Contains(k, "traefik.http.routers.") && strings.Contains(v, hostname) { // TODO: More complex, and self-ignore
				return &c, nil
			}
		}
	}

	return nil, errors.New("not found")
}

// Finds all containers on node that are labeled with lazyloader config
func findAllLazyloadContainers(ctx context.Context, includeStopped bool) ([]types.Container, error) {
	filters := filters.NewArgs()
	filters.Add("label", Config.Labels.Prefix)

	return dockerClient.ContainerList(ctx, types.ContainerListOptions{
		All:     includeStopped,
		Filters: filters,
	})
}
