package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"runtime"
	"traefik-lazyload/pkg/config"
	"traefik-lazyload/pkg/service"

	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

var core *service.Core

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

	var err error
	core, err = service.New(mustCreateDockerClient(), config.Model.PollFreq)
	if err != nil {
		logrus.Fatal(err)
	}
	defer core.Close()

	if config.Model.StopAtBoot {
		core.StopAll()
	}

	// Set up http server
	subFs, _ := fs.Sub(httpAssets, "assets")
	http.Handle(httpAssetPrefix, http.StripPrefix(httpAssetPrefix, http.FileServer(http.FS(subFs))))
	http.HandleFunc("/", ContainerHandler)

	logrus.Infof("Listening on %s...", config.Model.Listen)
	if config.Model.StatusHost != "" {
		logrus.Infof("Status host set to %s", config.Model.StatusHost)
	}
	http.ListenAndServe(config.Model.Listen, nil)
}

func ContainerHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Not Found")
		return
	}
	if host == config.Model.StatusHost && config.Model.StatusHost != "" {
		StatusHandler(w, r)
		return
	}

	if sOpts, err := core.StartHost(host); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, "not found")
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, err.Error())
		}
	} else {
		w.WriteHeader(http.StatusAccepted)
		renderErr := splashTemplate.Execute(w, SplashModel{
			Name:        host,
			WaitForCode: sOpts.WaitForCode,
			WaitForPath: sOpts.WaitForPath,
		})
		if renderErr != nil {
			logrus.Error(renderErr)
		}
	}
}

func StatusHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		statusPageTemplate.Execute(w, StatusPageModel{
			Active:         core.ActiveContainers(),
			Qualifying:     core.QualifyingContainers(),
			RuntimeMetrics: fmt.Sprintf("Heap=%d, InUse=%d, Total=%d, Sys=%d, NumGC=%d", stats.HeapAlloc, stats.HeapInuse, stats.TotalAlloc, stats.Sys, stats.NumGC),
		})
	default:
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Status page not found")
	}
}
