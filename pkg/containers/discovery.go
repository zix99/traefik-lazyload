package containers

import (
	"context"
	"strings"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

type Discovery struct {
	client *client.Client
}

func NewDiscovery(client *client.Client) *Discovery {
	return &Discovery{client}
}

// Return all containers that qualify to be load-managed (eg. have the tag)
func (s *Discovery) QualifyingContainers(ctx context.Context) ([]Wrapper, error) {
	return s.FindAllLazyload(ctx, true)
}

func (s *Discovery) ProviderContainers(ctx context.Context) ([]Wrapper, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.SubLabel("provides"))

	return wrapListResult(s.client.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters,
		All:     true,
	}))
}

func (s *Discovery) FindAllLazyload(ctx context.Context, includeStopped bool) ([]Wrapper, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.Model.LabelPrefix)

	return wrapListResult(s.client.ContainerList(ctx, types.ContainerListOptions{
		All:     includeStopped,
		Filters: filters,
	}))
}

func (s *Discovery) FindContainerByHostname(ctx context.Context, hostname string) (*Wrapper, error) {
	containers, err := s.FindAllLazyload(ctx, true)
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		if hostStr, ok := c.Config("hosts"); ok {
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

func (s *Discovery) FindDepProvider(ctx context.Context, name string) ([]Wrapper, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.SubLabel("provides")+"="+name)
	return wrapListResult(s.client.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters,
		All:     true,
	}))
}
