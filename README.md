# go-eventserver

EventServer is a socket server which reads events from an *event source* and
forwards them to the *user clients* when appropriate.

Check the section [Implementation Details](#implementation-details) below for more details.

## Quick Start
Make sure you have a working [Go environment](https://golang.org/doc/install).

### Dependencies
- Golang 1.8 or later
- `make` (optional)

### Steps to install and run the server
1. Clone the repository into your GOPATH
1. Go to the project directory:
`cd $GOPATH/src/github.com/sahildua2305/go-eventserver`
1. Modify `./config/config.json` file to configure the event source and user
client ports as you want.
1. Run the event server: `make run`
1. The event server will show give output like below:
```
INFO: 2017/12/17 15:00:46 eventserver.go:508: Loaded the event server config
INFO: 2017/12/17 15:00:46 eventserver.go:83: Listening for event source on port: 9090
INFO: 2017/12/17 15:00:46 eventserver.go:91: Listening for user clients on port: 9099
INFO: 2017/12/17 15:00:46 eventserver.go:515: Started the event server
```
This means that the server is now running and listening on ports mentioned
in your `config.json` file.

**Note**: If you don't have `make`, you can build the binary using:
```bash
$ go build .
```
And then run the binary using:
```bash
$ ./go-eventserver
```

### Running tests

To run tests for the entire package:
```bash
$ make test
```

To run the tests with coverage:
```bash
$ make test-cover
```

To run the benchmark tests for the package:
```bash
$ make test-bench
```

### Code formatting

To format the code during development:
```bash
$ make fmt
```

By default, the above command is of *dry* nature. It only shows the changes
that gofmt tool suggests.

To actually make the changes, you need to run:
```bash
$ make fmt-wet
```

To run [`golint`](https://github.com/golang/lint) on the source code:
```bash
$ make lint
```

### Quick test program

To test the event server with a test program that comes with this repository,
run the following command in another terminal:

```bash
$ ./followermaze.sh
```

This script will initialize a test program which starts sending events.

**Note**: The test program uses port 9090 for event source and 9099 for
user clients by default. If you want to use different ports, you can set
the following environment variables: *eventListenerPort* and
*clientListenerPort*.


## Running in Production
To run EventServer in production, you can install the versioned binary file
and run it with the corresponding `./config/config.json` file.

Event Server accepts the following command line argument(s):
```
  -config string
      config file to load (default "./config/config.json")
```

To run the compiled binary in production with custom config file:
```bash
$ ./go-eventserver -config "/path/to/config/file"
```

The configuration file in production can be served using some configuration
management tool like Puppet or Chef.

It's recommended to run the binary wrapped in a service using tools like -
[supervisord](http://supervisord.org/index.html) or
[serviced](https://github.com/control-center/serviced).

### Server Configuration

You can configure the following parameters of the server by specifying them
in `config.json` file:
- **eventListenerPort** - The port used by the event source.
- **clientListenerPort** - The port used to register clients.


## Implementation Details

- *event source*: It will send a stream of events which may or may not require
clients to be notified.
- *user clients*: Each one representing a specific user, these wait for
notifications for events which would be relevant to the user they represent.

#### The Events
There are five possible events. The table below describe payloads
sent by the *event source* and what they represent:

| Payload       | Sequence #| Type         | From User Id | To User Id |
|---------------|-----------|--------------|--------------|------------|
|666\|F\|60\|50 | 666       | Follow       | 60           | 50         |
|1\|U\|12\|9    | 1         | Unfollow     | 12           | 9          |
|542532\|B      | 542532    | Broadcast    | -            | -          |
|43\|P\|32\|56  | 43        | Private Msg  | 32           | 56         |
|634\|S\|32     | 634       | Status Update| 32           | -          |

Please check `instructions.md` to read more about these event types.

Events can come out of order, but clients will always receive the events in
the right order of sequence number.

## Scope of Improvement

- **Logging**: Logging can be improved. Right now, only the info logs are
being sent to stdout and the error logs are being sent to stderr. The current
implementation uses the built-in "log" package. We can use some more mature
logging module which can support the levelled logging to support different
log levels like - debug, info, warn, error, fatal.
- **Monitoring**: The current implementation does not have any kind of
monitoring around the system. We should have some monitoring to measure how
many events we are serving, what kind of clients are connecting and if
everything is going alright with our server. We can use
[go-metrics](https://github.com/rcrowley/go-metrics) package. We can keep
these monitoring metrics in [graphite](https://github.com/cyberdelia/go-metrics-graphite)
and then use some tools like [Bosun](https://bosun.org/) to enable alerts
in undesirable situations.
- **Sequence persistence**: We can implement the offset management similar to
how Kafka handles it in order to solve the following problems:
  - Make the server recover from the sequence number where it stopped earlier.
For example - in case something goes wrong with the server and it stops in
the middle, what happens with the events that it already had received?
  - Support the scenario when an event source ends the message streams and want
to start sending events again? Two possibilities - do we start processing from
the sequence number where we left or do we start from 1?
