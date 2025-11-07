package vaillantEbus

import (
	"fmt"
	"strings"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/request"

	sensonetEbus "github.com/WulfgarW/sensonetEbus"
)

// Connection is the Sensonet connection
type Connection struct {
	*request.Helper
	log           *util.Logger
	ebusdConn     *sensonetEbus.Connection
	pvUseStrategy string
	heatingZone   int
	phases        int
	//	heatingVetoDuration      int32
	heatingTemperatureOffset float64
	onoff                    bool
	quickVetoSetPoint        float32
	//quickVetoExpiresAt       string
}

// Global variable SensoNetConn is used to make data available in vehicle vks (not needed without vehicle vks)
var vaillantEbusConn *Connection
var vaillantEbusConnInitialised bool

// NewConnection creates a new Sensonet device connection.
func NewConnection(ebusdAddress, pvUseStrategy string, heatingZone, phases int, heatingTemperatureOffset float64) (*Connection, error) {
	if vaillantEbusConnInitialised {
		vaillantEbusConn.log.DEBUG.Println("In connection.NewConnection: vaillantEbusConn already initialised")
		return vaillantEbusConn, nil
	} else {
		log := util.NewLogger("vaillantEbus").Redact(ebusdAddress)
		client := request.NewHelper(log)
		conn := &Connection{
			Helper: client,
		}
		conn.pvUseStrategy = pvUseStrategy
		conn.heatingZone = heatingZone
		conn.phases = phases
		//	conn.heatingVetoDuration = heatingVetoDuration
		conn.heatingTemperatureOffset = heatingTemperatureOffset
		conn.log = log
		vaillantEbusConn = conn //this is not needed without vehicle vaillant-ebus_vehicle
		vaillantEbusConnInitialised = true

		var err error

		ebusdConn, err := sensonetEbus.NewConnection(ebusdAddress, sensonetEbus.WithLogger(log.TRACE))
		if err != nil {
			err = fmt.Errorf("sensonetEbuslib.NewConnection(). error: %s", err)
			return conn, err
		}
		conn.ebusdConn = ebusdConn

		details, err := ebusdConn.CheckEbusdConfig()
		if err == nil {
			log.DEBUG.Printf("   CheckEbusdConfig() returned no error. Details: \n%s \n", details)
			log.DEBUG.Println("   End of details")
		} else {
			if strings.Contains(fmt.Sprint(err), "Some ebus read commands got") {
				log.ERROR.Printf("   CheckEbusdConfig() returned: Details: \n%s , (Last) Error: %s \n", details, err)
			} else {
				log.WARN.Printf("   CheckEbusdConfig() returned: Details: \n%s , (Last) Error: %s \n This error will be ignord. \n", details, err)
				err = nil
			}
		}

		return conn, err
	}
}

func GetVaillantEbusConn() (*Connection, error) {
	if vaillantEbusConnInitialised {
		return vaillantEbusConn, nil
	} else {
		err := fmt.Errorf("Connection to Vaillant via ebus not initialised. ")
		return vaillantEbusConn, err
	}
}

/*

func (c *Connection) getSFMode(relData *VaillantRelDataStruct) error {
	var err error
	var findResult string

	c.ebusdConn, err = net.Dial("tcp", c.ebusdAddress)
	if err != nil {
		//err = fmt.Errorf("could not dial up to ebusd. error: %s", err)
		return err
	}
	defer c.ebusdConn.Close()
	c.ebusdReadBuffer = *bufio.NewReader(c.ebusdConn)
	//Getting SFMode for Hotwater
	findResult, err = c.ebusdRead(EBUSDREAD_HOTWATER_SFMODE, 0)
	if err != nil {
		return err
	} else {
		relData.Hotwater.HwcSFMode = findResult
	}

	//Getting Zone Data
	i := 0 //Index for relData.zones[]
	zonePrefix := fmt.Sprintf("z%01d", c.heatingZone)
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_SFMODE, 0)
	if err != nil {
		return err
	} else {
		relData.Zones[i].SFMode = findResult
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_QUICKVETOENDDATE, 0)
	if err != nil {
		return err
	} else {
		relData.Zones[i].QuickVetoEndDate = findResult
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_QUICKVETOENDTIME, 0)
	if err != nil {
		return err
	} else {
		relData.Zones[i].QuickVetoEndTime = findResult
	}
	c.log.DEBUG.Println("Timestamp for end of zone quick veto: ", relData.Zones[i].QuickVetoEndDate+" "+relData.Zones[i].QuickVetoEndTime)
	return nil
}
*/

func (d *Connection) Phases() int {
	return d.phases
}

