package bambulabs_cloud_api

import (
	"fmt"
	"github.com/torbenconto/bambulabs_cloud_api/pkg/mqtt"
	"sync"
)

type PrinterPool struct {
	mu       sync.Mutex
	printers sync.Map

	mqttClient *mqtt.Client
}

func NewPrinterPool(config *mqtt.ClientConfig) *PrinterPool {
	client := mqtt.NewClient(config)
	return &PrinterPool{
		mqttClient: client,
	}
}

func (p *PrinterPool) ConnectAll() error {
	err := p.mqttClient.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", err)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	p.printers.Range(func(_, value interface{}) bool {
		printer, ok := value.(*Printer)
		if !ok {
			return false
		}

		wg.Add(1)
		go func(p *Printer) {
			defer wg.Done()
			err := p.Connect()
			if err != nil {
				errChan <- fmt.Errorf("failed to connect to printer %s: %w", p.serial, err)
			}
		}(printer)

		return true
	})

	wg.Wait()
	close(errChan)

	var result error
	for err := range errChan {
		if result == nil {
			result = err
		} else {
			result = fmt.Errorf("%v; %w", result, err)
		}
	}

	return result
}

func (p *PrinterPool) AddPrinter(config *PrinterConfig) {
	printer := NewPrinter(config)
	p.printers.Store(config.SerialNumber, printer)
}

func (p *PrinterPool) GetData() (map[string]Data, error) {
	dataMap := make(map[string]Data)
	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	p.printers.Range(func(_, value interface{}) bool {
		printer, ok := value.(*Printer)
		if !ok {
			return false
		}

		wg.Add(1)
		go func(printer *Printer) {
			defer wg.Done()
			data, err := printer.Data()
			if err != nil {
				errChan <- fmt.Errorf("failed to get data from printer %s: %w", printer.serial, err)
				return
			}
			p.mu.Lock()
			dataMap[printer.serial] = data
			p.mu.Unlock()
		}(printer)

		return true
	})

	wg.Wait()
	close(errChan)

	var result error
	for err := range errChan {
		if result == nil {
			result = err
		} else {
			result = fmt.Errorf("%v; %w", result, err)
		}
	}

	return dataMap, result
}

func (p *PrinterPool) GetPrinter(serialNumber string) *Printer {
	printer, _ := p.printers.Load(serialNumber)
	return printer.(*Printer)
}

func (p *PrinterPool) GetPrinters() []*Printer {
	var printers []*Printer
	p.printers.Range(func(_, value interface{}) bool {
		printers = append(printers, value.(*Printer))
		return true
	})
	return printers
}

func (p *PrinterPool) RemovePrinter(serialNumber string) {
	p.printers.Delete(serialNumber)
}
