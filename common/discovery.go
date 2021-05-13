package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"time"
)

// DiscoveryClient for bilibili discovery
// http://github.com/bilibili/discovery
type DiscoveryClient struct {
	appid         string
	env           string
	discoveryHost string

	httpClient *http.Client
	closeCh    chan struct{}

	instances []DiscoveryInstance
}

// DiscoveryInstance .
type DiscoveryInstance struct {
	Region   string `json:"region"`
	Zone     string `json:"zone"`
	Env      string `json:"env"`
	AppID    string `json:"appid"`
	Hostname string `json:"hostname"`
	// "http":"","rpc":"","version":"424407","metadata":{"cluster":"","weight":"10"},
	Addrs  []string `json:"addrs"`
	Status int      `json:"status"`
	// various timestamps
}

// DiscoveryFetchResponse .
type DiscoveryFetchResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Instances []DiscoveryInstance `json:"instances"`
		// zone_instances
	} `json:"data"`
}

// NewDiscoveryClient .
func NewDiscoveryClient(appid string) *DiscoveryClient {
	env := os.Getenv("DEPLOY_ENV")
	discoveryHost := "discovery.bilibili.co"
	if env == "uat" {
		discoveryHost = "uat-" + discoveryHost
	}
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	client := &DiscoveryClient{
		appid:         appid,
		env:           env,
		discoveryHost: discoveryHost,
		httpClient:    httpClient,
		closeCh:       make(chan struct{}),
	}
	go func() {
		loop := true
		for loop {
			select {
			case <-client.closeCh:
				loop = false
			case <-time.After(30 * time.Second):
				client.fetch()
			}
		}
	}()
	return client
}

func (client *DiscoveryClient) fetch() error {
	params := url.Values{}
	params.Set("appid", client.appid)
	params.Set("env", client.env)
	//params.Set("region", "sh")
	params.Set("status", "1")

	url := "http://" + client.discoveryHost + "/discovery/fetch?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	var data DiscoveryFetchResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return fmt.Errorf("discovery: %s", err)
	}

	if data.Code != 0 {
		return fmt.Errorf("discovery: %s", data.Message)
	}

	client.instances = data.Data.Instances
	return nil
}

// Instance get a random instance from cluster
func (client *DiscoveryClient) Instance() (*DiscoveryInstance, error) {
	if client.instances == nil {
		err := client.fetch()
		if err != nil {
			return nil, err
		}
	}
	if len(client.instances) <= 0 {
		return nil, errors.New("discovery: no instances available")
	}
	return &client.instances[rand.Intn(len(client.instances))], nil
}

func (client *DiscoveryClient) Close() {
	if client.closeCh != nil {
		close(client.closeCh)
		client.closeCh = nil
	}
}
