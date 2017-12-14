package config_test

import (
	"testing"
	"github.com/sahildua2305/go-eventserver/config"
)

func TestLoadEventServerConfig_validConfig(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_valid.json")
	if err != nil {
		t.Error("Config is found to be invalid, expected to be valid.")
	}
}

func TestLoadEventServerConfig_invalidConfig(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_invalid.json")
	if err == nil {
		t.Error("Config is found to be valid, expected to be invalid.")
	}
}

func TestLoadEventServerConfig_missingConfigFile(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/some_random_name.json")
	if err == nil {
		t.Error("Config is found to be valid, expected to be missing.")
	}
}

func TestLoadEventServerConfig_missingEventListenerPort(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_missing_sourceport.json")
	if err == nil {
		t.Error("Config with missing eventListenerPort is found to be valid, expected to be invalid.")
	}
}

func TestLoadEventServerConfig_missingClientListenerPort(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_missing_clientport.json")
	if err == nil {
		t.Error("Config with missing clientListenerPort is found to be valid, expected to be invalid.")
	}
}

func TestLoadEventServerConfig_sameClientEventListenerPort(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_same_port.json")
	if err == nil {
		t.Error("Config with same clientListenerPort and eventListenerPort is found to be valid, expected to be invalid.")
	}
}
