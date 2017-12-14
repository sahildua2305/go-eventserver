package config

import (
	"io/ioutil"
	"encoding/json"
	"errors"
)

// EventServerConfig represents the default configuration of the server
// TODO (sahildua2305): Switch to yaml config instead of json config.
type EventServerConfig struct {
	EventListenerPort  int `json:"eventListenerPort"`
	ClientListenerPort int `json:"clientListenerPort"`
}

func LoadEventServerConfig(filePath string) (*EventServerConfig, error) {
	byteData, err := ioutil.ReadFile(filePath)
	if err != nil {
		// handle the error
		return nil, errors.New("unable to read the config file")
	}

	var config EventServerConfig
	// Here, we unmarshal our byteData into 'config'.
	err = json.Unmarshal(byteData, &config)
	if err != nil {
		// handle the error
		return nil, errors.New("unable to parse the JSON file")
	}

	// Ensure that the configuration is valid before returning it.
	err = config.validateConfig()
	if err != nil {
		// handle the error
		return nil, err
	}

	return &config, nil
}

// Validates the EventServerConfig struct.
// Ensures that ClientListenerPort and EventListenerPort are present and
// that they aren't the same.
func (config *EventServerConfig) validateConfig() error {
	// Ensure that ClientListenerPort is present in the config
	if config.ClientListenerPort == 0 {
		return errors.New("couldn't find the required key clientListenerPort")
	}

	// Ensure that EventListenerPort is present in the config
	if config.EventListenerPort == 0 {
		return errors.New("couldn't find the required key eventListenerPort")
	}

	if config.ClientListenerPort == config.EventListenerPort {
		return errors.New("clientListenerPort and eventListenerPort can not be same")
	}

	return nil
}
