// Package config manages the config for eventserver. It reads the json
// config file, parses the contents into EventServerConfig struct and
// validates it before returning.
package config

import (
	"encoding/json"
	"errors"
	"io/ioutil"
)

// EventServerConfig represents the default configuration of the server.
// Right now, it has only two fields for the ports for event source and
// user clients.
type EventServerConfig struct {
	EventListenerPort  int `json:"eventListenerPort"`
	ClientListenerPort int `json:"clientListenerPort"`
}

// Reads the event server config from the given file path and parses the json
// into EventServerConfig struct.
func LoadEventServerConfig(filePath string) (*EventServerConfig, error) {
	byteData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config EventServerConfig
	// Here, we unmarshal our byteData into 'config' struct.
	err = json.Unmarshal(byteData, &config)
	if err != nil {
		return nil, err
	}

	// Ensure that the configuration is valid before returning it.
	err = config.validateConfig()
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// Validates the EventServerConfig struct.
func (config *EventServerConfig) validateConfig() error {
	// Ensure that the config is not nil
	if config == nil {
		return errors.New("nil config found")
	}

	// Ensure that ClientListenerPort is present in the config
	if config.ClientListenerPort == 0 {
		return errors.New("missing clientListenerPort")
	}

	// Ensure that EventListenerPort is present in the config
	if config.EventListenerPort == 0 {
		return errors.New("missing eventListenerPort")
	}

	// Ensure that EventListenerPort and ClientListenerPort are different.
	if config.ClientListenerPort == config.EventListenerPort {
		return errors.New("clientListenerPort and eventListenerPort can not be same")
	}

	return nil
}
