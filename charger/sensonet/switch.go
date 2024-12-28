package sensonet

import (
	"fmt"
	"time"

	sensonetlib "github.com/WulfgarW/sensonet"
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
	//var err error
	d := sh.Connection
	//Calling GetSystem to make sure that data are updated
	state, err := d.sensonetConn.GetSystem(d.systemId)
	if err != nil {
		d.log.ERROR.Println("switch.Enabled. Error: ", err)
		return d.onoff, err
	}
	d.log.DEBUG.Println("In Switch.Enabled:  Zones[0].CurrentSpecialFunction=", state.State.Zones[0].CurrentSpecialFunction)

	newQuickmode := d.sensonetConn.GetCurrentQuickMode()
	d.log.DEBUG.Printf("In Switch.Enabled: GetCurrentQuickmode() returns \"%s\"", newQuickmode)
	if newQuickmode == "" || newQuickmode == sensonetlib.QUICKMODE_NOTHING {
		d.onoff = false
	} else {
		d.onoff = true
	}
	return d.onoff, nil
}

// Enable implements the api.Charger interface
func (sh *Switch) Enable(enable bool) error {
	d := sh.Connection
	state, err := d.sensonetConn.GetSystem(d.systemId)
	if err != nil {
		d.log.ERROR.Println("connection.TargetTemp. Error: ", err)
		return err
	}
	zoneData := sensonetlib.GetZoneData(state, d.heatingZone)
	var heatingPar sensonetlib.HeatingParStruct
	var hotwaterPar sensonetlib.HotwaterParStruct
	heatingPar.ZoneIndex = d.heatingZone
	if zoneData != nil {
		heatingPar.VetoSetpoint = float32(d.heatingTemperatureOffset) + float32(zoneData.State.DesiredRoomTemperatureSetpoint)
	} else {
		heatingPar.VetoSetpoint = -1.0
	}
	heatingPar.VetoDuration = -1.0 //negative value means: use default
	hotwaterPar.Index = -1
	if enable {
		result, err := d.sensonetConn.StartStrategybased(d.systemId, PVUseStrategyToSensonetStrategy(d.pvUseStrategy), &heatingPar, &hotwaterPar)
		if err != nil {
			err = fmt.Errorf("error return from StartStrategybased: %s", err)
			return err
		}
		d.log.DEBUG.Println("In Switch.Enable: StartStrategybased returns: ", result)
		if result == sensonetlib.QUICKMODE_HEATING {
			if heatingPar.VetoSetpoint < 0.0 {
				d.quickVetoSetPoint = sensonetlib.ZONEVETOSETPOINT_DEFAULT
			} else {
				d.quickVetoSetPoint = heatingPar.VetoSetpoint
			}
			if heatingPar.VetoDuration < 0.0 {
				d.quickVetoExpiresAt = (time.Now().Add(time.Duration(int64(sensonetlib.ZONEVETODURATION_DEFAULT*60) * int64(time.Minute)))).Format("15:04")
			} else {
				d.quickVetoExpiresAt = (time.Now().Add(time.Duration(int64(heatingPar.VetoDuration*60) * int64(time.Minute)))).Format("15:04")
			}
		}
	} else {
		result, err := d.sensonetConn.StopStrategybased(d.systemId, PVUseStrategyToSensonetStrategy(d.pvUseStrategy), &heatingPar, &hotwaterPar)
		if err != nil {
			err = fmt.Errorf("error return from StopStrategybased: %s", err)
			return err
		}
		d.log.DEBUG.Println("In Switch.Enable: StopStrategybased returns: ", result)
		d.quickVetoSetPoint = 0.0
		d.quickVetoExpiresAt = ""
	}
	d.onoff = enable
	return err
}

// CurrentPower implements the api.Meter interface
// Those are just dummy values. For eal values, an energy meter like Shelly 3EM is necessary
func (sh *Switch) CurrentPower() (float64, error) {
	var power float64

	d := sh.Connection
	newQuickmode := d.sensonetConn.GetCurrentQuickMode()
	//d.log.DEBUG.Println("Switch.CurrentPower", d.currentQuickmode, d.quickmodeStarted.Format("2006-01-02 15:04:05"))

	// Returns dummy values for CurrentPower if called
	if d.onoff {
		power = 3000.0
	} else {
		power = 0.0
	}
	if newQuickmode == sensonetlib.QUICKMODE_HEATING {
		power = 1500.0
	}
	return power, nil
}

func PVUseStrategyToSensonetStrategy(strategyAsString string) int {
	switch strategyAsString {
	case PVUSESTRATEGY_HOTWATER:
		return sensonetlib.STRATEGY_HOTWATER
	case PVUSESTRATEGY_HEATING:
		return sensonetlib.STRATEGY_HEATING
	case PVUSESTRATEGY_HOTWATER_THEN_HEATING:
		return sensonetlib.STRATEGY_HOTWATER_THEN_HEATING
	default:
		fmt.Printf("Warning! No mapping for PVUseStrategy %s to a strategy in sensonet", strategyAsString)
		return 0
	}
}