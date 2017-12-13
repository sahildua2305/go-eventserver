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
	quit := make(chan struct{})

	currentEventSequence = 1

	// Read server configuration from local config.json
	config, err := loadDefaultJsonConfig("./config.json")
	if err != nil {
		// handle the error
	}

	eventsChan, usersChan, err := handler(quit)
	if err != nil {
		// handle the error
	}

	es, err := net.Listen("tcp", ":" + strconv.Itoa((*config).EventListenerPort))
	if err != nil {
		// handle error
	}
	uc, err := net.Listen("tcp", ":" + strconv.Itoa((*config).ClientListenerPort))
	if err != nil {
		// handle error
	}
	defer es.Close()
	defer uc.Close()

	go acceptEventSourceConnections(es, eventsChan)
	acceptUserClientConnections(uc, usersChan)
}

func loadDefaultJsonConfig(filePath string) (*EventServerConfig, error) {
	byteData, err := ioutil.ReadFile(filePath)
	if err != nil {
		// handle the error
		return nil, errors.New("Unable to read the config file")
	}

	var config EventServerConfig
	// Here, we unmarshal our byteData which contains
	// jsonFile's contents into 'config'.
	err = json.Unmarshal(byteData, &config)
	if err != nil {
		// handle the error
		return nil, errors.New("Unable to parse the JSON file")
	}
	return &config, nil
}

// Handler function which initializes a new go routine to run in background.
// The go routine is mainly used to separate the two operations:
// - processing the received events
// - storing new user client connections
//
// We could have done this thing in two separate functions keeping the event
// processing part coupled with the event receiving function and keeping the
// event writing part followed by the event processing. However, that would
// be vulnerable to race conditions.
//
// We need this handler function to avoid the race conditions between processing
// the incoming events and the receiving new user clients.
func handler(quit chan struct{}) (chan<- Event, chan<- UserClient, error){
	// Channel to keep incoming events.
	eventsChan := make(chan Event)
	// Channel to keep the incoming user clients.
	usersChan := make(chan UserClient)
	// Map used to store the event channels assigned for each userId.
	userEventChannels := make(map[int]chan Event)
	// Map used to store the Events against their sequence number. This map
	// is basically used as a queue to make sure we keep adding the new events
	// to the queue and keep processing as and when we can.
	// We could have used an actual priority queue here to keep events in
	// sequence, however, using a map seems to be a reasonably good choice
	// because we can easily check whether we have an event for a sequence
	// number or not.
	eventsMap := make(map[int]Event)
	// Map used to store the followers for every user. Basically, we want to
	// maintain a list of followers for every user, but then it will be
	// computationally complex to delete any follower when we get 'Unfollow' event.
	// Hence, we use a map which stores a map of followers for every user.
	// A map allows us to (computationally) easily add or delete any new follower
	// as well as iterate over all followers of a user.
	followersMap := make(map[int]map[int]bool)

	// Run a goroutine in background to handle any incoming events as well as
	// incoming new user client connections.
	// This is done in a different routine to make sure we don't
	// block the event source or the user client routine. Events source
	// routine can keep reading the events from the event source and sending them
	// to the events channel to process in correct sequence. Similarly, the user
	// client routine can keep accepting the new user connections and sending
	// them to the users channel.
	go func() {
		for {
			select {
			// For handling the new connecting users:
			// Whenever a new user client connects, we will do two things:
			// - Create a new Event channel for the new user client. This
			//   channel will be used whenever we want to send any event to
			//   this user client. This ensures that we don't block the routine
			//   and can keep dispatching the events to the users in their
			//   respective channels. Once we have created an Event channel
			//   for the user, we will start a new go routine which will keep
			//   listening on that channel for any events sent to it.
			// - Keep track of the new Event channel assigned to the user.
			case newUser := <-usersChan:
				// Create a new channel for sending events for this user
				userEventChan := make(chan Event, 1)

				var ev Event
				go func() {
					for {
						// Listen for either an Event on user's assigned Event channel
						// or something on the quit channel.
						select {
						case ev = <-userEventChan:
							// If some event is received at the user's assigned Event
							// channel, send it to the user client.
							_, err := newUser.conn.Write([]byte(ev.payload+"\r\n"))
							if err != nil {
								// handle the error
								fmt.Printf("Unable to write to user %v\n", newUser.userId)
							}
						case <-quit:
							// If there's something sent on quit channel,
							// end the routine.
							return
						}
					}
				}()
				// Store the newly created Event channel against the userId.
				// This will eventually be used to decide which all channels
				// should an event be sent to.
				userEventChannels[newUser.userId] = userEventChan

			// For handling the new incoming events from event source:
			// Whenever a new event arrives on the channel, we do two things:
			// - Add that event to the map against its sequence number.
			// - Process as many events we can process after arrival of this event.
			case ev := <-eventsChan:
				eventsMap[ev.sequence] = ev
				// Keep processing the events as long as we can. That basically means
				// that we will keep processing the events in sequence order as long
				// as we have already received the events. We will stop as we find some
				// sequence for which event is missing.
				for {
					if e, ok := eventsMap[currentEventSequence]; ok {
						delete(eventsMap, e.sequence)
						processEvent(e, userEventChannels, followersMap)
						currentEventSequence++
					} else {
						break
					}
				}
			}
		}
	}()

	return eventsChan, usersChan, nil
}

