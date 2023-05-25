package service

import (
	"fmt"
	"strings"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types"
)

func sumNetworkBytes(networks map[string]types.NetworkStats) (recv int64, send int64) {
	for _, ns := range networks {
		recv += int64(ns.RxBytes)
		send += int64(ns.TxBytes)
	}
	return
}

func labelOrDefault(ct *types.Container, sublabel, dflt string) (string, bool) {
	if val, ok := ct.Labels[config.SubLabel(sublabel)]; ok {
		return val, true
	}
	return dflt, false
}

func shortId(id string) string {
	const SLEN = 8
	if len(id) <= SLEN {
		return id
	}
	return id[:SLEN]
}

func containerShort(c *types.Container) string {
	var name string
	if len(c.Names) > 0 {
		name = trimRootPath(c.Names[0])
	} else {
		name = c.Image
	}
	return fmt.Sprintf("%s(%s)", name, shortId(c.ID))
}

func trimRootPath(s string) string {
	if strings.HasPrefix(s, "/") {
		return s[1:]
	}
	return s
}

func isRunning(c *types.Container) bool {
	return c.State == "running"
}

func strSliceContains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
