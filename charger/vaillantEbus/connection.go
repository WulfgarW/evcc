package vaillantEbus

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"

	//"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/request"
)

// Connection is the Sensonet connection
type Connection struct {
	*request.Helper
	log             *util.Logger
	ebusdAddress    string
	ebusdConn       net.Conn
	ebusdReadBuffer bufio.Reader
	lastGetSystemAt time.Time
	pvUseStrategy   string
	heatingZone     int
	phases          int
	//	heatingVetoDuration      int32
	heatingTemperatureOffset float64
	currentQuickmode         string
	quickmodeStarted         time.Time
	quickmodeStopped         time.Time
	onoff                    bool
	quickVetoSetPoint        float32
	quickVetoExpiresAt       string
	relData                  VaillantRelDataStruct
	getSystemUpdateInterval  time.Duration
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
		log := util.NewLogger("vaillantEbus")
		client := request.NewHelper(log)
		conn := &Connection{
			Helper: client,
		}
		conn.ebusdAddress = ebusdAddress
		conn.pvUseStrategy = pvUseStrategy
		conn.heatingZone = heatingZone
		conn.phases = phases
		//	conn.heatingVetoDuration = heatingVetoDuration
		conn.heatingTemperatureOffset = heatingTemperatureOffset
		conn.log = log
		conn.currentQuickmode = ""
		conn.quickmodeStarted = time.Now()
		conn.getSystemUpdateInterval = 2 * time.Minute
		vaillantEbusConn = conn //this is not needed without vehicle vaillant-ebus_vehicle
		vaillantEbusConnInitialised = true

		var err error

		err = conn.connectToEbusd()
		if err != nil {
			//err = fmt.Errorf("could not connect to ebusd", err)
			return conn, err
		}

		//var res VaillantRelDataStruct
		err = conn.getSystem(&conn.relData, true)
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

func (c *Connection) connectToEbusd() error {
	var err error
	c.ebusdConn, err = net.Dial("tcp", c.ebusdAddress)
	if err != nil {
		//err = fmt.Errorf("could not dial up to ebusd. error: %s", err)
		return err
	}
	defer c.ebusdConn.Close()
	c.ebusdReadBuffer = *bufio.NewReader(c.ebusdConn)
	scanResult := c.ebusdScanResult()
	if scanResult == "" {
		c.log.ERROR.Println("Scan result empty")
		err = fmt.Errorf("empty scan result or error returned from ebusd: %s", scanResult)
		return err
	}
	c.log.DEBUG.Printf("Scan result= %s\n", scanResult)
	c.relData.Status.ControllerForSFMode = c.ebusdFindControllerForSFMode()
	if c.relData.Status.ControllerForSFMode == "" {
		c.log.ERROR.Println("Find result empty")
		err = fmt.Errorf("empty find %s or error returned from ebusd: %s", EBUSDREAD_HOTWATER_SFMODE, c.relData.Status.ControllerForSFMode)
		return err
	}
	c.log.INFO.Printf("Ebus Controller For SFMode= %s\n", c.relData.Status.ControllerForSFMode)
	return err
}

func (c *Connection) ebusdScanResult() string {
	var message, messageLine string
	var err error
	fmt.Fprintf(c.ebusdConn, "scan result\n")
	message = ""
	err = nil
	for err == nil {
		messageLine, err = c.ebusdReadBuffer.ReadString('\n')
		message = message + messageLine
		if err != nil || messageLine == "\n" {
			//err = fmt.Errorf("could not receive from ebusd. error: %s", err)
			return message
		}
	}
	return message
}

func (c *Connection) ebusdFindControllerForSFMode() string {
	_, err := fmt.Fprintf(c.ebusdConn, "find "+EBUSDREAD_HOTWATER_SFMODE+"\n")
	if err != nil {
		//err = fmt.Errorf("could not write to ebusd for ebusdFindControllerForSFMode. error: %s", err)
		c.log.ERROR.Printf("Error sending find command to ebusd: %s", err)
		return ""
	}
	var message string
	message, err = c.ebusdReadBuffer.ReadString('\n')
	if err != nil {
		c.log.ERROR.Printf("Error when reading from ebusd: %s", err)
		return ""
	}
	message = strings.TrimSpace(message)
	if message[:min(4, len(message))] == "ERR:" {
		c.log.INFO.Printf("When trying to find controller for SFMode, ebusd answered: %s", message)
	}
	strSlices := strings.SplitAfter(message, " ")
	message = strings.TrimSpace(strSlices[0])
	return message
}

func isNetConnClosedErr(err error) bool {
	switch {
	case
		errors.Is(err, net.ErrClosed),
		errors.Is(err, io.EOF),
		errors.Is(err, syscall.EPIPE):
		return true
	default:
		if strings.Contains(err.Error(), "wsasend") {
			return true
		}
		return false
	}
}

func (c *Connection) ebusdRead(searchString string, notOlderThan int) (string, error) {
	var err error
	var ebusCommand string
	if notOlderThan >= 0 {
		ebusCommand = fmt.Sprintf("read -m %0d ", notOlderThan)
	} else {
		ebusCommand = "read "
	}
	message := "ERR: no signal"
	readTry := 0
	buf := c.ebusdReadBuffer

	for message[:min(4, len(message))] == "ERR:" && readTry < 3 {
		_, err = fmt.Fprintf(c.ebusdConn, ebusCommand+searchString+"\n")
		if err != nil {
			c.log.ERROR.Printf("Error sending read command to ebusd: %s", err)
			if isNetConnClosedErr(err) {
				c.log.DEBUG.Println("Connection to ebusd is closed. Trying to reopen it.")
				err = c.refreshEbusdConnection()
				if err != nil {
					c.log.ERROR.Printf("refreshEbusdConnection not successful: %s", err)
					return "", err
				} else {
					_, err = fmt.Fprintf(c.ebusdConn, ebusCommand+searchString+"\n")
					if err != nil {
						c.log.ERROR.Printf("Error sending read command to ebusd: %s", err)
						return "", err
					}
					buf = c.ebusdReadBuffer
					readTry = 0
				}
			} else {
				return "", err
			}
		}
		message, err = buf.ReadString('\n')
		if err != nil && readTry > 1 {
			c.log.ERROR.Printf("Error when reading from ebusd: %s", err)
			return "", err
		}
		readTry = readTry + 1
		if readTry < 3 && message[:min(4, len(message))] == "ERR:" {
			c.log.DEBUG.Printf("Read try no. %d, Command: %s, ebusd answered: %s", readTry, ebusCommand+searchString, message)
		}
	}
	message = strings.TrimSpace(message)
	if message[:min(4, len(message))] == "ERR:" {
		c.log.INFO.Printf("Command: %s, ebusd answered: %s", ebusCommand+searchString, message)
		message = "ERR:"
	}
	return message, err
}

/*func (c *Connection) ebusdEmptyReadBuffer() error {
	var err error
	//buf := bufio.NewReader(c.ebusdConn)
	buf := c.ebusdReadBuffer

	for buf.Buffered() > 0 {
		fmt.Println("Bytes in Buffer:", buf.Buffered())
		_, err = buf.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			} else {
				c.log.ERROR.Printf("Error while emptying read buffer to ebusd: %s", err)
				return err
			}
		}
	}
	return nil
}*/

func (c *Connection) ebusdWrite(message string) error {
	var err error
	c.ebusdConn, err = net.Dial("tcp", c.ebusdAddress)
	if err != nil {
		//err = fmt.Errorf("could not dial up to ebusd. error: %s", err)
		return err
	}
	defer c.ebusdConn.Close()
	c.ebusdReadBuffer = *bufio.NewReader(c.ebusdConn)
	_, err = fmt.Fprintf(c.ebusdConn, "write "+message+"\n")
	if err != nil {
		c.log.ERROR.Printf("Error writing to ebusd: %s", err)
		return err
	}
	var ebusAnswer string
	ebusAnswer, err = c.ebusdReadBuffer.ReadString('\n')
	fmt.Println("Antwort auf write:", ebusAnswer)
	if err != nil {
		c.log.ERROR.Printf("Error when reading answer after ebusd write: %s", err)
		return err
	}
	if strings.TrimSpace(ebusAnswer) != "done" {
		c.log.INFO.Printf("Command: %s, ebusd answered: %s", "write "+message, ebusAnswer)
	}
	return err
}

func (c *Connection) refreshEbusdConnection() error {
	var err error
	c.ebusdConn, err = net.Dial("tcp", c.ebusdAddress)
	if err != nil {
		return err
	}
	c.ebusdReadBuffer = *bufio.NewReader(c.ebusdConn)
	return nil
}

func (c *Connection) getSystem(relData *VaillantRelDataStruct, reset bool) error {
	var err error
	var findResult string
	var convertedValue float64
	if !reset && time.Now().Before(c.lastGetSystemAt.Add(c.getSystemUpdateInterval)) {
		// Use relData that are already present instead of reading current data from ebusd
		return nil
	}
	c.ebusdConn, err = net.Dial("tcp", c.ebusdAddress)
	if err != nil {
		return err
	}
	defer c.ebusdConn.Close()
	c.ebusdReadBuffer = *bufio.NewReader(c.ebusdConn)
	//Getting Data for Hotwater
	findResult = ""
	for !slices.Contains([]string{"off", "auto", "day", "ERR:"}, findResult) && err == nil {
		findResult, err = c.ebusdRead(EBUSDREAD_HOTWATER_OPMODE, -1)
	}
	if err != nil {
		return err
	} else {
		if slices.Contains([]string{"off", "auto", "day"}, findResult) {
			relData.Hotwater.HwcOpMode = findResult
		} else {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_HOTWATER_OPMODE)
		}
	}
	findResult, err = c.ebusdRead(EBUSDREAD_HOTWATER_TEMPDESIRED, -1)
	if err != nil {
		return err
	} else {
		convertedValue, err = strconv.ParseFloat(findResult, 64)
		if err != nil || convertedValue <= 0 {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_HOTWATER_TEMPDESIRED)
		} else {
			relData.Hotwater.HwcTempDesired = convertedValue
		}
	}
	findResult, err = c.ebusdRead(EBUSDREAD_HOTWATER_STORAGETEMP, 60)
	if err != nil {
		return err
	} else {
		convertedValue, err = strconv.ParseFloat(findResult, 64)
		if err != nil || convertedValue <= 0 {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_HOTWATER_STORAGETEMP)
		} else {
			relData.Hotwater.HwcStorageTemp = convertedValue
		}
	}
	findResult, err = c.ebusdRead(EBUSDREAD_HOTWATER_SFMODE, 0)
	if err != nil {
		return err
	} else {
		if slices.Contains([]string{"load", "auto"}, findResult) {
			relData.Hotwater.HwcSFMode = findResult
		} else {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_HOTWATER_SFMODE)
		}
	}

	//Getting General Status Data
	findResult, err = c.ebusdRead(EBUSDREAD_STATUS_TIME, -1)
	if err != nil {
		return err
	} else {
		relData.Status.Time = findResult
	}
	findResult, err = c.ebusdRead(EBUSDREAD_STATUS_OUTSIDETEMPERATURE, -1)
	if err != nil {
		return err
	} else {
		relData.Status.OutsideTemperature, _ = strconv.ParseFloat(findResult, 64)
	}
	findResult, err = c.ebusdRead(EBUSDREAD_STATUS_SYSTEMFLOWTEMPERATUE, -1)
	if err != nil {
		return err
	} else {
		relData.Status.SystemFlowTemperature, _ = strconv.ParseFloat(findResult, 64)
	}
	findResult, err = c.ebusdRead(EBUSDREAD_STATUS_WATERPRESSURE, -1)
	if err != nil {
		return err
	} else {
		relData.Status.WaterPressure, _ = strconv.ParseFloat(findResult, 64)
	}
	findResult, err = c.ebusdRead(EBUSDREAD_STATUS_CURRENTCONSUMEDPOWER, 60)
	if err != nil {
		return err
	} else {
		convertedValue, err = strconv.ParseFloat(findResult, 64)
		if err != nil || convertedValue < 0 {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_STATUS_CURRENTCONSUMEDPOWER)
		} else {
			relData.Status.CurrentConsumedPower = convertedValue
		}
	}
	findResult, err = c.ebusdRead(EBUSDREAD_STATUS_STATUS01, -1)
	if err != nil {
		return err
	} else {
		relData.Status.Status01 = findResult
		c.log.DEBUG.Printf("Info: Value '%s' returnd from ebusd for %s", findResult, EBUSDREAD_STATUS_STATUS01)
	}
	findResult, err = c.ebusdRead(EBUSDREAD_STATUS_STATE, -1)
	if err != nil {
		return err
	} else {
		relData.Status.State = findResult
		c.log.DEBUG.Printf("Info: Value '%s' returnd from ebusd for %s", findResult, EBUSDREAD_STATUS_STATE)
	}

	//Getting Zone Data
	if len(relData.Zones) == 0 {
		relData.Zones = make([]VaillantRelDataZonesStruct, 1)
	}
	i := 0 //Index for relData.zones[]
	zonePrefix := fmt.Sprintf("z%01d", c.heatingZone)
	relData.Zones[i].Index = c.heatingZone
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_NAME, -1)
	if err != nil {
		return err
	} else {
		relData.Zones[i].Name = findResult
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_OPMODE, -1)
	if err != nil {
		return err
	} else {
		if slices.Contains([]string{"off", "auto", "day"}, findResult) {
			relData.Zones[i].OpMode = findResult
		} else {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_ZONE_OPMODE)
		}
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_SFMODE, 0)
	if err != nil {
		return err
	} else {
		if slices.Contains([]string{"auto", "veto"}, findResult) {
			relData.Zones[i].SFMode = findResult
		} else {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_ZONE_SFMODE)
		}
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_ACTUALROOMTEMPDESIRED, -1)
	if err != nil {
		return err
	} else {
		convertedValue, err = strconv.ParseFloat(findResult, 64)
		if err != nil || convertedValue <= 0 {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_ZONE_ACTUALROOMTEMPDESIRED)
		} else {
			relData.Zones[i].ActualRoomTempDesired = convertedValue
		}
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_ROOMTEMP, 180)
	if err != nil {
		return err
	} else {
		convertedValue, err = strconv.ParseFloat(findResult, 64)
		if err != nil || convertedValue <= 0 {
			c.log.DEBUG.Printf("Value '%s' returnd from ebusd for %s invalid and therefore ignored", findResult, EBUSDREAD_ZONE_ROOMTEMP)
		} else {
			relData.Zones[i].RoomTemp = convertedValue
		}
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_QUICKVETOTEMP, 0)
	if err != nil {
		return err
	} else {
		relData.Zones[i].QuickVetoTemp, _ = strconv.ParseFloat(findResult, 64)
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_QUICKVETOENDDATE, -1)
	if err != nil {
		return err
	} else {
		relData.Zones[i].QuickVetoEndDate = findResult
	}
	findResult, err = c.ebusdRead(zonePrefix+EBUSDREAD_ZONE_QUICKVETOENDTIME, -1)
	if err != nil {
		return err
	} else {
		relData.Zones[i].QuickVetoEndTime = findResult
		if relData.Zones[i].SFMode == "veto" {
			c.quickVetoExpiresAt = relData.Zones[i].QuickVetoEndTime
		}
	}

	//Set timestamp lastGetSystemAt and return nil error
	c.lastGetSystemAt = time.Now()
	return nil
}

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

