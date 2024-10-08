package charger

// Code generated by github.com/evcc-io/evcc/cmd/tools/decorate.go. DO NOT EDIT.

import (
	"github.com/evcc-io/evcc/api"
)

func decorateNRGKickGen2(base *NRGKickGen2, phaseSwitcher func(int) error) api.Charger {
	switch {
	case phaseSwitcher == nil:
		return base

	case phaseSwitcher != nil:
		return &struct {
			*NRGKickGen2
			api.PhaseSwitcher
		}{
			NRGKickGen2: base,
			PhaseSwitcher: &decorateNRGKickGen2PhaseSwitcherImpl{
				phaseSwitcher: phaseSwitcher,
			},
		}
	}

	return nil
}

type decorateNRGKickGen2PhaseSwitcherImpl struct {
	phaseSwitcher func(int) error
}

func (impl *decorateNRGKickGen2PhaseSwitcherImpl) Phases1p3p(p0 int) error {
	return impl.phaseSwitcher(p0)
}
