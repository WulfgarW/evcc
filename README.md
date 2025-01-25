# vaillant-ebus and sensonet modules as extension for evcc 

This is a fork of evcc-io/evcc to add an extension for Vaillant heat pumps controlable via ebusd or via the sensoNET (VR921) control module and a myVaillant account.

This fork of evcc should only be used if you have a Vaillant heat pump and want to use evcc for PV optimised hotwater production or heating.
All other persons should visit github.com/evcc-io/evcc. There you can also find all general documentation about evcc.

## Features
(1) vaillant-ebus
- Adds a charger module vaillant-ebus und a vehicle module vaillant-ebus_vehicle to allow PV optimised heating with a Vaillant heat pump
- The vaillant-ebus module connects to an ebus daemon (https://github.com/john30/ebusd) to get system information from the heat pump and to start/stop quick modes. So to use this evcc extension, an ebus adapter (see https://adapter.ebusd.eu/) and the ebusd (=software) are needed. 
- The current power consumption of the heat pump is read via ebus, but an external energy meter like Shelly 3EM can be used as well.
- As the vaillant-ebus charger module supports two different charge modes ('Hotwater Boost' and 'Zone Quick Veto'), small additions were made to the
  module loadpoint.go and the vue file Vehicle.vue to display the current charge mode. The usage of these two changes is optional. 

(2) sensonet
- Adds a charger module sensonet_und a vehicle module sensonet_vehicle to allow PV optimised heating with a Vaillant heat pump
- The sensonet module connects to the myVaillant portal to get system information from the heat pump and to start/stop quick modes.
- As the sensonet charger module supports two different charge modes ('Hotwater Boost' and 'Zone Quick Veto'), small additions were made to the
  module loadpoint.go and the vue file Vehicle.vue to display the current charge mode. The usage of these two changes is optional. 

## How it works
(1) vaillant-ebus
To use this extension a charger section for 'vaillant-ebus' has to be added to evcc.yaml. You can use vehicles of type vaillant-ebus_vehicle to switch between different PV use strategies without stopping evcc. Or you set the parameter 'integrateddevice: true' for the vaillant-ebus charger and define no vehicles.

The vaillant-ebus charge module initiates a telnet connection to the ebusd which can reside on the same machine or on another machine in the local network. Via 'read' commands, information about the heat pump state is gathered and some of it like SoC (=current temperature), TargetSoC (=temperature setpoint) and current power consumption presented by the evcc web front end.
As evcc is able to read the current power consumption of the heat pump via ebusd, no external metering device like Shelly 3EM is necessary. (But if an energy meter like Shelly 3EM is present for the Vaillant heatpump and shall be used, then a meter section has to be added to evcc.yaml as well.) 
The vaillant-ebus charger, the vaillant-ebus_vehicle vehicles and the energy meter have to be combinated as a loadpoint in the yaml file.

If the evcc loadpoint module signals to the vaillant-ebus charger module that a charging session should be started, then the vaillant-ebus module does the following:
   (a) If ('pvusestrategy='hotwater') or (pvusestrategy='hotwater_than_heating' and current hotwater temperature is more than 5°C below the setpoint), then a hotwater boost is initiated via a 'write' request to the ebusd.
   (b) If ('pvusestrategy='heating') or (pvusestrategy='hotwater_than_heating' and current hotwater temperature is less than 5°C below the setpoint), then a heating zone quick veto is initiated via a 'write' request to the ebusd for the zone given by the heatingzone parameter in the evcc.yaml file (default=0). The duration of the veto is 30 minutes and the veto_setpoint= normal_temperature_setpoint + heatingtemperatureoffset (default=2).
Via the regular 'read' commands, the vaillant-ebus module notices when the hotwater boost or the zone quick veto ends and changes the enabled() status to false to report this to the loadpoint module of evcc.
If the loadpoint module tells the vaillant-ebus module to stop a charging session, then the vaillant-ebus module ends the hotwater boost or the zone quick veto via a 'write' request to the ebusd.

(2) sensonet
To use this extension a charger section for 'sensonet' has to be added to evcc.yaml. You can use vehicles of type sensonet_vehicle to switch between different PV use strategies. Or you set the parameter 'integrateddevice: true' for the sensonet charger and define no vehicles.
(If an energy meter like Shelly 3EM is present for the Vaillant heatpump, then a meter section has to be added to evcc.yaml as well.)
The sensonet charger, the sensonet_vehicle vehicles and the energy meter have to be combinated as a loadpoint in the yaml file.

The sensonet charge module initiates a http API interface to the myVaillant portal and keeps this connection active by regular refresh token calls. A json system report is obtained about every two minutes using a http GET call. This report is analysed to extract information about heating zones, operating modes, current temperatures and temperature setpoints.
Some of the gathered data like SoC (=current temperature), TargetSoC (=temperature setpoint) and current power consumption are presented by the evcc web front end. (Some heat pump systems can not provide the current power consumption to the myVaillant portal and therefore the sensonet charger can not read it from there. Then the use of an external metering device is recommended but not mandatory.)

If the evcc loadpoint module signals to the sensonet charger module that a charging session should be started, then the sensonet module does the following:
   (a) If ('pvusestrategy='hotwater') or (pvusestrategy='hotwater_than_heating' and current hotwater temperature is more than 5°C below the setpoint), then a hotwater boost is initiated via http POST request.
   (b) If ('pvusestrategy='heating') or (pvusestrategy='hotwater_than_heating' and current hotwater temperature is less than 5°C below the setpoint), then a heating zone quick veto is initiated via http POST request for the zone given by the heatingzone parameter in the evcc.yaml file (default=0). The duration of the veto is 3 hours and the veto_setpoint= normal_temperature_setpoint + heatingtemperatureoffset (default=2).
Via the json system reports, the sensonet module notices when the hotwater boost or the zone quick veto ends and changes the enabled() status to false to report this to the loadpoint module of evcc.
If the loadpoint module tells the sensonet module to stop a charging session, then the sensonet module sends a "cancel hotwater boost" or "cancel zone quick veto" to the myVaillant portal via http DELETE request.
It happens, that it takes a few minutes before the json system report from the myVaillant portal reflects an initiated or canceled boost or quick veto. Don't worry!

## Custom ebus message definition file 15.ctlv2.csv
At the moment, some ebus message definitions needed for the initiation of a zone quick veto are missing in the "official" ebus configuration files under https://ebus.github.io/. 
It works, if you start the ebusd service with the config path https://ebus.github.io/next/. Or you can download the config files to a local path and substitute 15.ctlv2.csv by the file https://github.com/WulfgarW/sensonetEbus/ebusd-config-files/15.ctlv2.csv 

## Request for feedback
Feedback of beta testers is welcome.

## Further notes
The vaillant-ebus charger is based on the sensonetEbus package at https://github.com/WulfgarW/sensonetEbus.
The sensonet charger is based on the sensonet package at https://github.com/WulfgarW/sensonet