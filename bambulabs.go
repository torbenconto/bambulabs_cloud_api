package bambulabs_cloud_api

import (
	"fmt"
	"github.com/torbenconto/bambulabs_cloud_api/pkg/mqtt"
	"github.com/torbenconto/bambulabs_cloud_api/state"
	"image/color"
	"strconv"
)

type Printer struct {
	mqttClient *mqtt.Client
	serial     string
}

func NewPrinter(config *PrinterConfig) *Printer {
	return &Printer{
		mqttClient: config.MqttClient,
		serial:     config.SerialNumber,
	}
}

func (p *Printer) Connect() error {
	return nil
}

func (p *Printer) Disconnect() {
	p.mqttClient.Disconnect()
}

func unsafeParseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func unsafeParseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func (p *Printer) Data() (Data, error) {
	data := p.mqttClient.Data(p.serial)

	final := Data{
		Ams:                     make([]Ams, 0),
		AmsExists:               data.Print.Ams.AmsExistBits == "1",
		BedTargetTemperature:    data.Print.BedTargetTemper,
		BedTemperature:          data.Print.BedTemper,
		AuxiliaryFanSpeed:       unsafeParseInt(data.Print.BigFan1Speed),
		ChamberFanSpeed:         unsafeParseInt(data.Print.BigFan2Speed),
		PartFanSpeed:            unsafeParseInt(data.Print.CoolingFanSpeed),
		HeatbreakFanSpeed:       unsafeParseInt(data.Print.HeatbreakFanSpeed),
		ChamberTemperature:      data.Print.ChamberTemper,
		GcodeFile:               data.Print.GcodeFile,
		GcodeFilePreparePercent: unsafeParseInt(data.Print.GcodeFilePreparePercent),
		GcodeState:              state.GcodeState(data.Print.GcodeState),
		PrintPercentDone:        data.Print.McPercent,
		PrintErrorCode:          data.Print.McPrintErrorCode,
		RemainingPrintTime:      data.Print.McRemainingTime,
		SubtaskName:             data.Print.SubtaskName,
		TotalLayerNumber:        data.Print.TotalLayerNum,
		NozzleDiameter:          data.Print.NozzleDiameter,
		NozzleTargetTemperature: data.Print.NozzleTargetTemper,
		NozzleTemperature:       data.Print.NozzleTemper,
		Sdcard:                  data.Print.Sdcard,
		WifiSignal:              data.Print.WifiSignal,
	}

	colors := make([]color.RGBA, 0)
	for _, col := range data.Print.VtTray.Cols {
		if col == "" {
			colors = append(colors, color.RGBA{})
		} else {
			c, err := parseHexColorFast(col)
			if err != nil {
				return Data{}, fmt.Errorf("parseHexColorFast() error %w", err)
			}
			colors = append(colors, c)
		}
	}

	var trayColor = color.RGBA{}
	if data.Print.VtTray.TrayColor != "" {
		var err error
		trayColor, err = parseHexColorFast(data.Print.VtTray.TrayColor)
		if err != nil {
			return Data{}, fmt.Errorf("parseHexColorFast() error %w", err)
		}
	}

	final.VtTray = Tray{
		ID:                unsafeParseInt(data.Print.VtTray.ID),
		BedTemperature:    unsafeParseFloat(data.Print.VtTray.BedTemp),
		Colors:            colors,
		DryingTemperature: unsafeParseFloat(data.Print.VtTray.DryingTemp),
		DryingTime:        unsafeParseInt(data.Print.VtTray.DryingTime),
		NozzleTempMax:     unsafeParseFloat(data.Print.VtTray.NozzleTempMax),
		NozzleTempMin:     unsafeParseFloat(data.Print.VtTray.NozzleTempMin),
		TrayColor:         trayColor,
		TrayDiameter:      unsafeParseFloat(data.Print.VtTray.TrayDiameter),
		TraySubBrands:     data.Print.VtTray.TraySubBrands,
		TrayType:          data.Print.VtTray.TrayType,
		TrayWeight:        unsafeParseInt(data.Print.VtTray.TrayWeight),
	}

	for _, ams := range data.Print.Ams.Ams {
		trays := make([]Tray, 0)

		for _, tray := range ams.Tray {
			colors := make([]color.RGBA, 0)

			for _, col := range tray.Cols {
				if col == "" {
					colors = append(colors, color.RGBA{})
				}
				c, err := parseHexColorFast(col)
				if err != nil {
					return Data{}, fmt.Errorf("parseHexColorFast() error %w", err)
				}
				colors = append(colors, c)
			}

			var trayColor = color.RGBA{}
			if tray.TrayColor != "" {
				var err error
				trayColor, err = parseHexColorFast(tray.TrayColor)
				if err != nil {
					return Data{}, fmt.Errorf("parseHexColorFast() error %w", err)
				}
			}

			trays = append(trays, Tray{
				ID:                unsafeParseInt(tray.ID),
				BedTemperature:    unsafeParseFloat(tray.BedTemp),
				Colors:            colors,
				DryingTemperature: unsafeParseFloat(tray.DryingTemp),
				DryingTime:        unsafeParseInt(tray.DryingTime),
				NozzleTempMax:     unsafeParseFloat(tray.NozzleTempMax),
				NozzleTempMin:     unsafeParseFloat(tray.NozzleTempMin),
				TrayColor:         trayColor,
				TrayDiameter:      unsafeParseFloat(tray.TrayDiameter),
				TraySubBrands:     tray.TraySubBrands,
				TrayType:          tray.TrayType,
				TrayWeight:        unsafeParseInt(tray.TrayWeight),
			})
		}

		final.Ams = append(final.Ams, Ams{
			Humidity:    unsafeParseInt(ams.Humidity),
			ID:          unsafeParseInt(ams.ID),
			Temperature: unsafeParseFloat(ams.Temp),
			Trays:       trays,
		})
	}

	return final, nil
}
