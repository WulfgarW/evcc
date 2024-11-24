package sensonet

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	"github.com/ernesto-jimenez/httplogger"
	"github.com/spf13/jwalterweatherman"

	sensonetlib "github.com/WulfgarW/sensonet"
	"github.com/evcc-io/evcc/api"

	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/request"
)

// Connection is the Sensonet connection
type Connection struct {
	*request.Helper
	log           *util.Logger
	user          string
	password      string
	realm         string
	sensonetConn  *sensonetlib.Connection
	systemId      string
	pvUseStrategy string
	heatingZone   int
	phases        int
	//	heatingVetoDuration      int32
	heatingTemperatureOffset float64
	onoff                    bool
	quickVetoSetPoint        float32
	quickVetoExpiresAt       string
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
		utillog := util.NewLogger("sensonet")
		client := request.NewHelper(utillog)
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		conn := &Connection{
			Helper: client,
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
		conn.Client.Jar, err = cookiejar.New(nil)
		if err != nil {
			err = fmt.Errorf("could not reset cookie jar. error: %s", err)
			return conn, err
		}

		var credentials sensonetlib.CredentialsStruct
		credentials.User = user
		credentials.Password = password
		//Activate httplogger and logging in sensonetlib for log levels TRACE or DEBUG
		if util.WWlogLevelForArea("sensonet") == jwalterweatherman.LevelTrace || util.WWlogLevelForArea("sensonet") == jwalterweatherman.LevelDebug {
			log.SetOutput(os.Stderr) //changing output of stadard log to os.stderr. (In main.go, it is set to io.Discard)
			log := log.New(os.Stderr, "sensonet: ", log.Lshortfile)
			client.Transport = httplogger.NewLoggedTransport(http.DefaultTransport, newLogger(log))
		}
		//client.Transport = http.DefaultTransport //comment this line out, if you wish logging of the http requests in sensonetlib
		snconn, newtoken, err := sensonetlib.NewConnection(client.Client, &credentials, nil)
		if err != nil {
			err = fmt.Errorf("could not get Homes[] information. error: %s", err)
			return conn, err
		}
		utillog.DEBUG.Println("In connection.NewConnection: Call of sensonetlib.NewConnection() successful")
		utillog.DEBUG.Println("Got new Token. Vaild until: ", newtoken.Expiry)
		conn.sensonetConn = snconn
		homes, err := conn.sensonetConn.GetHomes()
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

func (d *Connection) CurrentQuickmode() string {
	return d.sensonetConn.GetCurrentQuickMode()
}

func (d *Connection) QuickVetoExpiresAt() string {
	return d.quickVetoExpiresAt
}

// CurrentTemp is called bei Soc
func (d *Connection) CurrentTemp() (float64, error) {
	state, err := d.sensonetConn.GetSystem(d.systemId)
	if err != nil {
		d.log.ERROR.Println("connection.CurrentTemp. Error: ", err)
		return 0, err
	}
	if d.CurrentQuickmode() == sensonetlib.QUICKMODE_HEATING {
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
		dhwData := sensonetlib.GetDhwData(state, -1)
		return float64(dhwData.State.CurrentDhwTemperature), nil
	}
}

// TargetTemp is called bei TargetSoc
func (d *Connection) TargetTemp() (float64, error) {
	state, err := d.sensonetConn.GetSystem(d.systemId)
	if err != nil {
		d.log.ERROR.Println("connection.TargetTemp. Error: ", err)
		return 0, err
	}
	if d.CurrentQuickmode() == sensonetlib.QUICKMODE_HEATING {
		zoneData := sensonetlib.GetZoneData(state, d.heatingZone)
		if zoneData != nil {
			if zoneData.State.CurrentSpecialFunction == "QUICK_VETO" {
				return float64(zoneData.State.DesiredRoomTemperatureSetpoint), nil
			} else {
				return float64(d.quickVetoSetPoint), nil
			}
		}
		return float64(d.quickVetoSetPoint), nil
	} else {
		dhwData := sensonetlib.GetDhwData(state, -1)
		return float64(dhwData.Configuration.TappingSetpoint), nil
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
	if d.CurrentQuickmode() != "" {
		status = api.StatusC
	}
	return status, nil
}
