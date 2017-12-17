// Command eventserver is a socket server which reads events from an event
// source and forwards them to the user clients when appropriate.
package main

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/sahildua2305/go-eventserver/config"
)

var (
	currentEventSequence int
	logInfo              *log.Logger
	logErr               *log.Logger
)

// eventServer represents the server state.
type eventServer struct {
	// Listener object listening for event source on EventListenerPort.
	esListener net.Listener

	// Listener object listening for user clients on ClientListenerPort.
	ucListener net.Listener

	// Boolean to keep track of the running state of the server.
	hasStopped bool

	// Channel to support communication with go routines while stopping
	// the server gracefully.
	quit chan struct{}
}

// event represents an event struct as received by the event source.
type event struct {
	payload    string // Raw event message which will be sent to user clients
	sequence   int    // Sequence number of the event
	eventType  string // Type of the event among valid types - F, U, B, P, S
	fromUserId int    // From User Id, meaning depends on the event type
	toUserId   int    // To User Id, meaning depends on the event type
}

// userClient represents a user client that connects to the server.
type userClient struct {
	userId int      // User Id that the user client represents
	conn   net.Conn // Connection object for the user client
}

// Initialize the two log handlers for INFO and ERROR level.
func init() {
	logInfo = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	logErr = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
}

// Function that acts as the starting of the server.
// Returns *eventServer pointer reference with all fields set.
func startServer(cfg *config.EventServerConfig) (*eventServer, error) {
	quit := make(chan struct{})

	// This can probably be taken out of here on config level.
	currentEventSequence = 1

	// Creates a background worker for handling events processing and new
	// user client connections.
	eventsChan, usersChan, err := backgroundWorkerInit(quit)
	if err != nil {
		return nil, err
	}

	// Start listening on EventListenerPort.
	es, err := net.Listen("tcp", ":"+strconv.Itoa((*cfg).EventListenerPort))
	if err != nil {
		recover()
		return nil, err
	}
	logInfo.Println("Listening for event source on port:", (*cfg).EventListenerPort)

	// Start listening on ClientListenerPort.
	uc, err := net.Listen("tcp", ":"+strconv.Itoa((*cfg).ClientListenerPort))
	if err != nil {
		recover()
		return nil, err
	}
	logInfo.Println("Listening for user clients on port:", (*cfg).ClientListenerPort)

	// Go routine to handle event source connections.
	go listenForEventSource(es, eventsChan, quit)
	// Go routine to handle user client connections.
	go listenForUserClients(uc, usersChan, quit)

	return &eventServer{
		esListener: es,
		ucListener: uc,
		hasStopped: false,
		quit:       quit,
	}, nil
}

// Function to stop the running event server **gracefully**.
// In case, the event server has already stopped, this will throw an error.
// If server is running, it will close the quit channel, hence signalling
// to all the running go routines to return and also close the listeners for
// the event source and the user clients.
func (e *eventServer) gracefulStop() error {
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
// user handling part coupled with the function receiving user connections.
// However, that would be vulnerable to race conditions.
//
// We need this function to avoid the race conditions between processing
// the incoming events and the storing new user clients.
func backgroundWorkerInit(quit chan struct{}) (chan<- event, chan<- userClient, error) {
	// Channel to keep the incoming events.
	eventsChan := make(chan event)

	// Channel to keep the incoming user clients.
	usersChan := make(chan userClient)

	// Map used to store the event channels assigned for each userId.
	userEventChannels := make(map[int]chan event)

	// Map used to store the events against their sequence number. This map
	// is basically used as a queue to make sure we keep adding the new events
	// to the queue at proper indices and keep processing as and when we can.
	// We could have used an actual priority queue here to keep events in
	// sequence, however, using a map seems to be a reasonably good choice
	// because we can easily check whether we have an event for a sequence
	// number or not.
	eventsMap := make(map[int]event)

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
			// For handling the new connecting users.
			case newUser := <-usersChan:
				// Create a new channel for sending events to this user.
				// This channel will be used whenever we want to send any
				// event to this user client.
				userEventChan := make(chan event, 1)

				// Store the newly created event channel against the userId.
				// This will eventually be used to decide which all channels
				// should an event be sent to.
				userEventChannels[newUser.userId] = userEventChan

				// Wait for incoming events on this user channel.
				go waitForEvent(newUser, userEventChan, quit)

			// For handling the new incoming events from event source.
			case ev := <-eventsChan:
				// Add that event to the map against its sequence number.
				eventsMap[ev.sequence] = ev

				// Process as many events we can process after arrival of this event.
				processEventsInOrder(eventsMap, userEventChannels, followersMap)

			// For returning from the go routine.
			case <-quit:
				return
			}
		}
	}()

	return eventsChan, usersChan, nil
}

// Wait for a given user client to receive events on its assigned events channel.
func waitForEvent(uc userClient, userEventChan <-chan event, quit <-chan struct{}) {
	for {
		// Listen for either an event on user's assigned event channel
		// or something on the quit channel.
		select {
		// For handling the incoming event message.
		case ev := <-userEventChan:
			writeEvent(uc, ev)

		// For returning from the go routine.
		case <-quit:
			return
		}
	}
}

