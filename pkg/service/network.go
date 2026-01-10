package service

import (
	"github.com/docker/docker/api/types"
)

type netIntfState struct {
	Rx, Tx uint64
}

type netState struct {
	intf map[string]*netIntfState
}

func newNetState() *netState {
	return &netState{
		intf: make(map[string]*netIntfState),
	}
}

func (s *netState) Rx() uint64 {
	var total uint64
	for _, intf := range s.intf {
		total += intf.Rx
	}
	return total
}

func (s *netState) Tx() uint64 {
	var total uint64
	for _, intf := range s.intf {
		total += intf.Tx
	}
	return total
}

// Returns number of interfaces changed
func (s *netState) UpdateStats(stats map[string]types.NetworkStats) int {
	changed := 0

	for name, state := range stats {
		if curr, ok := s.intf[name]; ok {
			if state.RxBytes > curr.Rx || state.TxBytes > curr.Tx {
				curr.Rx = state.RxBytes
				curr.Tx = state.TxBytes
				changed++
			}
		} else {
			s.intf[name] = &netIntfState{
				Rx: state.RxBytes,
				Tx: state.TxBytes,
			}
			changed++ // new interface is a change
		}
	}

	return changed
}
