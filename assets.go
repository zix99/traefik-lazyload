package main

import (
	"embed"
	"path"
	"text/template"
	"traefik-lazyload/pkg/config"
	"traefik-lazyload/pkg/service"
)

//go:embed assets/*
var httpAssets embed.FS

const httpAssetPrefix = "/__llassets/"

type SplashModel struct {
	*service.ContainerState
	Name string
}

var splashTemplate = template.Must(template.ParseFS(httpAssets, path.Join("assets", config.Model.Splash)))

type StatusPageModel struct {
	Active         []*service.ContainerState
	Qualifying     []service.ContainerWrapper
	RuntimeMetrics string
}

var statusPageTemplate = template.Must(template.ParseFS(httpAssets, "assets/status.html"))
