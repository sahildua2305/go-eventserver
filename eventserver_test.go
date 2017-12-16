package main

import (
	"io"
	"net"
	"strconv"
	"testing"

	"github.com/sahildua2305/go-eventserver/config"
	"os/exec"
	"strings"
)

///// Unit tests /////

// Tests parseEvent function extensively for various event messages that
// the server might need to handle. Contains a good number of valid and
// invalid event messages. Feel free to add more as you come across more
// edge cases to make sure this function is robust.
func TestParseEvent(t *testing.T) {
	want := map[string]bool{
		// Random bad message cases
		"":              false,
		"123224":        false,
		"abcdkjdhfjsdf": false,
		"|":             false,
		"12|||":         false,
		"|||":           false,
		"1|L|2|3":       false,

		// Follow message cases
		"12|F|21212|31212":                   true,
		"1234567890|F|1234567890|1234567891": true,
		"12|F":            false,
		"12|F|12":         false,
		"|F|12|13":        false,
		"1|F|12|12|12|12": false,
		"1|F||2":          false,
		"1|F|2|":          false,

		// Unfollow message cases
		"2|U|4|5":   true,
		"2|U|4":     false,
		"2|U|4|":    false,
		"2|U||4":    false,
		"|U|4|5":    false,
		"|U||":      false,
		"1|U|2|3|4": false,

		// Broadcast message cases
		"1|B":          true,
		"1234567890|B": true,
		"|B":           false,
		"1|B|2":        false,
		"1|B|2|3":      false,

		// Private message cases
		"1|P|12|11": true,
		"1|P||":     false,
		"|P||":      false,
		"1|P|2|3|":  false,
		"|P":        false,
		"1|P":       false,
		"1|P|2|":    false,

		// Status message cases
		"12|S|23":    true,
		"12|S|23|23": false,
		"12|S|":      false,
	}

	for msg := range want {
		_, err := parseEvent(msg)
		if err != nil && want[msg] {
			t.Errorf("parseEvent with event - %v: want ok, got %+v", msg, err)
		}
		if err == nil && !want[msg] {
			t.Errorf("parseEvent with event - %v: want not ok, got ok", msg)
		}
	}
}

///// Functional tests /////

// Tests whether the server can be started and gracefully stopped
// without any errors.
func TestEventServer_startAndStop(t *testing.T) {
	cfg, err := config.LoadEventServerConfig("./config/testdata/config_valid.json")
	if err != nil {
		t.Error("Couldn't load server config, got error: ", err)
	}
	es, err := startServer(cfg)
	if err != nil {
		t.Error("Server couldn't be started, got error: ", err)
	}
	err = es.gracefulStop()
	if err != nil {
		t.Error("Server couldn't be stopped gracefully, got error: ", err)
	}
	if !es.hasStopped {
		t.Error("Server is still running, expected to be stopped")
	}
	if err = es.gracefulStop(); err == nil {
		t.Error("gracefulStop() ran successfully, expected it to throw error")
	}
}

// Tests the event server with the given jar test program for 1000 events.
func TestEventServer_runWithJarHarness(t *testing.T) {
	cfg := &config.EventServerConfig{EventListenerPort: 8080, ClientListenerPort: 8088}
	es, err := startServer(cfg)
	if err != nil {
		t.Fatal("Server couldn't be started, got error: ", err)
	}
	c := exec.Command("time", "java", "-server", "-Xmx1G", "-jar", "./follower-maze-2.0.jar")
	c.Env = []string{"totalEvents=1000", "eventListenerPort=8080", "clientListenerPort=8088"}
	out, err := c.CombinedOutput()
	if err != nil {
		t.Error("Got error: ", err)
	}
	if !strings.Contains(string(out), "ALL NOTIFICATIONS RECEIVED") {
		t.Error("Test failed with jar harness, got incorrect output")
	}
	err = es.gracefulStop()
	if err != nil {
		t.Error("Server couldn't be stopped gracefully, got error: ", err)
	}
}

