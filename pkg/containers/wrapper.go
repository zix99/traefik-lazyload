package containers

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types"
	"github.com/sirupsen/logrus"
)

// Wrapper for container results that opaques and adds some methods to that data
type Wrapper struct {
	types.Container
}

// Human-consumable name + ID
func (s *Wrapper) NameID() string {
	var name string
	if len(s.Names) > 0 {
		name = strings.TrimPrefix(s.Names[0], "/")
	} else {
		name = s.Image
	}
	return fmt.Sprintf("%s (%s)", name, s.ShortId())
}

// char-len capped ID
func (s *Wrapper) ShortId() string {
	const SLEN = 8
	if len(s.ID) <= SLEN {
		return s.ID
	}
	return s.ID[:SLEN]
}

// Returns config labels with the prefix trimmed
func (s *Wrapper) ConfigLabels() map[string]string {
	var matchString = config.Model.LabelPrefix + "."

	ret := make(map[string]string)
	for k, v := range s.Labels {
		if strings.HasPrefix(k, matchString) {
			ret[k[len(matchString):]] = v
		}
	}

	return ret
}

func (s *Wrapper) Config(sublabel string) (string, bool) {
	ret, ok := s.Labels[config.SubLabel(sublabel)]
	return ret, ok
}

func (s *Wrapper) ConfigOrDefault(sublabel, dflt string) (string, bool) {
	if val, ok := s.Config(sublabel); ok {
		return val, true
	}
	return dflt, false
}

func (s *Wrapper) ConfigCSV(sublabel string, dflt []string) ([]string, bool) {
	if val, ok := s.Config(sublabel); ok {
		return strings.Split(val, ","), true
	}
	return dflt, false
}

func (s *Wrapper) ConfigInt(sublabel string, dflt int) (int, bool) {
	val, ok := s.Config(sublabel)
	if !ok {
		return dflt, false
	}

	if ival, err := strconv.Atoi(val); err != nil {
		logrus.Warnf("Unable to parse %s on %s: %v. Using default of %d", sublabel, s.NameID(), err, dflt)
		return dflt, false
	} else {
		return ival, true
	}
}

func (s *Wrapper) ConfigDuration(sublabel string, dflt time.Duration) (time.Duration, bool) {
	val, ok := s.Config(sublabel)
	if !ok {
		return dflt, false
	}

	if dur, err := time.ParseDuration(val); err != nil {
		logrus.Warnf("Unable to parse %s on %s: %v. Using default of %s", sublabel, s.NameID(), err, dflt.String())
		return dflt, false
	} else {
		return dur, true
	}
}

// true if state is running
func (s *Wrapper) IsRunning() bool {
	return s.State == "running"
}

// Wrap a container set
func wrapContainers(cts ...types.Container) []Wrapper {
	ret := make([]Wrapper, len(cts))
	for i, c := range cts {
		ret[i] = Wrapper{c}
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].NameID() < ret[j].NameID()
	})
	return ret
}

func wrapListResult(cts []types.Container, err error) ([]Wrapper, error) {
	return wrapContainers(cts...), err
}
