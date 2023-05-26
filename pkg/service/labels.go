package service

import (
	"strconv"
	"strings"
	"time"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types"
	"github.com/sirupsen/logrus"
)

func labelOrDefault(ct *types.Container, sublabel, dflt string) (string, bool) {
	if val, ok := ct.Labels[config.SubLabel(sublabel)]; ok {
		return val, true
	}
	return dflt, false
}

func labelOrDefaultArr(ct *types.Container, sublabel string) ([]string, bool) {
	if val, ok := ct.Labels[config.SubLabel(sublabel)]; ok {
		return strings.Split(val, ","), true
	}
	return []string{}, false
}

func labelOrDefaultInt(ct *types.Container, sublabel string, dflt int) (int, bool) {
	s, ok := labelOrDefault(ct, sublabel, "")
	if !ok {
		return dflt, false
	}

	if val, err := strconv.Atoi(s); err != nil {
		logrus.Warnf("Unable to parse %s on %s: %v. Using default of %d", sublabel, containerShort(ct), err, dflt)
		return dflt, false
	} else {
		return val, true
	}
}

func labelOrDefaultDuration(ct *types.Container, sublabel string, dflt time.Duration) (time.Duration, bool) {
	s, ok := labelOrDefault(ct, sublabel, "")
	if !ok {
		return dflt, false
	}

	if val, err := time.ParseDuration(s); err != nil {
		logrus.Warnf("Unable to parse %s on %s: %v. Using default of %s", sublabel, containerShort(ct), err, dflt.String())
		return dflt, false
	} else {
		return val, true
	}
}