func (d *Connection) currentQuickmode() string {
	return d.ebusdConn.GetCurrentQuickMode()
}

func (d *Connection) QuickVetoExpiresAt() string {
	return d.ebusdConn.GetQuickModeExpiresAt()
}

// CurrentTemp is called bei Soc
func (d *Connection) CurrentTemp() (float64, error) {
	state, err := d.ebusdConn.GetSystem(false)
	if err != nil {
		d.log.ERROR.Println("Connection.CurrentTemp. Error: ", err)
		return 0, err
	}
	hotWaterOn := false
	if state.Hotwater.HwcOpMode == sensonetEbus.OPERATIONMODE_AUTO {
		hotWaterOn = true
	}

	if d.currentQuickmode() == sensonetEbus.QUICKMODE_HEATING || !hotWaterOn {
		currentTemp := 5.0
		for _, z := range state.Zones {
			if currentTemp == 5.0 && z.RoomTemp > currentTemp {
				currentTemp = z.RoomTemp
			}
			if z.Index == d.heatingZone && z.RoomTemp != 0.0 {
				currentTemp = z.RoomTemp
			}
		}
		return currentTemp, nil
	}
	return state.Hotwater.HwcStorageTemp, nil
}

// TargetTemp is called bei TargetSoc
func (d *Connection) TargetTemp() (int64, error) {
	state, err := d.ebusdConn.GetSystem(false)
	if err != nil {
		d.log.ERROR.Println("Switch.TargetTemp. Error: ", err)
		return 0, err
	}
	hotWaterOn := false
	if state.Hotwater.HwcOpMode == sensonetEbus.OPERATIONMODE_AUTO {
		hotWaterOn = true
	}

	if d.currentQuickmode() == sensonetEbus.QUICKMODE_HEATING {
		z := sensonetEbus.GetZoneData(state.Zones, d.heatingZone)
		if z.QuickVetoTemp > 0 {
			return int64(z.QuickVetoTemp), nil
		} else {
			return int64(d.quickVetoSetPoint), nil
		}
	}
	if !hotWaterOn {
		z := sensonetEbus.GetZoneData(state.Zones, d.heatingZone)
		return int64(z.ActualRoomTempDesired), nil
	}
	return int64(state.Hotwater.HwcTempDesired), nil
}

// CheckPVUseStrategy is called bei vaillant-ebus_vehicle.Soc()
func (d *Connection) CheckPVUseStrategy(vehicleStrategy string) error {
	if d.pvUseStrategy != vehicleStrategy && vehicleStrategy != "not_set" {
		d.log.INFO.Printf("Changing PVUseStrategy of charger from '%s' to '%s'", d.pvUseStrategy, vehicleStrategy)
		d.pvUseStrategy = vehicleStrategy
	}
	return nil
}

func (d *Connection) Status() (api.ChargeStatus, error) {
	state, _ := d.ebusdConn.GetSystem(false)
	status := api.StatusB
	if time.Now().Add(time.Duration(-4 * int64(time.Minute))).After(state.LastGetSystem) {
		//It is not expected, that the last update of the get system value is longer ago than 4 minutes
		status = api.StatusA // disconnected
	}
	if d.currentQuickmode() != "" {
		status = api.StatusC
	}
	return status, nil
}

func (d *Connection) ModeText() string {
	state, err := d.ebusdConn.GetSystem(false)
	if err != nil {
		d.log.ERROR.Println("connection.TargetTemp. Error: ", err)
		return ""
	}
	hotWaterOn := false
	if state.Hotwater.HwcOpMode == sensonetEbus.OPERATIONMODE_AUTO {
		hotWaterOn = true
	}

	tempInfo := "hotwater temp. shown)"
	if d.currentQuickmode() == sensonetEbus.QUICKMODE_HEATING || !hotWaterOn {
		tempInfo = "heating temp. shown)"
	}

	switch d.currentQuickmode() {
	case sensonetEbus.QUICKMODE_HOTWATER:
		return " (Hotwater Boost active)"
	case sensonetEbus.QUICKMODE_HEATING:
		if d.ebusdConn.GetQuickModeExpiresAt() != "" {
			return " (Heating Quick Veto active. Ends " + d.ebusdConn.GetQuickModeExpiresAt() + ")"
		}
		return " (Heating Quick Veto active)"
	case sensonetEbus.QUICKMODE_NOTHING:
		if d.ebusdConn.GetQuickModeExpiresAt() != "" {
			return " (charger running idle. Ends " + d.ebusdConn.GetQuickModeExpiresAt() + "; " + tempInfo
		}
		return " (charger running idle; " + tempInfo
	}
	return " (regular mode; " + tempInfo
}
