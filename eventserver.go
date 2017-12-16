// Command eventserver is a socker server which reads events from an event
// source and forwards them to the user clients when appropriate.
package main

import (
	"bufio"
	"errors"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/sahildua2305/go-eventserver/config"
	"io"
	"log"
)

var (
	currentEventSequence int
	logInfo              *log.Logger
	logErr               *log.Logger
)

// EventServer represents the server state
type EventServer struct {
	esListener net.Listener
	ucListener net.Listener
	hasStopped bool
	quit       chan struct{}
}

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

// Initialize the two log handlers for INFO and ERROR level.
func init() {
	logInfo = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	logErr = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
}

func startServer(cfg *config.EventServerConfig) (*EventServer, error) {
	quit := make(chan struct{})

	currentEventSequence = 1

	eventsChan, usersChan, err := backgroundWorkerInit(quit)
	if err != nil {
		return nil, err
	}

	es, err := net.Listen("tcp", ":"+strconv.Itoa((*cfg).EventListenerPort))
	if err != nil {
		recover()
		return nil, err
	}
	logInfo.Println("Listening for event source on port:", (*cfg).EventListenerPort)

	uc, err := net.Listen("tcp", ":"+strconv.Itoa((*cfg).ClientListenerPort))
	if err != nil {
		recover()
		return nil, err
	}
	logInfo.Println("Listening for user clients on port:", (*cfg).ClientListenerPort)

	go listenForEventSource(es, eventsChan, quit)
	go listenForUserClients(uc, usersChan, quit)

	return &EventServer{
		esListener: es,
		ucListener: uc,
		hasStopped: false,
		quit:       quit,
	}, nil
}

// Function to stop the running event server **gracefully**.
// In case, the event server has already stopped, this will throw an error.
// If server is running, it will close the quit channel, hence signalling
// to all the running go routines to quit and also close the listeners for
// the event source and the user clients.
func (e *EventServer) gracefulStop() error {
	if e == nil {
		return errors.New("invalid event server passed")
	}
	if e.hasStopped {
		return errors.New("event server has already been stopped")
	}
	close(e.quit) // close the quit channel
	e.hasStopped = true
	e.esListener.Close() // close the event source listener
	e.ucListener.Close() // close the user clients listener
	return nil
}

// backgroundWorkerInit function which initializes a new go routine to run in
// background. The go routine is mainly used to separate the two operations:
// - processing the received events
// - storing new user client connections
//
// We could have done this thing in two separate functions keeping the event
// processing part coupled with the event receiving function and keeping the
// event writing part followed by the event processing. However, that would
// be vulnerable to race conditions.
//
// We need this function to avoid the race conditions between processing
// the incoming events and the receiving new user clients.
func backgroundWorkerInit(quit chan struct{}) (chan<- Event, chan<- UserClient, error) {
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
				// Store the newly created Event channel against the userId.
				// This will eventually be used to decide which all channels
				// should an event be sent to.
				userEventChannels[newUser.userId] = userEventChan
				go waitForEvent(newUser, userEventChan, quit)

			// For handling the new incoming events from event source:
			// Whenever a new event arrives on the channel, we do two things:
			// - Add that event to the map against its sequence number.
			// - Process as many events we can process after arrival of this event.
			case ev := <-eventsChan:
				eventsMap[ev.sequence] = ev
				processEventsInOrder(eventsMap, userEventChannels, followersMap)

			case <-quit:
				return
			}
		}
	}()
	return eventsChan, usersChan, nil
}

// Wait for a given user client to receive events on it's assigned events channel.
func waitForEvent(uc UserClient, userEventChan <-chan Event, quit <-chan struct{}) {
	for {
		// Listen for either an Event on user's assigned Event channel
		// or something on the quit channel.
		select {
		case ev := <-userEventChan:
			// If some event is received at the user's assigned Event
			// channel, send it to the user client.
			writeEvent(uc, ev)
		case <-quit:
			// If there's something sent on quit channel,
			// end the routine.
			return
		}
	}
}