func acceptEventSourceConnections(listener net.Listener, eventsChan chan<- Event) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			// handle error
		}
		defer conn.Close()

		// Once the event source has connected, start listening to the events
		// in a go routine and perform these actions:
		// - Read message sent from the event source.
		// - Parse the event message to form Event struct.
		// - Send the parsed event to eventsChan.
		go func() {
			r := bufio.NewReader(conn)
			for {
				// Read new event message from the event source
				message, err := r.ReadString('\n')
				if err != nil {
					// handle the error
					break
				}
				// Clean/trim the message.
				message = strings.Trim(message, "\n")
				message = strings.Trim(message, "\r")
				// Parse the message to form Event struct
				event, err := parseEvent(message)
				if err != nil {
					continue
				}
				// Send the parsed event to the eventsChan.
				eventsChan <- *event
			}
		}()
	}
}

// TODO (sahildua2305): add better error handling for invalid event messages.
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

func processEvent(event Event, userEventChannels map[int]chan Event, followersMap map[int]map[int]bool) {
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
		// Send event to the channel assigned for user event.toUserId.
		if uec, exists := userEventChannels[event.toUserId]; exists {
			uec <- event
		}
	case "U":
		// Unfollow event
		f, exists := followersMap[event.toUserId]
		if exists {
			delete(f, event.fromUserId)
		}
	case "B":
		// Broadcast message event
		// Send message to channels for all connected user clients.
		for _, uec := range userEventChannels {
			uec <- event
		}
	case "P":
		// Private message event
		// Send event to the channel assigned for user event.toUserId.
		if uec, exists := userEventChannels[event.toUserId]; exists {
			uec <- event
		}
	case "S":
		// Status update event
		for u := range followersMap[event.fromUserId] {
			// Send event to the channel assigned for every follower
			if uec, exists := userEventChannels[u]; exists {
				uec <- event
			}
		}
	default:
		// Invalid event type
		// handle error
	}
}

func acceptUserClientConnections(listener net.Listener, usersChan chan<- UserClient) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			// handle error
		}
		defer conn.Close()

		// Once a user client has connected, we go into a go routine to
		// read the message from the client which will contain the userId
		// associated with the client.
		// We also need to handle the case when we don't receive any message
		// from the connected client.
		go func() {
			message, err := bufio.NewReader(conn).ReadString('\n')
			if err != nil {
				// handle the error
			}
			message = strings.Trim(message, "\n")
			message = strings.Trim(message, "\r")
			userId, err := strconv.Atoi(message)
			if err != nil {
				// handle the error
			}
			uc := UserClient{
				userId: userId,
				conn:   conn,
			}
			usersChan <- uc
		}()
	}
}