func (d *Connection) Phases() int {
	return d.phases
}

func (d *Connection) CurrentQuickmode() string {
	return d.currentQuickmode
}

func (d *Connection) QuickVetoExpiresAt() string {
	return d.quickVetoExpiresAt

}

// CurrentTemp is called bei Soc
func (d *Connection) CurrentTemp() (float64, error) {
	err := d.getSystem(&d.relData, false)
	if err != nil {
		d.log.ERROR.Println("Switch.CurrentTemp. Error: ", err)
		return 0, err
	}
	if d.CurrentQuickmode() == QUICKMODE_HEATING {
		currentTemp := 5.0
		for _, z := range d.relData.Zones {
			if currentTemp == 5.0 && z.RoomTemp > currentTemp {
				currentTemp = z.RoomTemp
			}
			if z.Index == d.heatingZone && z.RoomTemp != 0.0 {
				currentTemp = z.RoomTemp
			}
		}
		return currentTemp, nil
	}
	return d.relData.Hotwater.HwcStorageTemp, nil
}

// TargetTemp is called bei TargetSoc
func (d *Connection) TargetTemp() (float64, error) {
	err := d.getSystem(&d.relData, false)
	if err != nil {
		d.log.ERROR.Println("Switch.TargetTemp. Error: ", err)
		return 0, err
	}
	if d.CurrentQuickmode() == QUICKMODE_HEATING {
		for _, z := range d.relData.Zones {
			if z.Index == d.heatingZone {
				//return z.ActualRoomTempDesired, nil
				return z.QuickVetoTemp, nil
			}
		}
		return float64(d.quickVetoSetPoint), nil
	}
	return d.relData.Hotwater.HwcTempDesired, nil
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
	status := api.StatusB
	if time.Now().Add(time.Duration(-4 * int64(time.Minute))).After(d.lastGetSystemAt) {
		status = api.StatusA // disconnected
	}
	if d.CurrentQuickmode() != "" {
		status = api.StatusC
	}
	return status, nil
}

