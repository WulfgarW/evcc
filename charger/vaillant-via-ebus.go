package charger

// LICENSE

// Copyright (c) 2024 andig & 2025 wulfgarw

// This module is NOT covered by the MIT license. All rights reserved.

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/WulfgarW/sensonetEbus"
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/util"
)

const PVUSESTRATEGY_HOTWATER_THEN_HEATING string = "hotwater_then_heating"
const PVUSESTRATEGY_HOTWATER string = "hotwater"
const PVUSESTRATEGY_HEATING string = "heating"

func init() {
	registry.AddCtx("vaillant-via-ebus", NewVaillantViaEbusFromConfig)
}

type VaillantViaEbus struct {
	*SgReady
	log         *util.Logger
	ebusdConn   *sensonetEbus.Connection
	heatingZone int
}

//go:generate decorate -f decorateVaillantViaEbus -b *VaillantViaEbus -r api.Charger -t "api.Meter,CurrentPower,func() (float64, error)" -t "api.Battery,Soc,func() (float64, error)" -t "api.SocLimiter,GetLimitSoc,func() (int64, error)"

// NewVaillantViaEbusFromConfig creates a VaillantViaEbus configurable charger from generic config
func NewVaillantViaEbusFromConfig(ctx context.Context, other map[string]interface{}) (api.Charger, error) {
	cc := struct {
		embed         `mapstructure:",squash"`
		EbusdAddress  string
		PVUseStrategy string
		HeatingZone   int
		//heatingVetoDuration      	int32
		HeatingTemperatureOffset float32
		Phases                   int
		Cache                    time.Duration
	}{
		embed: embed{
			Icon_:     "heatpump",
			Features_: []api.Feature{api.Heating, api.IntegratedDevice},
		},
		Phases: 1,
	}

	if err := util.DecodeOther(other, &cc); err != nil {
		return nil, err
	}

	log := util.NewLogger("vaillantViaEbus").Redact(cc.EbusdAddress)

	ebusdConn, err := sensonetEbus.NewConnection(cc.EbusdAddress, sensonetEbus.WithLogger(log.TRACE))
	if err != nil {
		err = fmt.Errorf("sensonetEbuslib.NewConnection(). error: %s", err)
		return nil, err
	}

	details, err := ebusdConn.CheckEbusdConfig()
	if err == nil {
		log.DEBUG.Printf("   CheckEbusdConfig() returned no error. Details: \n%s", details)
		log.DEBUG.Println("   End of details")
	} else {
		log.ERROR.Printf("   CheckEbusdConfig() returned: Details: \n%s , (Last) Error: %s \n", details, err)
	}

	res := &VaillantViaEbus{
		log:         log,
		ebusdConn:   ebusdConn,
		heatingZone: cc.HeatingZone,
	}

	set := func(mode int64) error {
		state, err := ebusdConn.GetSystem(false)
		if err != nil {
			log.ERROR.Println("Connection.CurrentTemp. Error: ", err)
			return err
		}
		z := sensonetEbus.GetZoneData(state.Zones, cc.HeatingZone)
		switch mode {
		case Normal:
			var heatingPar sensonetEbus.HeatingParStruct
			heatingPar.ZoneIndex = cc.HeatingZone
			heatingPar.VetoSetpoint = float32(z.ActualRoomTempDesired) + cc.HeatingTemperatureOffset
			heatingPar.VetoDuration = -1.0
			result, err := ebusdConn.StopStrategybased(&heatingPar)
			log.DEBUG.Println("StopStrategybased() returns ", result)
			return err
		case Boost:
			var heatingPar sensonetEbus.HeatingParStruct
			heatingPar.ZoneIndex = cc.HeatingZone
			heatingPar.VetoSetpoint = float32(z.ActualRoomTempDesired) + cc.HeatingTemperatureOffset
			heatingPar.VetoDuration = -1.0
			result, err := ebusdConn.StartStrategybased(PVUseStrategyToSensonetEbusStrategy(cc.PVUseStrategy), &heatingPar)
			log.DEBUG.Println("StartStrategybased() returns ", result)
			return err
		default:
			return api.ErrNotAvailable
		}
	}

	var get func() (int64, error)
	get = func() (int64, error) {
		system, err := ebusdConn.GetSystem(false)
		if err != nil {
			res.log.ERROR.Println("get. Error: ", err)
			return 0, err
		}
		res.log.DEBUG.Println("Status last read from ebusd at:", system.LastGetSystem)
		//d.log.DEBUG.Println("In Switch.Enabled: Zones[0].CurrentSpecialFunction=", state.State.Zones[0].CurrentSpecialFunction)

		newQuickmode := ebusdConn.GetCurrentQuickMode()
		res.log.DEBUG.Printf("In Switch.Enabled: GetCurrentQuickmode() returns \"%s\"", newQuickmode)
		if newQuickmode == "" { //|| newQuickmode == sensonetEbus.QUICKMODE_NOTHING {
			return Normal, err
		}
		return Boost, err
	}

	sgr, err := NewSgReady(ctx, &cc.embed, set, get, nil)
	if err != nil {
		return nil, err
	}

	res.SgReady = sgr

	var power func() (float64, error)
	power = func() (float64, error) {
		return res.ebusdConn.GetSystemCurrentPower()
	}

	var temp func() (float64, error)
	temp = func() (float64, error) {
		state, err := ebusdConn.GetSystem(false)
		if err != nil {
			log.ERROR.Println("temp(). Error: ", err)
			return 0, err
		}
		if ebusdConn.GetCurrentQuickMode() == sensonetEbus.QUICKMODE_HEATING {
			currentTemp := 5.0
			for _, z := range state.Zones {
				if currentTemp == 5.0 && z.RoomTemp > currentTemp {
					currentTemp = z.RoomTemp
				}
				if z.Index == cc.HeatingZone && z.RoomTemp != 0.0 {
					currentTemp = z.RoomTemp
				}
			}
			return currentTemp, nil
		}
		return state.Hotwater.HwcStorageTemp, nil
	}

	var targetTemp func() (int64, error)
	targetTemp = func() (int64, error) {
		state, err := ebusdConn.GetSystem(false)
		if err != nil {
			log.ERROR.Println("targetTemp(). Error: ", err)
			return 0, err
		}
		if ebusdConn.GetCurrentQuickMode() == sensonetEbus.QUICKMODE_HEATING {
			z := sensonetEbus.GetZoneData(state.Zones, cc.HeatingZone)
			return int64(z.QuickVetoTemp), nil
		}
		return int64(state.Hotwater.HwcTempDesired), nil
	}

	//res.Diagnose() // Uncomment this, if you want to see diagnostic info

	return decorateVaillantViaEbus(res, power, temp, targetTemp), nil
}

