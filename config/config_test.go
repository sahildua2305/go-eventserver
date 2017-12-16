package config_test

import (
	"github.com/sahildua2305/go-eventserver/config"
	"reflect"
	"testing"
)

///// Unit tests /////

// Test with a valid config file. Ensure that the config is parsed correctly.
func TestLoadEventServerConfig_validConfig(t *testing.T) {
	got, err := config.LoadEventServerConfig("./testdata/config_valid.json")
	if err != nil {
		t.Error("Config is found to be invalid, expected to be valid.")
	}

	want := &config.EventServerConfig{ClientListenerPort: 9099, EventListenerPort: 9090}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Incorrect config returned, want: %+v, got: %+v", want, got)
	}
}

// Test with invalid config file.
func TestLoadEventServerConfig_invalidConfig(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_invalid.json")
	if err == nil {
		t.Error("Config is found to be valid, expected to be invalid.")
	}
}

// Test with a missing config file.
func TestLoadEventServerConfig_missingConfigFile(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/some_random_name.json")
	if err == nil {
		t.Error("Config is found to be valid, expected to be missing.")
	}
}

// Test with a config file missing eventListenerPort.
func TestLoadEventServerConfig_missingEventListenerPort(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_missing_sourceport.json")
	if err == nil {
		t.Error("Config with missing eventListenerPort is found to be valid, expected to be invalid.")
	}
}

// Test with a config file missing clientListenerPort.
func TestLoadEventServerConfig_missingClientListenerPort(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_missing_clientport.json")
	if err == nil {
		t.Error("Config with missing clientListenerPort is found to be valid, expected to be invalid.")
	}
}

// Test with a config file having same eventListenerPort and clientListenerPort.
func TestLoadEventServerConfig_sameClientEventListenerPort(t *testing.T) {
	_, err := config.LoadEventServerConfig("./testdata/config_same_port.json")
	if err == nil {
		t.Error("Config with same clientListenerPort and eventListenerPort is found to be valid, expected to be invalid.")
	}
}

///// Benchmark tests /////

// Benchmarks the performance of LoadEventServerConfig.
func BenchmarkLoadEventServerConfig(b *testing.B) {
	for n := 0; n < b.N; n++ {
		got, err := config.LoadEventServerConfig("./testdata/config_valid.json")
		if err != nil {
			b.Error("Config is found to be invalid, expected to be valid.")
		}
		want := &config.EventServerConfig{ClientListenerPort: 9099, EventListenerPort: 9090}
		if !reflect.DeepEqual(got, want) {
			b.Errorf("Incorrect config returned, want: %+v, got: %+v", want, got)
		}
	}
}