// This function checks the operation mode of heating and hotwater and the hotwater live temperature
// and returns, which quick mode should be started, when evcc sends an "Enable"
func (c *Connection) WhichQuickMode() (int, error) {
	err := c.getSystem(&c.relData, false)
	if err != nil {
		err = fmt.Errorf("could not read current status information in WhichQuickMode(): %s", err)
		return 0, err
	}
	//c.log.DEBUG.Println("PV Use Strategy = ", c.pvUseStrategy)
	c.log.DEBUG.Printf("Checking if hot water boost possible. Operation Mode = %s, temperature setpoint= %02.2f, live temperature= %02.2f",
		c.relData.Hotwater.HwcOpMode, c.relData.Hotwater.HwcTempDesired, c.relData.Hotwater.HwcStorageTemp)
	hotWaterBoostPossible := false
	// For pvUseStrategy='hotwater', a hotwater boost is possible when hotwater storage temperature is less than the temperature setpoint.
	// For other pvUseStrategies, a hotwater boost is possible when hotwater storage temperature is less than the temperature setpoint minus 5°C
	addOn := -5.0
	if c.pvUseStrategy == PVUSESTRATEGY_HOTWATER {
		addOn = 0.0
	}
	if c.relData.Hotwater.HwcStorageTemp < c.relData.Hotwater.HwcTempDesired+addOn &&
		c.relData.Hotwater.HwcOpMode == OPERATIONMODE_AUTO {
		hotWaterBoostPossible = true
	}

	heatingQuickVetoPossible := false
	for _, z := range c.relData.Zones {
		if z.Index == c.heatingZone {
			c.log.DEBUG.Printf("Checking if heating quick veto possible. Operation Mode = %s", z.OpMode)
			if z.OpMode == OPERATIONMODE_AUTO {
				heatingQuickVetoPossible = true
			}
		}
	}

	whichQuickMode := 0
	switch c.pvUseStrategy {
	case PVUSESTRATEGY_HOTWATER:
		if hotWaterBoostPossible {
			whichQuickMode = 1
		} else {
			c.log.DEBUG.Println("PV Use Strategy = hotwater, but hotwater boost not possible")
		}
	case PVUSESTRATEGY_HEATING:
		if heatingQuickVetoPossible {
			whichQuickMode = 2
		} else {
			c.log.DEBUG.Println("PV Use Strategy = heating, but heating quick veto not possible")
		}
	case PVUSESTRATEGY_HOTWATER_THEN_HEATING:
		if hotWaterBoostPossible {
			whichQuickMode = 1
		} else {
			if heatingQuickVetoPossible {
				whichQuickMode = 2
			} else {
				c.log.DEBUG.Println("PV Use Strategy = hotwater_then_heating, but both not possible")
			}
		}
	}
	return whichQuickMode, err
}
