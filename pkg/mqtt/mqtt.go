package mqtt

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

const (
	clientID       = "golang-bambulabs-api"
	topicTemplate  = "device/%s/report"
	commandTopic   = "device/%s/request"
	qos            = 0
	updateInterval = 10 * time.Second
)

type ClientConfig struct {
	Host       string
	Port       int
	Serials    []string // List of serial numbers
	Username   string
	AccessCode string
	Timeout    time.Duration
}

type Client struct {
	config      *ClientConfig
	client      paho.Client
	mutex       sync.Mutex
	data        map[string]Message
	lastUpdate  time.Time
	messageChan chan paho.Message
	doneChan    chan struct{}
	ticker      *time.Ticker
}

func NewClient(config *ClientConfig) *Client {
	opts := paho.NewClientOptions().
		AddBroker(fmt.Sprintf("mqtts://%s:%d", config.Host, config.Port)).
		SetClientID(clientID).
		SetUsername(config.Username).
		SetPassword(config.AccessCode).
		SetTLSConfig(&tls.Config{InsecureSkipVerify: true}).
		SetAutoReconnect(true)

	client := &Client{
		config:      config,
		data:        make(map[string]Message),
		messageChan: make(chan paho.Message, 200),
		doneChan:    make(chan struct{}),
		ticker:      time.NewTicker(updateInterval),
	}

	opts.SetOnConnectHandler(client.onConnect)
	opts.SetConnectionLostHandler(client.onConnectionLost)
	opts.SetDefaultPublishHandler(client.handleMessage)

	client.client = paho.NewClient(opts)

	return client
}

func (c *Client) Connect() error {
	token := c.client.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}
	log.Println("Connected to MQTT broker")
	go c.processMessages()
	go c.periodicUpdate()
	c.updateAllSerials()
	return nil
}

func (c *Client) Disconnect() {
	close(c.doneChan)
	c.ticker.Stop()
	c.client.Disconnect(250)
	log.Println("Disconnected from MQTT broker")
}

func (c *Client) Publish(command *Command) error {
	rawCommand, err := command.JSON()
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	topic := fmt.Sprintf(commandTopic, c.config.Serials[0])
	token := c.client.Publish(topic, qos, false, rawCommand)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish to topic %s: %w", topic, token.Error())
	}

	log.Printf("Published command to topic %s", topic)
	return nil
}

func (c *Client) Data(serial string) Message {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.data[serial]
}

func (c *Client) onConnect(client paho.Client) {
	for _, serial := range c.config.Serials {
		topic := fmt.Sprintf(topicTemplate, serial)
		token := client.Subscribe(topic, qos, nil)
		if token.Wait() && token.Error() != nil {
			log.Printf("Failed to subscribe to topic %s: %v", topic, token.Error())
			return
		}
		log.Printf("Subscribed to topic %s", topic)
	}
}

func (c *Client) onConnectionLost(client paho.Client, err error) {
	log.Printf("Connection lost: %v", err)
}

func (c *Client) handleMessage(client paho.Client, msg paho.Message) {
	select {
	case c.messageChan <- msg:
		log.Printf("Message received: %s", msg.Topic())
	default:
		<-c.messageChan
		c.messageChan <- msg
		log.Println("Message dropped: channel full")
	}
}

const workerCount = 10

func (c *Client) processMessages() {
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case msg := <-c.messageChan:
					c.processPayload(msg)
				case <-c.doneChan:
					return
				}
			}
		}()
	}
	wg.Wait()
}

func (c *Client) processPayload(msg paho.Message) {
	var received Message
	if err := json.Unmarshal(msg.Payload(), &received); err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		return
	}

	serial := extractSerialFromTopic(msg.Topic())

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if existing, exists := c.data[serial]; exists {
		mergeMessages(&existing, &received)
		c.data[serial] = existing
	} else {
		c.data[serial] = received
	}
}

func extractSerialFromTopic(topic string) string {
	re := regexp.MustCompile(`device/([^/]+)/report`)
	matches := re.FindStringSubmatch(topic)
	if len(matches) > 1 {
		return matches[1]
	}
	log.Println("Failed to extract serial from topic:", topic)
	return ""
}

// Private methods

// update triggers a data refresh by publishing a "push_all" command.
func (c *Client) update() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if time.Since(c.lastUpdate) < c.config.Timeout {
		return
	}

	c.lastUpdate = time.Now()
	command := NewCommand(Pushing).AddCommandField("pushall")
	js, _ := command.JSON()
	fmt.Println("command", js)
	if err := c.Publish(command); err != nil {
		log.Printf("Failed to publish update command: %v", err)
	}
}

func (c *Client) periodicUpdate() {
	for {
		select {
		case <-c.ticker.C:
			c.updateAllSerials()
		case <-c.doneChan:
			return
		}
	}
}

func (c *Client) updateAllSerials() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if time.Since(c.lastUpdate) < c.config.Timeout {
		return
	}

	c.lastUpdate = time.Now()
	for _, serial := range c.config.Serials {
		command := NewCommand(Pushing).AddCommandField("pushall")
		js, _ := command.JSON()
		fmt.Println("command", js)
		if err := c.PublishToSerial(command, serial); err != nil {
			log.Printf("Failed to publish update command to serial %s: %v", serial, err)
		}
	}
}

func (c *Client) PublishToSerial(command *Command, serial string) error {
	rawCommand, err := command.JSON()
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	topic := fmt.Sprintf(commandTopic, serial)
	token := c.client.Publish(topic, qos, false, rawCommand)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish to topic %s: %w", topic, token.Error())
	}

	log.Printf("Published command to topic %s", topic)
	return nil
}

// mergeMessages recursively merges the existing and new messages.
func mergeMessages(existing, new *Message) {
	// Use reflection to iterate through the fields of the "Print" struct.
	mergeStructs(&existing.Print, &new.Print)
}

// mergeStructs dynamically merges fields of two structs using reflection.
func mergeStructs(existing, new interface{}) {
	existingVal := reflect.ValueOf(existing).Elem()
	newVal := reflect.ValueOf(new).Elem()

	// Iterate over each field in the struct.
	for i := 0; i < existingVal.NumField(); i++ {
		field := existingVal.Field(i)

		// Ensure that the field is a valid field to merge.
		newField := newVal.Field(i)
		if !newField.IsValid() {
			continue
		}

		// Only merge if the field is non-zero in the new struct.
		if !newField.IsZero() {
			// If it's a struct, recursively merge it.
			if newField.Kind() == reflect.Struct {
				mergeStructs(field.Addr().Interface(), newField.Addr().Interface())
			} else {
				// Otherwise, set the field to the new value.
				field.Set(newField)
			}
		}
	}
}
