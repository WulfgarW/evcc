package vehicle

import (
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/charger/sensonet"
	"github.com/evcc-io/evcc/util"
)

// Sensonet_vehicle is an api.Vehicle implementation for Vaillant Vks heat pump controlled by sensonet
type Sensonet_vehicle struct {
	*embed
	//	vehicle *Vehicle
	//	Title string
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

	//Get pointer to the connection struct of the charger sensonet
	conn, err := sensonet.GetSensoNetConn()
	if err != nil {
		return nil, err
	}

	v := &Sensonet_vehicle{
		embed:         &cc.embed,
		PvUseStrategy: cc.PvUseStrategy,
		conn:          conn,
	}

	if v.Title() == "" {
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
	status, err := v.conn.Status()
	if err != nil {
		return api.StatusA, err
	}
	return status, err
}

var _ api.SocLimiter = (*Sensonet_vehicle)(nil)

// TargetSoc implements the api.SocLimiter interface
func (v *Sensonet_vehicle) GetLimitSoc() (int64, error) {
	tt, err := v.conn.TargetTemp()
	if err != nil {
		return 0, err
	}
	return int64(tt), err
}

// StartCharge implements the api.VehicleChargeController interface
/*var _ api.Resurrector = (*Sensonet_vehicle)(nil)

func (v *Sensonet_vehicle) WakeUp() error {
	//_, err := v.vehicle.Wakeup()
	err := error(nil)
	return v.apiError(err)
}

var _ api.VehicleChargeController = (*Sensonet_vehicle)(nil)

// StartCharge implements the api.VehicleChargeController interface
func (v *Sensonet_vehicle) StartCharge() error {
	//_, err := v.vehicle.StartCharging()
	v.SetTitle("Sensonet_vehicle starting load process")
	err := error(nil)
	return v.apiError(err)
}

// StopCharge implements the api.VehicleChargeController interface
func (v *Sensonet_vehicle) StopCharge() error {
	//err := v.apiError(v.vehicle.StopCharging())
	v.SetTitle("Sensonet_vehicle stopping load process")
	err := error(nil)

	// ignore sleeping vehicle
	if errors.Is(err, api.ErrAsleep) {
		err = nil
	}

	return err
}*/
