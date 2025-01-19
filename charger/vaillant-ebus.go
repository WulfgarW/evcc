package charger

import (
	"fmt"

	"github.com/WulfgarW/sensonetEbus"
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/charger/vaillantEbus"
	"github.com/evcc-io/evcc/util"
)

// VaillantEbus charger implementation
type VaillantEbus struct {
	conn *vaillantEbus.Switch
	*switchSocket
}

func init() {
	registry.Add("vaillant-ebus", NewVaillantEbusFromConfig)
}

// NewVaillantEbusFromConfig creates a VaillantEbus charger from generic config
func NewVaillantEbusFromConfig(other map[string]interface{}) (api.Charger, error) {
	var cc struct {
		embed         `mapstructure:",squash"`
		Ebusdaddress  string
		PvUseStrategy string
		HeatingZone   int
		Phases        int
		//		HeatingVetoDuration      int32
		HeatingTemperatureOffset float64
		StandbyPower             float64
	}

	if err := util.DecodeOther(other, &cc); err != nil {
		return nil, err
	}
	//WW Setting api.feature "Heating(4)" manually, because it does not work from the evcc.yaml file
	//WW Can be deleted, if it works from the config file
	//cc.embed.Features_ = append(cc.embed.Features_, 4)

	return NewVaillantEbus(cc.embed, cc.Ebusdaddress, cc.PvUseStrategy, cc.HeatingZone, cc.Phases, cc.HeatingTemperatureOffset, cc.StandbyPower)
}

// NewVaillantEbus creates VaillantEbus charger
func NewVaillantEbus(embed embed, ebusdAddress, pvUseStrategy string, heatingZone, phases int, heatingTemperatureOffset, standbypower float64) (*VaillantEbus, error) {
	conn, err := vaillantEbus.NewConnection(ebusdAddress, pvUseStrategy, heatingZone, phases, heatingTemperatureOffset)
	if err != nil {
		return nil, err
	}

	c := &VaillantEbus{
		conn: vaillantEbus.NewSwitch(conn),
	}

	c.switchSocket = NewSwitchSocket(&embed, c.Enabled, c.conn.CurrentPower, standbypower)

	return c, nil
}

// Enabled implements the api.Charger interface
func (c *VaillantEbus) Enabled() (bool, error) {
	return c.conn.Enabled()
}

// Enable implements the api.Charger interface
func (c *VaillantEbus) Enable(enable bool) error {
	err := c.conn.Enable(enable)
	if err != nil {
		return err
	}

	enabled, err := c.Enabled()
	switch {
	case err != nil:
		return err
	case enable != enabled:
		onoff := map[bool]string{true: "on", false: "off"}
		return fmt.Errorf("switch %s failed", onoff[enable])
	default:
		return nil
	}
}

func (c *VaillantEbus) Phases() int {
	return c.conn.Phases()
}

// Status implements the api.ChargeState interface
func (c *VaillantEbus) Status() (api.ChargeStatus, error) {
	status, err := c.conn.Status()
	if err != nil {
		return api.StatusA, err
	}
	return status, err
}

func (c *VaillantEbus) ModeText() string {
	switch c.conn.CurrentQuickmode() {
	case sensonetEbus.QUICKMODE_HOTWATER:
		return " (Hotwater Boost active)"
	case sensonetEbus.QUICKMODE_HEATING:
		if c.conn.QuickVetoExpiresAt() != "" {
			return " (Heating Quick Veto active. Ends " + c.conn.QuickVetoExpiresAt() + ")"
		}
		return " (Heating Quick Veto active)"
	case sensonetEbus.QUICKMODE_NOTHING:
		return " (charger running idle)"
	}
	return " (regular mode; hotwater temp. shown)"
}
