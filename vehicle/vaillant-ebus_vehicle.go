package vehicle

import (
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/charger/vaillantEbus"
	"github.com/evcc-io/evcc/util"
)

// Vaillant-ebus_vehicle is an api.Vehicle implementation for Vaillant Vks heat pump controlled via ebus
type VaillantEbus_vehicle struct {
	*embed
	//	vehicle *Vehicle
	//	Title string
	PvUseStrategy string
	conn          *vaillantEbus.Connection
}

func init() {
	registry.Add("vaillant-ebus_vehicle", NewVaillantEbusVehicleFromConfig)
}

// NewVaillantEbusVehicleFromConfig creates a new vehicle
func NewVaillantEbusVehicleFromConfig(other map[string]interface{}) (api.Vehicle, error) {
	cc := struct {
		embed         `mapstructure:",squash"`
		PvUseStrategy string
	}{}

	if err := util.DecodeOther(other, &cc); err != nil {
		return nil, err
	}

	//Get pointer to the connection struct of the charger vaillant-ebus
	conn, err := vaillantEbus.GetVaillantEbusConn()
	if err != nil {
		return nil, err
	}

	v := &VaillantEbus_vehicle{
		embed:         &cc.embed,
		PvUseStrategy: cc.PvUseStrategy,
		conn:          conn,
	}

	if v.Title() == "" {
		v.SetTitle("VaillantEbus_V")
	}

	return v, nil
}

// apiError converts HTTP 408 error to ErrTimeout
/*func (v *VaillantEbus_vehicle) apiError(err error) error {
	if err != nil && err.Error() == "408 Request Timeout" {
		err = api.ErrAsleep
	}
	return err
}*/

// Soc implements the api.Vehicle interface
func (v *VaillantEbus_vehicle) Soc() (float64, error) {
	tt, err := v.conn.CurrentTemp()
	if err != nil {
		return 0, err
	}
	err = v.conn.CheckPVUseStrategy(v.PvUseStrategy)
	return float64(tt), err
}

//var _ api.ChargeState = (*VaillantEbus_vehicle)(nil)

// Status implements the api.ChargeState interface
func (v *VaillantEbus_vehicle) Status() (api.ChargeStatus, error) {
	status, err := v.conn.Status()
	if err != nil {
		return api.StatusA, err
	}
	return status, err
}

var _ api.SocLimiter = (*VaillantEbus_vehicle)(nil)

// TargetSoc implements the api.SocLimiter interface
func (v *VaillantEbus_vehicle) GetLimitSoc() (int64, error) {
	tt, err := v.conn.TargetTemp()
	if err != nil {
		return 0, err
	}
	return int64(tt), err
}

// StartCharge implements the api.VehicleChargeController interface
/*var _ api.Resurrector = (*VaillantEbus_vehicle)(nil)

func (v *VaillantEbus_vehicle) WakeUp() error {
	//_, err := v.vehicle.Wakeup()
	err := error(nil)
	//return apiError(err)
	return err
}

/*
var _ api.VehicleChargeController = (*VaillantEbus_vehicle)(nil)

// StartCharge implements the api.VehicleChargeController interface
func (v *VaillantEbus_vehicle) StartCharge() error {
	//_, err := v.vehicle.StartCharging()
	v.SetTitle("VaillantEbus_vehicle starting load process")
	err := error(nil)
	return v.apiError(err)
}

// StopCharge implements the api.VehicleChargeController interface
func (v *VaillantEbus_vehicle) StopCharge() error {
	//err := v.apiError(v.vehicle.StopCharging())
	v.SetTitle("VaillantEbus_vehicle stopping load process")
	err := error(nil)

	// ignore sleeping vehicle
	if errors.Is(err, api.ErrAsleep) {
		err = nil
	}

	return err
}*/
