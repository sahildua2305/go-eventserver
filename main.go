package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

var currentEventSequence int

// Event represents an event struct as received by the event source
type Event struct {
	payload    string //
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

func main() {
	userConnsMap := make(map[int]UserClient)
	followersMap := make(map[int]map[int]bool)
	eventQueue := make(map[int]Event)

	currentEventSequence = 1

	es, err := net.Listen("tcp", ":9090")
	if err != nil {
		// handle error
	}
	uc, err := net.Listen("tcp", ":9099")
	if err != nil {
		// handle error
	}
	defer es.Close()
	defer uc.Close()

	go acceptEventSourceConnections(es, userConnsMap, followersMap, eventQueue)
	go acceptUserClientConnections(uc, userConnsMap)
	time.Sleep(time.Hour)
}

func acceptEventSourceConnections(listener net.Listener, userConns map[int]UserClient, followersMap map[int]map[int]bool, eventQueue map[int]Event) {
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
			eventQueue[event.sequence] = *event
			for {
				if e, ok := eventQueue[currentEventSequence]; ok {
					delete(eventQueue, e.sequence)
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
