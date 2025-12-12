package vaillantEbus

import (
	"fmt"
	//"math"
	//"time"

	"github.com/WulfgarW/sensonetEbus"
)

type Switch struct {
	*Connection
}

func NewSwitch(conn *Connection) *Switch {
	res := &Switch{
		Connection: conn,
	}

	return res
}

// Enabled implements the api.Charger interface
func (sh *Switch) Enabled() (bool, error) {
	d := sh.Connection
	//Calling GetSystem to make sure that data are updated
	state, err := d.ebusdConn.GetSystem(false)
	if err != nil {
		d.log.ERROR.Println("switch.Enabled. Error: ", err)
		return d.onoff, err
	}
	d.log.DEBUG.Println("Status last read from ebusd at:", state.LastGetSystem)

	newQuickmode := d.ebusdConn.GetCurrentQuickMode()
	d.log.DEBUG.Printf("In Switch.Enabled: GetCurrentQuickmode() returns \"%s\"", newQuickmode)
	if newQuickmode == "" { //|| newQuickmode == sensonetEbus.QUICKMODE_NOTHING {
		d.onoff = false
	} else {
		d.onoff = true
	}
	return d.onoff, nil
}

// Enable implements the api.Charger interface
func (sh *Switch) Enable(enable bool) error {
	d := sh.Connection
	state, err := d.ebusdConn.GetSystem(true)
	if err != nil {
		d.log.ERROR.Println("connection.Enable. Error: ", err)
		return err
	}
	zoneData := sensonetEbus.GetZoneData(state.Zones, d.heatingZone)
	var heatingPar sensonetEbus.HeatingParStruct
	heatingPar.ZoneIndex = d.heatingZone
	if zoneData != nil {
		heatingPar.VetoSetpoint = float32(d.heatingTemperatureOffset) + float32(zoneData.ActualRoomTempDesired)
	} else {
		heatingPar.VetoSetpoint = -1.0
	}
	heatingPar.VetoDuration = -1.0 //negative value means: use default
	if enable {
		d.log.DEBUG.Println("In Switch.Enable: PVUseStrategy= ", d.pvUseStrategy, ", converted to ", PVUseStrategyToSensonetEbusStrategy(d.pvUseStrategy), "for sensonetEbus.")
		result, err := d.ebusdConn.StartStrategybased(PVUseStrategyToSensonetEbusStrategy(d.pvUseStrategy), &heatingPar)
		if err != nil {
			err = fmt.Errorf("error return from StartStrategybased: %s", err)
			return err
		}
		d.log.DEBUG.Println("In Switch.Enable: StartStrategybased returns: ", result)
		if result == sensonetEbus.QUICKMODE_HEATING {
			if heatingPar.VetoSetpoint < 0.0 {
				d.quickVetoSetPoint = sensonetEbus.ZONEVETOSETPOINT_DEFAULT
			} else {
				d.quickVetoSetPoint = heatingPar.VetoSetpoint
			}
			/*if heatingPar.VetoDuration < 0.0 {
				d.quickVetoExpiresAt = (time.Now().Add(time.Duration(int64(sensonetEbus.ZONEVETODURATION_DEFAULT*60) * int64(time.Minute)))).Format("15:04")
			} else {
				d.quickVetoExpiresAt = (time.Now().Add(time.Duration(int64(heatingPar.VetoDuration*60) * int64(time.Minute)))).Format("15:04")
			}*/
		}
	} else {
		result, err := d.ebusdConn.StopStrategybased(&heatingPar)
		if err != nil {
			err = fmt.Errorf("error return from StopStrategybased: %s", err)
			return err
		}
		d.log.DEBUG.Println("In Switch.Enable: StopStrategybased returns: ", result)
		d.quickVetoSetPoint = 0.0
		//d.quickVetoExpiresAt = ""
	}
	d.onoff = enable
	return err
}

// CurrentPower implements the api.Meter interface
// Those are just dummy values. For eal values, an energy meter like Shelly 3EM is necessary
func (sh *Switch) CurrentPower() (float64, error) {
	d := sh.Connection
	power, err := d.ebusdConn.GetSystemCurrentPower()
	if err != nil {
		d.log.DEBUG.Println("In Switch.CurrentPower. GetSystemCurrentPower returned error: ", err)
		power = 0.0
		//err = d.getSFMode(&d.relData) //Update SFMode for hotwater and heating zone
	}
	d.log.DEBUG.Println("Switch.CurrentPower", d.currentQuickmode(), "Power:", power)
	return power * 1000, nil
}

func PVUseStrategyToSensonetEbusStrategy(strategyAsString string) int {
	switch strategyAsString {
	case PVUSESTRATEGY_HOTWATER:
		return sensonetEbus.STRATEGY_HOTWATER
	case PVUSESTRATEGY_HEATING:
		return sensonetEbus.STRATEGY_HEATING
	case PVUSESTRATEGY_HOTWATER_THEN_HEATING:
		return sensonetEbus.STRATEGY_HOTWATER_THEN_HEATING
	default:
		fmt.Printf("Warning! No mapping for PVUseStrategy %s to a strategy in sensonet", strategyAsString)
		return 0
	}
}