// Tests the behaviour of server with some raw messages including
// some bad ones.
func TestEventServer_withRawEventMessages(t *testing.T) {
	cfg := &config.EventServerConfig{EventListenerPort: 6060, ClientListenerPort: 6066}
	es, err := startServer(cfg)
	if err != nil {
		t.Fatal("Server couldn't be started, got error:", err)
	}

	conn, err := net.Dial("tcp", "localhost:"+strconv.Itoa(cfg.EventListenerPort))
	if err != nil {
		t.Error("Couldn't open port for event source, got error: ", err)
	}

	eventMsgs := []string{
		"123224",
		"",
		"abcdkjdhfjsdf",
		"2|U|4|5",
		"12|S|23",
		"1|U|2|3|4",
	}
	for _, msg := range eventMsgs {
		_, err = io.WriteString(conn, msg)
		if err != nil {
			t.Errorf("Couldn't send event: %+v, got error: %+v", msg, err)
		}
	}
	conn.Close()

	if es.hasStopped {
		t.Error("Server has stopped, expected to be running")
	}
	err = es.gracefulStop()
	if err != nil {
		t.Error("Server couldn't be stopped gracefully, got error: ", err)
	}
}

// Tests the behaviour when one of the ports is already open and hence,
// the server can't be started.
func TestEventServer_alreadyBusyPort(t *testing.T) {
	cfg := &config.EventServerConfig{EventListenerPort: 5050, ClientListenerPort: 5055}

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.EventListenerPort))
	if err != nil {
		t.Fatal("Couldn't open port for event source, got error: ", err)
	}
	es, err := startServer(cfg)
	if err == nil {
		t.Error("Server started without error, expected error")
	}
	listener.Close()

	listener, err = net.Listen("tcp", ":"+strconv.Itoa(cfg.ClientListenerPort))
	if err != nil {
		t.Error("Couldn't open port for user clients, got error: ", err)
	}
	es, err = startServer(cfg)
	if err == nil {
		t.Error("Server started without error, expected error")
	}
	listener.Close()

	err = es.gracefulStop()
	if err == nil {
		t.Error("gracefulStop() ran successfully, expected it to throw error")
	}
}

///// Benchmark tests /////

func benchmarkEventServer(args []string, b *testing.B) {
	// run server with events b.N times
	for n := 0; n < b.N; n++ {
		cfg := &config.EventServerConfig{EventListenerPort: 7070, ClientListenerPort: 7077}
		es, err := startServer(cfg)
		if err != nil {
			b.Fatal("Server couldn't be started, got error:", err)
		}
		c := exec.Command("time", "java", "-server", "-Xmx1G", "-jar", "./follower-maze-2.0.jar")
		c.Env = []string{"eventListenerPort=7070", "clientListenerPort=7077"}
		c.Env = append(c.Env, args...)
		out, err := c.CombinedOutput()
		if err != nil {
			b.Error("Got error: ", err)
		}
		if !strings.Contains(string(out), "ALL NOTIFICATIONS RECEIVED") {
			b.Error("Test failed with jar harness, got incorrect output")
		}
		err = es.gracefulStop()
		if err != nil {
			b.Error("Server couldn't be stopped gracefully, got error: ", err)
		}
	}
}

func BenchmarkEventServer_1000Events1Batch10ConnectedUsers(b *testing.B) {
	benchmarkEventServer([]string{"totalEvents=1000", "maxEventSourceBatchSize=1", "concurrencyLevel=10"}, b)
}

func BenchmarkEventServer_1000Events10Batch10ConnectedUsers(b *testing.B) {
	benchmarkEventServer([]string{"totalEvents=1000", "maxEventSourceBatchSize=10", "concurrencyLevel=10"}, b)
}

func BenchmarkEventServer_1000Events100Batch10ConnectedUsers(b *testing.B) {
	benchmarkEventServer([]string{"totalEvents=1000", "maxEventSourceBatchSize=100", "concurrencyLevel=10"}, b)
}

func BenchmarkEventServer_10000Events100Batch100ConnectedUsers(b *testing.B) {
	benchmarkEventServer([]string{"totalEvents=10000", "maxEventSourceBatchSize=100", "concurrencyLevel=100"}, b)
}

func BenchmarkEventServer_10000Events1000Batch100ConnectedUsers(b *testing.B) {
	benchmarkEventServer([]string{"totalEvents=10000", "maxEventSourceBatchSize=1000", "concurrencyLevel=100"}, b)
}

func BenchmarkEventServer_10000Events10000Batch100ConnectedUsers(b *testing.B) {
	benchmarkEventServer([]string{"totalEvents=10000", "maxEventSourceBatchSize=10000", "concurrencyLevel=100"}, b)
}

func BenchmarkEventServer_100000Events10000Batch1000ConnectedUsers(b *testing.B) {
	benchmarkEventServer([]string{"totalEvents=100000", "maxEventSourceBatchSize=10000", "concurrencyLevel=1000"}, b)
}