func (v *VaillantViaEbus) Diagnose() {
	state, err := v.ebusdConn.GetSystem(true)
	fmt.Println("Diagnosis:")
	if err != nil {
		v.log.ERROR.Println(err)
		return
	}
	b, err := json.MarshalIndent(state, "", "  ")
	fmt.Printf("%s", b)
	fmt.Println("End of Diagnosis")
}

func (v *VaillantViaEbus) ModeText() string {
	state, err := v.ebusdConn.GetSystem(false)
	if err != nil {
		v.log.ERROR.Println("connection.TargetTemp. Error: ", err)
		return ""
	}
	hotWaterOn := false
	if state.Hotwater.HwcOpMode == sensonetEbus.OPERATIONMODE_AUTO {
		hotWaterOn = true
	}

	tempInfo := "hotwater temp. shown)"
	if v.ebusdConn.GetCurrentQuickMode() == sensonetEbus.QUICKMODE_HEATING || !hotWaterOn {
		tempInfo = "heating temp. shown)"
	}

	switch v.ebusdConn.GetCurrentQuickMode() {
	case sensonetEbus.QUICKMODE_HOTWATER:
		return " (Hotwater Boost active)"
	case sensonetEbus.QUICKMODE_HEATING:
		if v.ebusdConn.GetQuickModeExpiresAt() != "" {
			return " (Heating Quick Veto active. Ends " + v.ebusdConn.GetQuickModeExpiresAt() + ")"
		}
		return " (Heating Quick Veto active)"
	case sensonetEbus.QUICKMODE_NOTHING:
		if v.ebusdConn.GetQuickModeExpiresAt() != "" {
			return " (charger running idle. Ends " + v.ebusdConn.GetQuickModeExpiresAt() + "; " + tempInfo
		}
		return " (charger running idle; " + tempInfo
	}
	return " (regular mode; " + tempInfo
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
		fmt.Printf("Warning! No mapping for PVUseStrategy %s to a strategy in sensonetEbus", strategyAsString)
		return 0
	}
}
