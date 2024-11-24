package charger

import (
	"fmt"

	sensonetlib "github.com/WulfgarW/sensonet"
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/charger/sensonet"
	"github.com/evcc-io/evcc/util"
)

// Sensonet charger implementation
type Sensonet struct {
	conn *sensonet.Switch
	*switchSocket
}

func init() {
	registry.Add("sensonet", NewSensonetFromConfig)
}

// NewSensonetFromConfig creates a Sensonet charger from generic config
func NewSensonetFromConfig(other map[string]interface{}) (api.Charger, error) {
	var cc struct {
		embed         `mapstructure:",squash"`
		User          string
		Password      string
		Realm         string
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

	return NewSensonet(cc.embed, cc.User, cc.Password, cc.Realm, cc.PvUseStrategy, cc.HeatingZone, cc.Phases, cc.HeatingTemperatureOffset, cc.StandbyPower)
}

// NewSensonet creates Sensonet charger
func NewSensonet(embed embed, user, password, realm, pvUseStrategy string, heatingZone, phases int, heatingTemperatureOffset, standbypower float64) (*Sensonet, error) {
	conn, err := sensonet.NewConnection(user, password, realm, pvUseStrategy, heatingZone, phases, heatingTemperatureOffset)
	if err != nil {
		return nil, err
	}

	c := &Sensonet{
		conn: sensonet.NewSwitch(conn),
	}

	c.switchSocket = NewSwitchSocket(&embed, c.Enabled, c.conn.CurrentPower, standbypower)

	return c, nil
}

// Enabled implements the api.Charger interface
func (c *Sensonet) Enabled() (bool, error) {
	return c.conn.Enabled()
}

// Enable implements the api.Charger interface
func (c *Sensonet) Enable(enable bool) error {
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

func (c *Sensonet) Phases() int {
	return c.conn.Phases()
}

// Status implements the api.ChargeState interface
func (c *Sensonet) Status() (api.ChargeStatus, error) {
	status, err := c.conn.Status()
	if err != nil {
		return api.StatusA, err
	}
	return status, err
}

func (c *Sensonet) ModeText() string {
	switch c.conn.CurrentQuickmode() {
	case sensonetlib.QUICKMODE_HOTWATER:
		return " (Hotwater Boost active)"
	case sensonetlib.QUICKMODE_HEATING:
		if c.conn.QuickVetoExpiresAt() != "" {
			return " (Heating Quick Veto active. Ends " + c.conn.QuickVetoExpiresAt() + ")"
		}
		return " (Heating Quick Veto active)"
	case sensonetlib.QUICKMODE_NOTHING:
		return " (charger running idle)"
	}
	return " (regular mode; hotwater temp. shown)"
}
