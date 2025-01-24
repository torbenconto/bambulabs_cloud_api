package bambulabs_cloud_api

import "github.com/torbenconto/bambulabs_cloud_api/pkg/mqtt"

type Config struct {
	Region   Region
	Email    string
	Password string
}

type PrinterConfig struct {
	MqttClient   *mqtt.Client
	SerialNumber string
}
