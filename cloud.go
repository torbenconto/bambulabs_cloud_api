package bambulabs_cloud_api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/torbenconto/bambulabs_cloud_api/pkg/mqtt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const userAgent = "BambulabsCloudAPI/1.0"

const (
	baseUrlCN = "https://api.bambulab.cn/v1"
	baseUrlUS = "https://api.bambulab.com/v1"
)

const (
	mqttHostCN = "cn.mqtt.bambulab.cn"
	mqttHostUS = "us.mqtt.bambulab.com"
)

type baseResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

type Client struct {
	region   Region
	email    string
	password string
	token    string
}

func NewClient(config *Config) *Client {
	return &Client{
		region:   config.Region,
		email:    config.Email,
		password: config.Password,
	}
}

func NewClientWithToken(region Region, token string) *Client {
	client := NewClient(&Config{
		Region: region,
	})
	client.token = token
	return client
}

func (c *Client) getBaseUrl() string {
	if c.region == China {
		return baseUrlCN
	}

	return baseUrlUS
}

func (c *Client) getMqttHost() string {
	if c.region == China {
		return mqttHostCN
	}

	return mqttHostUS
}

type loginRequest struct {
	Email    string `json:"account"`
	Password string `json:"password"`
}
type loginResponse struct {
	Token     string `json:"accessToken"`
	LoginType string `json:"loginType"`
}

func (c *Client) Login() (string, error) {
	if c.token != "" {
		return c.token, nil
	}

	url := c.getBaseUrl() + "/user-service/user/login"

	body, err := json.Marshal(loginRequest{
		Email:    c.email,
		Password: c.password,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent) // Add User-Agent header

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: %s", response.Status)
	}

	body, err = io.ReadAll(response.Body)

	var loginResp loginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return "", err
	}

	if loginResp.LoginType == "verifyCode" {
		return "", nil
	}

	c.token = loginResp.Token

	return c.token, nil
}

type submitVerificationCodeRequest struct {
	Email string `json:"account"`
	Code  string `json:"code"`
}

func (c *Client) SubmitVerificationCode(code string) (string, error) {
	if c.token != "" {
		return c.token, nil
	}

	url := c.getBaseUrl() + "/user-service/user/login"

	body, err := json.Marshal(submitVerificationCodeRequest{
		Email: c.email,
		Code:  code,
	})

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent) // Add User-Agent header

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: %s", response.Status)
	}

	body, err = io.ReadAll(response.Body)

	var loginResp loginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return "", err
	}

	c.token = loginResp.Token

	return c.token, nil
}

type userInfoResponse struct {
	UserID int `json:"uid"`
}

func (c *Client) GetUserID() (int, error) {
	if c.token == "" {
		return -1, fmt.Errorf("no token")
	}

	url := c.getBaseUrl() + "/design-user-service/my/preference"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return -1, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent) // Add User-Agent header

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("get user id failed: %s", response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return -1, err
	}

	var userInfoResp userInfoResponse
	if err := json.Unmarshal(body, &userInfoResp); err != nil {
		return -1, err
	}

	return userInfoResp.UserID, nil
}

type Device struct {
	DevID          string `json:"dev_id"`
	Name           string `json:"name"`
	Online         bool   `json:"online"`
	PrintStatus    string `json:"print_status"`
	DevModelName   string `json:"dev_model_name"`
	DevProductName string `json:"dev_product_name"`
	DevAccessCode  string `json:"dev_access_code"`
}

type getPrintersResponse struct {
	baseResponse
	Devices []Device `json:"devices"`
}

// GetPrintersAsPool returns a printer pool with all printers that are bound to the user.
// Please note that the ftp clients do not function over the cloud.
func (c *Client) GetPrintersAsPool() (*PrinterPool, error) {
	if c.token == "" {
		return &PrinterPool{}, fmt.Errorf("no token")
	}

	url := c.getBaseUrl() + "/iot-service/api/user/bind"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &PrinterPool{}, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent) // Add User-Agent header

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return &PrinterPool{}, err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return &PrinterPool{}, fmt.Errorf("get printers failed: %s", response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return &PrinterPool{}, err
	}

	var printersResp getPrintersResponse
	if err := json.Unmarshal(body, &printersResp); err != nil {
		return &PrinterPool{}, err
	}

	uid, err := c.GetUserID()
	if err != nil {
		return &PrinterPool{}, err
	}

	var serials []string
	for _, device := range printersResp.Devices {
		serials = append(serials, device.DevID)
	}

	mqttConfig := &mqtt.ClientConfig{
		Host:       c.getMqttHost(),
		Port:       8883,
		Serials:    serials,
		Username:   "u_" + strconv.Itoa(uid),
		AccessCode: c.token,
		Timeout:    10 * time.Second,
	}

	pool := NewPrinterPool(mqttConfig)
	for _, device := range printersResp.Devices {
		pool.AddPrinter(&PrinterConfig{
			MqttClient:   pool.mqttClient,
			SerialNumber: device.DevID,
		})
	}

	return pool, nil
}

type GetTasksResponse struct {
	Total int `json:"total"`
	Hits  []struct {
		ID               int    `json:"id"`
		DesignID         int    `json:"designId"`
		ModelID          int    `json:"modelId"`
		Title            string `json:"title"`
		Cover            string `json:"cover"`
		Status           int    `json:"status"`
		Weight           int    `json:"weight"`
		Length           int    `json:"length"`
		CostTime         int    `json:"costTime"`
		ProfileID        int    `json:"profileId"`
		PlateIndex       int    `json:"plateIndex"`
		PlateName        string `json:"plateName"`
		DeviceID         string `json:"deviceId"`
		DeviceModel      string `json:"deviceModel"`
		DeviceName       string `json:"deviceName"`
		AMSDetailMapping []struct {
			AMS                int    `json:"ams"`
			SourceColor        string `json:"sourceColor"`
			TargetColor        string `json:"targetColor"`
			FilamentID         int    `json:"filamentId"`
			FilamentType       string `json:"filamentType"`
			TargetFilamentType string `json:"targetFilamentType"`
			Weight             int    `json:"weight"`
			NozzleID           int    `json:"nozzleId"`
			AMSID              int    `json:"amsId"`
			SlotID             int    `json:"slotId"`
		} `json:"amsDetailMapping"`
	} `json:"hits"`
}

func (c *Client) GetTasks(serial string) (*GetTasksResponse, error) {
	if c.token == "" {
		return &GetTasksResponse{}, fmt.Errorf("no token")
	}

	url := c.getBaseUrl() + "/user-service/my/tasks?deviceId=" + serial

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &GetTasksResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return &GetTasksResponse{}, err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return &GetTasksResponse{}, fmt.Errorf("get task failed: %s", response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return &GetTasksResponse{}, err
	}

	var tasksResp GetTasksResponse
	if err := json.Unmarshal(body, &tasksResp); err != nil {
		return &GetTasksResponse{}, err
	}

	return &tasksResp, nil
}

func (c *Client) ListDevices() ([]Device, error) {
	if c.token == "" {
		return nil, fmt.Errorf("no token")
	}

	url := c.getBaseUrl() + "/iot-service/api/user/bind"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent) // Add User-Agent header

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list devices failed: %s", response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var devicesResp getPrintersResponse
	if err := json.Unmarshal(body, &devicesResp); err != nil {
		return nil, err
	}

	return devicesResp.Devices, nil
}
