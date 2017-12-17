# go-eventserver

**Event Server** is a socket server which reads events from an event source and forwards them to the user clients when appropriate.

## Scope of Improvement

Rather than a plain list of things-to-do, this is of a list of topics to start a discussion on, within the team and reach a consensus before implementing:
- Make the server recover from the sequence number where it stopped earlier. For example - in case something goes wrong with the server and it stops in the middle, what happens with the events that it already had received?
- What happens when an event source ends the message streams and want to start sending events again? Two possibilities - do we start processing from the sequence number where we left or do we start from 1?
