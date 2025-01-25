package sensonet

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"os"
	"time"

	"golang.org/x/oauth2"

	"github.com/ernesto-jimenez/httplogger"
	"github.com/spf13/jwalterweatherman"

	sensonetlib "github.com/WulfgarW/sensonet"
	"github.com/evcc-io/evcc/api"

	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/request"
)

// Connection is the Sensonet connection
type Connection struct {
	client        *http.Client
	log           *util.Logger
	user          string
	password      string
	realm         string
	sensonetCtrl  *sensonetlib.Controller
	systemId      string
	pvUseStrategy string
	heatingZone   int
	phases        int
	//	heatingVetoDuration      int32
	heatingTemperatureOffset float64
	onoff                    bool
	quickVetoSetPoint        float32
	//quickVetoExpiresAt       string
}

// Global variable sensoNetConn is used to make data available in vehicle sensonet-vehicle
// Global variable sensoNetConnInitialised shows that sensoNetConn is already initialised
var sensoNetConn *Connection
var sensoNetConnInitialised bool

// Block for httplogging
var Timeout = 10 * time.Second

type httpLogger struct {
	log *log.Logger
}

func newLogger(log *log.Logger) *httpLogger {
	return &httpLogger{
		log: log,
	}
}
func (l *httpLogger) LogRequest(req *http.Request) {
	l.log.Printf(
		"Request %s %s",
		req.Method,
		req.URL.String(),
	)
}

func (l *httpLogger) LogResponse(req *http.Request, res *http.Response, err error, duration time.Duration) {
	duration /= time.Millisecond
	if err != nil {
		l.log.Println(err)
	} else {
		l.log.Printf(
			"Response method=%s status=%d durationMs=%d %s",
			req.Method,
			res.StatusCode,
			duration,
			req.URL.String(),
		)
	}
}

//End of block for httplogging

// NewConnection creates a new Sensonet device connection.
func NewConnection(user, password, realm, pvUseStrategy string, heatingZone, phases int, heatingTemperatureOffset float64) (*Connection, error) {
	if sensoNetConnInitialised {
		sensoNetConn.log.DEBUG.Println("In connection.NewConnection: sensoNetConn already initialised")
		return sensoNetConn, nil
	} else {
		utillog := util.NewLogger("sensonet").Redact(user, password)
		client := request.NewClient(utillog)
		ctxClient := request.NewClient(utillog)

		conn := &Connection{
			client: client,
		}
		conn.user = user
		conn.password = password
		conn.realm = realm
		conn.pvUseStrategy = pvUseStrategy
		conn.heatingZone = heatingZone
		conn.phases = phases
		//	conn.heatingVetoDuration = heatingVetoDuration
		conn.heatingTemperatureOffset = heatingTemperatureOffset
		conn.log = utillog

		var err error

		ctx := context.WithValue(context.TODO(), oauth2.HTTPClient, ctxClient)
		clientCtx := context.WithValue(ctx, oauth2.HTTPClient, ctxClient)
		oc := sensonetlib.Oauth2ConfigForRealm(conn.realm)
		token, err := oc.PasswordCredentialsToken(clientCtx, conn.user, conn.password)
		if err != nil {
			utillog.FATAL.Println(err)
		}
		utillog.DEBUG.Println("In connection.NewConnection: Call of sensonetlib.oc.PasswordCredentialsToken() successful")
		utillog.DEBUG.Println("Got new Token. Vaild until: ", token.Expiry)
		//Activate httplogger and logging in sensonetlib for log levels TRACE or DEBUG
		if util.WWlogLevelForArea("sensonet") == jwalterweatherman.LevelTrace || util.WWlogLevelForArea("sensonet") == jwalterweatherman.LevelDebug {
			log.SetOutput(os.Stderr) //changing output of stadard log to os.stderr. (In main.go, it is set to io.Discard)
			log := log.New(os.Stderr, "sensonet: ", log.Lshortfile)
			client.Transport = httplogger.NewLoggedTransport(http.DefaultTransport, newLogger(log))
		}
		snconn, err := sensonetlib.NewConnection(oc.TokenSource(clientCtx, token), sensonetlib.WithHttpClient(client))
		if err != nil {
			err = fmt.Errorf("sensonetlib.NewConnection(). error: %s", err)
			return conn, err
		}
		snctrl, err := sensonetlib.NewController(snconn, sensonetlib.WithLogger(utillog.DEBUG))
		if err != nil {
			err = fmt.Errorf("sensonet.NewController(). error: %s", err)
			return conn, err
		}
		utillog.DEBUG.Println("In connection.NewConnection: Call of sensonetlib.NewConnection() successful")
		conn.sensonetCtrl = snctrl
		homes, err := conn.sensonetCtrl.GetHomes()
		if err != nil {
			err = fmt.Errorf("could not get Homes[] information. error: %s", err)
			return conn, err
		}
		conn.systemId = homes[0].SystemID

		sensoNetConn = conn
		sensoNetConnInitialised = true
		return conn, nil
	}
}

func GetSensoNetConn() (*Connection, error) {
	if sensoNetConnInitialised {
		return sensoNetConn, nil
	} else {
		err := fmt.Errorf("Connection to sensonet library not initialised. ")
		return sensoNetConn, err
	}
}

func (d *Connection) Phases() int {
	return d.phases
}

func (d *Connection) currentQuickmode() string {
	return d.sensonetCtrl.GetCurrentQuickMode()
}