// Writes a given event's payload to a given user's connection.
// TODO: It's possible that the given user has disconnected.
func writeEvent(uc userClient, ev event) {
	_, err := uc.conn.Write([]byte(ev.payload + "\r\n"))
	if err != nil {
		logErr.Println("Unable to write to user:", uc.userId)
	}
}

// Keep processing the events as long as we can. That basically means
// that we will keep processing the events in sequence order as long
// as we have already received the events. We will stop as we find some
// sequence for which event is missing.
func processEventsInOrder(eventsMap map[int]event, userEventChannels map[int]chan event, followersMap map[int]map[int]bool) {
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
func listenForEventSource(listener net.Listener, eventsChan chan<- event, quit <-chan struct{}) {
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
			// - Parse the event message to form event struct.
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
					// Parse the message to form event struct
					event, err := parseEvent(message)
					if err != nil {
						logErr.Println("Error while parsing the event message, got error:", err)
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

// Reads the event message string and parses it into event struct.
// Parse the payload and sequence first and then parse the fromUserId and
// toUserId depending on the type of the event (F|U|B|P|S).
func parseEvent(message string) (*event, error) {
	var e event
	var err error

	e.payload = message
	ev := strings.Split(message, "|")
	if len(ev) < 2 || len(ev) > 4 {
		return nil, errors.New("invalid event message format")
	}
	e.sequence, err = strconv.Atoi(ev[0])
	if err != nil {
		return nil, err
	}
	e.eventType = ev[1]
	switch e.eventType {
	case "F":
		// Follow event
		finalEvent, err := fillUserIds(e, ev)
		if err != nil {
			return nil, err
		}
		return finalEvent, nil
	case "U":
		// Unfollow event
		finalEvent, err := fillUserIds(e, ev)
		if err != nil {
			return nil, err
		}
		return finalEvent, nil
	case "B":
		// Broadcast message event
		if len(ev) != 2 {
			return nil, errors.New("")
		}
		return &e, nil
	case "P":
		// Private message event
		finalEvent, err := fillUserIds(e, ev)
		if err != nil {
			return nil, err
		}
		return finalEvent, nil
	case "S":
		// Status update event
		if len(ev) != 3 {
			return nil, errors.New("invalid event message format")
		}
		e.fromUserId, err = strconv.Atoi(ev[2])
		if err != nil {
			return nil, err
		}
		e.toUserId = 0
		return &e, nil
	}
	return nil, errors.New("invalid event type")
}

// Converts the splitted event message and converts the User Ids from it.
// Then fills them into the event struct and returns the final event.
func fillUserIds(e event, s []string) (*event, error) {
	fromUserId, toUserId, err := convertUserIds(e.payload, s)
	if err != nil {
		return nil, err
	}
	e.fromUserId = *fromUserId
	e.toUserId = *toUserId
	return &e, nil
}

// Parses the splitted event message and returns the User Ids from it.
func convertUserIds(msg string, s []string) (*int, *int, error){
	if len(s) != 4 {
		return nil, nil, errors.New("invalid event message format")
	}
	fromUserId, err := strconv.Atoi(s[2])
	if err != nil {
		return nil, nil, err
	}
	toUserId, err := strconv.Atoi(s[3])
	if err != nil {
		return nil, nil, err
	}
	return &fromUserId, &toUserId, nil
}

// Accepts the parsed event and depending on the type of the event, sends event
// to the user channels associated with the user clients which should be notified
// for a particular event.
func processSingleEvent(e event, userEventChannels map[int]chan event, followersMap map[int]map[int]bool) {
	switch e.eventType {
	case "F":
		// Follow event
		// Get the existing followers of the user.
		f, exists := followersMap[e.toUserId]
		if !exists {
			f = make(map[int]bool)
		}
		// Add a new follower to the list of followers and save it in followersMap.
		f[e.fromUserId] = true
		followersMap[e.toUserId] = f
		// Send event to the channel assigned for user event.toUserId.
		if uec, exists := userEventChannels[e.toUserId]; exists {
			uec <- e
		}
	case "U":
		// Unfollow event
		// Get all the followers of the user and delete entry for toUserId.
		if f, exists := followersMap[e.toUserId]; exists {
			delete(f, e.fromUserId)
		}
	case "B":
		// Broadcast message event
		// Send message to channels for all connected user clients.
		for _, uec := range userEventChannels {
			uec <- e
		}
	case "P":
		// Private message event
		// Send event to the channel assigned for user event.toUserId.
		if uec, exists := userEventChannels[e.toUserId]; exists {
			uec <- e
		}
	case "S":
		// Status update event
		for u := range followersMap[e.fromUserId] {
			// Send event to the channel assigned for every follower
			if uec, exists := userEventChannels[u]; exists {
				uec <- e
			}
		}
	}
}

// Accepts connection for new user clients and starts listening for their
// first message in a go routine. The read message contains the userId of
// the user a client represents. After a userId is read, the userClient
// is sent to the users channel which was created using backgroundWorkerInit().
func listenForUserClients(listener net.Listener, usersChan chan<- userClient, quit <-chan struct{}) {
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
				usersChan <- userClient{
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
