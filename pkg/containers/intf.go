package containers

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

type Host interface {
	ContainerList(ctx context.Context, clo types.ContainerListOptions) ([]types.Container, error)

	ContainerStart(ctx context.Context, id string, opt types.ContainerStartOptions) error
	ContainerStop(ctx context.Context, id string, opt container.StopOptions) error

	ContainerStatsOneShot(ctx context.Context, id string) (types.ContainerStats, error)

	Close() error
}
