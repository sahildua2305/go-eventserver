package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"encoding/json"
	"io/ioutil"
)

var currentEventSequence int

// Event represents an event struct as received by the event source
type Event struct {
	payload    string
	sequence   int
	eventType  string
	fromUserId int
	toUserId   int
}

// UserClient represents a user client that connects to the server
type UserClient struct {
	userId int
	conn   net.Conn
}

// EventServerConfig represents the default configuration of the server
type EventServerConfig struct {
	LogLevel           string `json:"logLevel"`
	EventListenerPort  int    `json:"eventListenerPort"`
	ClientListenerPort int    `json:"clientListenerPort"`
}

func main() {
	// TODO (sahildua2305): Fix this. Can we take it out of this?
	currentEventSequence = 1

	// Read server configuration from local config.json
	config := loadDefaultJsonConfig("./config.json")

	es, err := net.Listen("tcp", ":" + strconv.Itoa(config.EventListenerPort))
	if err != nil {
		// handle error
	}
	uc, err := net.Listen("tcp", ":" + strconv.Itoa(config.ClientListenerPort))
	if err != nil {
		// handle error
	}
	defer es.Close()
	defer uc.Close()

	userConnsMap := make(map[int]UserClient)
	followersMap := make(map[int]map[int]bool)
	eventsMap := make(map[int]Event)

	go acceptEventSourceConnections(es, userConnsMap, followersMap, eventsMap)
	acceptUserClientConnections(uc, userConnsMap)
}

func loadDefaultJsonConfig(filePath string) EventServerConfig {
	byteData, err := ioutil.ReadFile(filePath)
	if err != nil {
		// handle the error
	}

	var config EventServerConfig
	// Here, we unmarshal our byteData which contains
	// jsonFile's contents into 'config'.
	err = json.Unmarshal(byteData, &config)
	if err != nil {
		// handle the error
	}
	return config
}

func acceptEventSourceConnections(listener net.Listener, userConns map[int]UserClient, followersMap map[int]map[int]bool, eventsMap map[int]Event) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			// handle error
		}

		r := bufio.NewReader(conn)
		for {
			message, _ := r.ReadString('\n')
			message = strings.Trim(message, "\n")
			message = strings.Trim(message, "\r")
			event, err := parseEvent(message)
			if err != nil {
				continue
			}
			eventsMap[event.sequence] = *event
			for {
				if e, ok := eventsMap[currentEventSequence]; ok {
					delete(eventsMap, e.sequence)
					processEvent(e, userConns, followersMap)
					currentEventSequence++;
				} else {
					break
				}
			}
		}
	}
}

func parseEvent(message string) (*Event, error) {
	var event Event
	var err error

	event.payload = message
	ev := strings.Split(message, "|")
	if len(ev) < 2 || len(ev) > 4 {
		return nil, errors.New("Invalid event message")
	}
	event.sequence, err = strconv.Atoi(ev[0])
	if err != nil {
		// handle error
	}
	event.eventType = ev[1]
	switch event.eventType {
	case "F":
		// Follow event
		event.fromUserId, _ = strconv.Atoi(ev[2])
		event.toUserId, _ = strconv.Atoi(ev[3])
		return &event, nil
	case "U":
		// Unfollow event
		event.fromUserId, _ = strconv.Atoi(ev[2])
		event.toUserId, _ = strconv.Atoi(ev[3])
		return &event, nil
	case "B":
		// Broadcast message event
		event.fromUserId = 0
		event.toUserId = 0
		return &event, nil
	case "P":
		// Private message event
		event.fromUserId, _ = strconv.Atoi(ev[2])
		event.toUserId, _ = strconv.Atoi(ev[3])
		return &event, nil
	case "S":
		// Status update event
		event.fromUserId, _ = strconv.Atoi(ev[2])
		event.toUserId = 0
		return &event, nil
	default:
		// Invalid event type
		// handle the error
	}
	return nil, errors.New("Invalid event type")
}

func processEvent(event Event, userConns map[int]UserClient, followersMap map[int]map[int]bool) {
	et := event.eventType
	switch et {
	case "F":
		// Follow event
		f, exists := followersMap[event.toUserId]
		if !exists {
			f = make(map[int]bool)
		}
		f[event.fromUserId] = true
		followersMap[event.toUserId] = f
		// send message to event.toUserId
		sendMessageToUser(event.payload, event.toUserId, userConns)
	case "U":
		// Unfollow event
		f, exists := followersMap[event.toUserId]
		if exists {
			delete(f, event.fromUserId)
		}
	case "B":
		// Broadcast message event
		// send message to all connected user clients
		for u := range userConns {
			sendMessageToUser(event.payload, u, userConns)
		}
	case "P":
		// Private message event
		// send message to event.toUserId
		sendMessageToUser(event.payload, event.toUserId, userConns)
	case "S":
		// Status update event
		for u := range followersMap[event.fromUserId] {
			// send message to every follower

			sendMessageToUser(event.payload, u, userConns)
		}
	default:
		// Invalid event type
		// handle error
	}
}

func sendMessageToUser(message string, uid int, userConns map[int]UserClient) {
	uc, exists := userConns[uid]
	if !exists {
		// handle the error
		return
	}

	if uc.conn == nil {
		return
	}
	_, err := uc.conn.Write([]byte(message+"\r\n"))
	if err != nil {
		fmt.Printf("Unable to write to user %v\n", uid)
	}
}

func acceptUserClientConnections(listener net.Listener, userConns map[int]UserClient) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			// handle error
		}

		message, _ := bufio.NewReader(conn).ReadString('\n')
		message = strings.Trim(message, "\n")
		message = strings.Trim(message, "\r")
		userId, _ := strconv.Atoi(message)
		uc := UserClient{
			userId: userId,
			conn:   conn,
		}
		userConns[userId] = uc
	}
}