// Writes a given event's payload to a given user's connection.
func writeEvent(uc UserClient, ev Event) {
	_, err := uc.conn.Write([]byte(ev.payload + "\r\n"))
	if err != nil {
		logErr.Println("Unable to write to user:", uc.userId)
	}
}

// Keep processing the events as long as we can. That basically means
// that we will keep processing the events in sequence order as long
// as we have already received the events. We will stop as we find some
// sequence for which event is missing.
func processEventsInOrder(eventsMap map[int]Event, userEventChannels map[int]chan Event, followersMap map[int]map[int]bool) {
	for {
		e, ok := eventsMap[currentEventSequence]
		if !ok {
			break
		}
		delete(eventsMap, e.sequence)
		processSingleEvent(e, userEventChannels, followersMap)
		currentEventSequence++
	}
}

// Accepts connection for event source and starts listening for events
// in a go routine. The read event is then sent to the events channel
// created by backgroundWorkerInit() to process in correct order of sequence number.
func listenForEventSource(listener net.Listener, eventsChan chan<- Event, quit <-chan struct{}) {
	for {
		connChan := make(chan net.Conn, 1)
		// We have to accept the connections in a different go routine now,
		// because otherwise we won't be able to quit gracefully from this
		// routine. To be able to support quit channel functioning, we need
		// to make everything else in this go routine non-blocking.
		go acceptConnAndSendToChan(listener, connChan)

		// Now since we need to end the go routines when the server is quit,
		// we need to have this blocking listener for one of the two channels:
		// - quit: when server is about to quit, close the listener.
		// - connChan: when a new connection is accepted.
		select {
		case <-quit:
			listener.Close()
			return
		case conn := <-connChan:
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
						if err == io.EOF {
							logInfo.Println("End of messages from event source, got EOF")
							return
						}
						logErr.Println("Unable to read from event source, got error:", err)
						return
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
}

// Accepts the connections on given listener and sends the connections to
// the given channel if the connection was successful.
func acceptConnAndSendToChan(listener net.Listener, connChan chan<- net.Conn) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	connChan <- conn
}

// Reads the event message string and parses it into Event struct.
// Parse the payload and sequence first and then parse the fromUserId and
// toUserId depending on the type of the event (F|U|B|P|S).
func parseEvent(message string) (*Event, error) {
	var event Event
	var err error

	event.payload = message
	ev := strings.Split(message, "|")
	if len(ev) < 2 || len(ev) > 4 {

		return nil, errors.New("invalid event message")
	}
	event.sequence, err = strconv.Atoi(ev[0])
	if err != nil {
		return nil, err
		return nil, errors.New("couldn't get event sequence from message")
	}
	event.eventType = ev[1]
	switch event.eventType {
	case "F":
		// Follow event
		if len(ev) != 4 {
			return nil, errors.New("received follow(F) event message of wrong format")
		}
		event.fromUserId, err = strconv.Atoi(ev[2])
		if err != nil {
			return nil, errors.New("received follow(F) event message with invalid fromUserId")
		}
		event.toUserId, err = strconv.Atoi(ev[3])
		if err != nil {
			return nil, errors.New("received follow(F) event message with invalid toUserId")
		}
		return &event, nil
	case "U":
		// Unfollow event
		if len(ev) != 4 {
			return nil, errors.New("received unfollow(U) event message of wrong format")
		}
		event.fromUserId, err = strconv.Atoi(ev[2])
		if err != nil {
			return nil, errors.New("received unfollow(U) event message with invalid fromUserId")
		}
		event.toUserId, err = strconv.Atoi(ev[3])
		if err != nil {
			return nil, errors.New("received unfollow(U) event message with invalid toUserId")
		}
		return &event, nil
	case "B":
		// Broadcast message event
		if len(ev) != 2 {
			return nil, errors.New("received broadcast(B) event message of wrong format")
		}
		return &event, nil
	case "P":
		// Private message event
		if len(ev) != 4 {
			return nil, errors.New("received private(P) event message of wrong format")
		}
		event.fromUserId, err = strconv.Atoi(ev[2])
		if err != nil {
			return nil, errors.New("received private(P) event message with invalid fromUserId")
		}
		event.toUserId, err = strconv.Atoi(ev[3])
		if err != nil {
			return nil, errors.New("received private(P) event message with invalid toUserId")
		}
		return &event, nil
	case "S":
		// Status update event
		if len(ev) != 3 {
			return nil, errors.New("received status(S) event message of wrong format")
		}
		event.fromUserId, err = strconv.Atoi(ev[2])
		if err != nil {
			return nil, errors.New("received status(S) event message with invalid fromUserId")
		}
		event.toUserId = 0
		return &event, nil
	}
	return nil, errors.New("invalid event type")
}

// Accepts the parsed event and depending on the type of the event, sends event
// to the user channels associated with the user clients which should be notified
// for a particular event.
func processSingleEvent(event Event, userEventChannels map[int]chan Event, followersMap map[int]map[int]bool) {
	switch event.eventType {
	case "F":
		// Follow event
		// Get the existing followers of the user.
		f, exists := followersMap[event.toUserId]
		if !exists {
			f = make(map[int]bool)
		}
		// Add a new follower to the list of followers and save it in followersMap.
		f[event.fromUserId] = true
		followersMap[event.toUserId] = f
		// Send event to the channel assigned for user event.toUserId.
		if uec, exists := userEventChannels[event.toUserId]; exists {
			uec <- event
		}
	case "U":
		// Unfollow event
		// Get all the followers of the user and delete entry for toUserId.
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
	}
}

// Accepts connection for new user clients and starts listening for their
// first message in a go routine. The read message contains the userId of
// the user a client represents. After a userId is read, the UserClient
// is sent to the users channel which was created using backgroundWorkerInit().
func listenForUserClients(listener net.Listener, usersChan chan<- UserClient, quit <-chan struct{}) {
	for {
		connChan := make(chan net.Conn, 1)
		// We have to accept the connections in a different go routine now,
		// because otherwise we won't be able to quit gracefully from this
		// routine. To be able to support quit channel functioning, we need
		// to make everything else in this go routine non-blocking.
		go acceptConnAndSendToChan(listener, connChan)

		select {
		case <-quit:
			listener.Close()
			return
		case conn := <-connChan:
			// Once a user client has connected, we go into a go routine to
			// read the message from the client which will contain the userId
			// associated with the client.
			// We also need to handle the case when we don't receive any message
			// from the connected client.
			go func() {
				userId, err := readAndParseUserId(conn)
				if err != nil {
					logErr.Println("Unable to receive user id from the client, got error:", err)
					return
				}
				// Send this user client to usersChan which we created in the
				// background worker.
				usersChan <- UserClient{
					userId: *userId,
					conn:   conn,
				}
			}()
		}
	}
}

// Reads the message from user client connection and parses the message
// to fetch the userId that connection represents.
// Returns the parsed userId if successful, otherwise error.
func readAndParseUserId(conn net.Conn) (*int, error) {
	m, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return nil, err
	}
	m = strings.Trim(m, "\n")
	m = strings.Trim(m, "\r")
	userId, err := strconv.Atoi(m)
	if err != nil {
		return nil, err
	}
	return &userId, nil
}

func main() {
	// Read server configuration from local config.json
	cfg, err := config.LoadEventServerConfig("./config/config.json")
	if err != nil {
		logErr.Println("Unable to load server config, got error:", err)
		os.Exit(1)
	}
	logInfo.Println("Loaded the event server config")

	es, err := startServer(cfg)
	if err != nil {
		logErr.Println("Unable to start the server, got error:", err)
		os.Exit(1)
	}
	logInfo.Println("Started the event server")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)

	// Wait for a signal on this channel.
	<-signalChan
	// Once a signal is received on signalChan, stop the server gracefully.
	logInfo.Println("Stopping the event server gracefully")
	err = es.gracefulStop()
	if err != nil {
		logErr.Println("Unable to stop the server gracefully, got error:", err)
		os.Exit(1)
	}
	signal.Stop(signalChan)
	logInfo.Println("Exiting event server!")
}
