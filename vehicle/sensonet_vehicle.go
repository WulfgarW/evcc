package vehicle

import (
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/charger/sensonet"
	"github.com/evcc-io/evcc/util"
)

// Sensonet_vehicle is an api.Vehicle implementation for Vaillant Vks heat pump controlled by sensonet
type Sensonet_vehicle struct {
	*embed
	PvUseStrategy string
	conn          *sensonet.Connection
}

func init() {
	registry.Add("sensonet_vehicle", NewSensonetVehicleFromConfig)
}

// NewSensonetVehicleFromConfig creates a new vehicle
func NewSensonetVehicleFromConfig(other map[string]interface{}) (api.Vehicle, error) {
	cc := struct {
		embed         `mapstructure:",squash"`
		PvUseStrategy string
	}{}

	if err := util.DecodeOther(other, &cc); err != nil {
		return nil, err
	}

	log := util.NewLogger("sensonet_vehicle")

	//Get pointer to the connection struct of the charger sensonet
	conn, err := sensonet.GetSensoNetConn()
	if err != nil {
		log.ERROR.Println(err)
		log.ERROR.Println("You can not use the sensonet_vehicle without a sensonet charger. You can ignore this message, if you are in evcc configure")
	}

	v := &Sensonet_vehicle{
		embed:         &cc.embed,
		PvUseStrategy: cc.PvUseStrategy,
		conn:          conn,
	}

	if v.GetTitle() == "" {
		v.SetTitle("Sensonet_V")
	}

	return v, nil
}

// apiError converts HTTP 408 error to ErrTimeout
/*func (v *Sensonet_vehicle) apiError(err error) error {
	if err != nil && err.Error() == "408 Request Timeout" {
		err = api.ErrAsleep
	}
	return err
}*/

// Soc implements the api.Vehicle interface
func (v *Sensonet_vehicle) Soc() (float64, error) {
	if _, err := sensonet.GetSensoNetConn(); err != nil {
		return 0, err
	}
	tt, err := v.conn.CurrentTemp()
	if err != nil {
		return 0, err
	}
	err = v.conn.CheckPVUseStrategy(v.PvUseStrategy)
	return float64(tt), err
}

//var _ api.ChargeState = (*Sensonet_vehicle)(nil)

// Status implements the api.ChargeState interface
func (v *Sensonet_vehicle) Status() (api.ChargeStatus, error) {
	if _, err := sensonet.GetSensoNetConn(); err != nil {
		return api.StatusA, err
	}
	status, err := v.conn.Status()
	if err != nil {
		return api.StatusA, err
	}
	return status, err
}

var _ api.SocLimiter = (*Sensonet_vehicle)(nil)

// TargetSoc implements the api.SocLimiter interface
func (v *Sensonet_vehicle) GetLimitSoc() (int64, error) {
	if _, err := sensonet.GetSensoNetConn(); err != nil {
		return 0, err
	}
	tt, err := v.conn.TargetTemp()
	if err != nil {
		return 0, err
	}
	return tt, err
}