// CurrentTemp is called bei Soc
func (d *Connection) CurrentTemp() (float64, error) {
	state, err := d.sensonetCtrl.GetSystem(d.systemId)
	if err != nil {
		d.log.ERROR.Println("connection.CurrentTemp. Error: ", err)
		return 0, err
	}
	hotWaterOn := false
	hotWaterCurrentTemp := 0.0
	if len(state.State.Dhw) > 0 {
		// Extracting correct State.Dhw element
		dhwData := sensonetlib.GetDhwData(state, -1)
		if dhwData.Configuration.OperationModeDhw == sensonetlib.OPERATIONMODE_TIME_CONTROLLED {
			hotWaterOn = true
			hotWaterCurrentTemp = dhwData.State.CurrentDhwTemperature
		}
	}
	if len(state.State.DomesticHotWater) > 0 {
		// Extracting correct State.Dhw element
		domesticHotWaterData := sensonetlib.GetDomesticHotWaterData(state, -1)
		if domesticHotWaterData.Configuration.OperationModeDomesticHotWater == sensonetlib.OPERATIONMODE_TIME_CONTROLLED {
			hotWaterOn = true
			hotWaterCurrentTemp = domesticHotWaterData.State.CurrentDomesticHotWaterTemperature
		}
	}

	if d.currentQuickmode() == sensonetlib.QUICKMODE_HEATING || !hotWaterOn {
		zoneData := sensonetlib.GetZoneData(state, d.heatingZone)
		currentTemp := 5.0
		if zoneData.State.CurrentRoomTemperature > currentTemp {
			currentTemp = zoneData.State.CurrentRoomTemperature
		}
		if zoneData.State.CurrentRoomTemperature == 0 && 1 == 1 {
			currentTemp = 20.0
		}
		return currentTemp, nil
	} else {
		return hotWaterCurrentTemp, nil
	}
}

// TargetTemp is called bei TargetSoc
func (d *Connection) TargetTemp() (int64, error) {
	state, err := d.sensonetCtrl.GetSystem(d.systemId)
	if err != nil {
		d.log.ERROR.Println("connection.TargetTemp. Error: ", err)
		return 0, err
	}
	hotWaterOn := false
	hotWaterSetpoint := 0.0
	if len(state.State.Dhw) > 0 {
		// Extracting correct State.Dhw element
		dhwData := sensonetlib.GetDhwData(state, -1)
		if dhwData.Configuration.OperationModeDhw == sensonetlib.OPERATIONMODE_TIME_CONTROLLED {
			hotWaterOn = true
			hotWaterSetpoint = dhwData.Configuration.TappingSetpoint
		}
	}
	if len(state.State.DomesticHotWater) > 0 {
		// Extracting correct State.Dhw element
		domesticHotWaterData := sensonetlib.GetDomesticHotWaterData(state, -1)
		if domesticHotWaterData.Configuration.OperationModeDomesticHotWater == sensonetlib.OPERATIONMODE_TIME_CONTROLLED {
			hotWaterOn = true
			hotWaterSetpoint = domesticHotWaterData.Configuration.TappingSetpoint
		}
	}

	if d.currentQuickmode() == sensonetlib.QUICKMODE_HEATING || !hotWaterOn {
		zoneData := sensonetlib.GetZoneData(state, d.heatingZone)
		if zoneData != nil {
			if zoneData.State.CurrentSpecialFunction == "QUICK_VETO" {
				return int64(zoneData.State.DesiredRoomTemperatureSetpoint), nil
			} else {
				return int64(d.quickVetoSetPoint), nil
			}
		}
		return int64(d.quickVetoSetPoint), nil
	}
	if !hotWaterOn {
		zoneData := sensonetlib.GetZoneData(state, d.heatingZone)
		return int64(zoneData.State.DesiredRoomTemperatureSetpoint), nil
	} else {
		return int64(hotWaterSetpoint), nil
	}
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
	/*if time.Now().After(d.tokenExpiresAt) {
		status = api.StatusA // disconnected
	}*/
	if d.currentQuickmode() != "" {
		status = api.StatusC
	}
	return status, nil
}

func (d *Connection) ModeText() string {
	state, err := d.sensonetCtrl.GetSystem(d.systemId)
	if err != nil {
		d.log.ERROR.Println("connection.TargetTemp. Error: ", err)
		return ""
	}
	hotWaterOn := false
	if len(state.State.Dhw) > 0 {
		// Extracting correct State.Dhw element
		dhwData := sensonetlib.GetDhwData(state, -1)
		if dhwData.Configuration.OperationModeDhw == sensonetlib.OPERATIONMODE_TIME_CONTROLLED {
			hotWaterOn = true
		}
	}
	if len(state.State.DomesticHotWater) > 0 {
		// Extracting correct State.Dhw element
		domesticHotWaterData := sensonetlib.GetDomesticHotWaterData(state, -1)
		if domesticHotWaterData.Configuration.OperationModeDomesticHotWater == sensonetlib.OPERATIONMODE_TIME_CONTROLLED {
			hotWaterOn = true
		}
	}

	tempInfo := "hotwater temp. shown)"
	if d.currentQuickmode() == sensonetlib.QUICKMODE_HEATING || !hotWaterOn {
		tempInfo = "heating temp. shown)"
	}

	switch d.currentQuickmode() {
	case sensonetlib.QUICKMODE_HOTWATER:
		return " (Hotwater Boost active)"
	case sensonetlib.QUICKMODE_HEATING:
		if d.sensonetCtrl.GetQuickModeExpiresAt() != "" {
			return " (Heating Quick Veto active. Ends " + d.sensonetCtrl.GetQuickModeExpiresAt() + ")"
		}
		return " (Heating Quick Veto active)"
	case sensonetlib.QUICKMODE_NOTHING:
		if d.sensonetCtrl.GetQuickModeExpiresAt() != "" {
			return " (charger running idle. Ends " + d.sensonetCtrl.GetQuickModeExpiresAt() + "; " + tempInfo
		}
		return " (charger running idle; " + tempInfo
	}
	return " (regular mode; " + tempInfo
}
