package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"traefik-lazyload/pkg/config"
	"traefik-lazyload/pkg/containers"
	"traefik-lazyload/pkg/service"

	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

type controller struct {
	core      *service.Core
	discovery *containers.Discovery
}

func mustCreateDockerClient() *client.Client {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logrus.Fatal("Unable to connect to docker: ", err)
	}

	return cli
}

func main() {
	if config.Model.Verbose {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debug("Verbose is on")
	}

	dockerClient := mustCreateDockerClient()
	discovery := containers.NewDiscovery(dockerClient)

	var err error
	core, err := service.New(dockerClient, discovery, config.Model.PollFreq)
	if err != nil {
		logrus.Fatal(err)
	}
	defer core.Close()

	if config.Model.StopAtBoot {
		core.StopAll()
	}

	controller := controller{
		core,
		discovery,
	}

	// Set up http server
	subFs, _ := fs.Sub(httpAssets, "assets")
	router := http.NewServeMux()
	router.Handle(httpAssetPrefix, http.StripPrefix(httpAssetPrefix, http.FileServer(http.FS(subFs))))
	router.HandleFunc("/", controller.ContainerHandler)

	srv := &http.Server{
		Addr:    config.Model.Listen,
		Handler: router,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	go func() {
		<-sigChan
		logrus.Info("Shutting down...")
		srv.Shutdown(context.Background())
	}()

	logrus.Infof("Listening on %s...", config.Model.Listen)
	if config.Model.StatusHost != "" {
		logrus.Infof("Status host set to %s", config.Model.StatusHost)
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logrus.Fatal(err)
	}
}

func (s *controller) ContainerHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Not Found")
		return
	}
	if host == config.Model.StatusHost && config.Model.StatusHost != "" {
		s.StatusHandler(w, r)
		return
	}

	if sOpts, err := s.core.StartHost(host); err != nil {
		if errors.Is(err, containers.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, "not found")
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, err.Error())
		}
	} else {
		w.WriteHeader(http.StatusAccepted)
		renderErr := splashTemplate.Execute(w, SplashModel{
			Hostname:       host,
			ContainerState: sOpts,
		})
		if renderErr != nil {
			logrus.Error(renderErr)
		}
	}
}

func (s *controller) StatusHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)

		qualifying, _ := s.discovery.QualifyingContainers(r.Context())
		providers, _ := s.discovery.ProviderContainers(r.Context())

		statusPageTemplate.Execute(w, StatusPageModel{
			Active:         s.core.ActiveContainers(),
			Qualifying:     qualifying,
			Providers:      providers,
			RuntimeMetrics: fmt.Sprintf("Heap=%d, InUse=%d, Total=%d, Sys=%d, NumGC=%d", stats.HeapAlloc, stats.HeapInuse, stats.TotalAlloc, stats.Sys, stats.NumGC),
		})
	default:
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Status page not found")
	}
}
