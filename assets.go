package main

import (
	"embed"
	"path"
	"text/template"
	"traefik-lazyload/pkg/config"
	"traefik-lazyload/pkg/containers"
	"traefik-lazyload/pkg/service"
)

//go:embed assets/*
var httpAssets embed.FS

const httpAssetPrefix = "/__llassets/"

type SplashModel struct {
	*service.ContainerState
	Hostname string
}

type StatusPageModel struct {
	Active         []*service.ContainerState
	Qualifying     []containers.Wrapper
	Providers      []containers.Wrapper
	RuntimeMetrics string
}

type assetTemplates struct {
	splash *template.Template
	status *template.Template
}

func LoadTemplates() *assetTemplates {
	return &assetTemplates{
		splash: template.Must(template.ParseFS(httpAssets, path.Join("assets", config.Model.Splash))),
		status: template.Must(template.ParseFS(httpAssets, "assets/status.html")),
	}
}
