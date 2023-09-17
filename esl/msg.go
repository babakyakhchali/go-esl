package esl

import "fmt"

// A ESLMessage models messages send or received on esl connection.
type ESLMessage struct {
	msgString     string // raw string as read from socket
	ContentType   string
	ContentLength uint64
	Body          string
	Headers       map[string]string
}

func (e ESLMessage) GetReply() string {
	return e.Headers["Reply-Text"]
}

func (e ESLMessage) GetHeader(hdr string) string {
	return e.Headers[hdr]
}

func (e *ESLMessage) GetEventName() string {
	return e.Headers["Event-Name"]
}

func (e ESLMessage) Serialize() string {
	s := ""
	for hdr, value := range e.Headers {
		s += fmt.Sprintf("[%s] : [%s]\n", hdr, value)
	}
	if e.Body != "" {
		s += "\n\n" + e.Body
	}
	return s
}

func (e ESLMessage) String() string {
	return e.Serialize()
}

func (e ESLMessage) StringBrief() string {
	if e.ContentType == ContentTypeTextEventPlain {
		return fmt.Sprintf("Event-Name:%s, UniqueID:%s", e.Headers[MessageHeaderEventName], e.Headers[MessageHeaderUniqueID])
	} else {
		return e.String()
	}
}

type EslChannelExecuteResult struct {
	ApplicationData     string
	ApplicationResponse string
	ApplicationUUID     string
	Application         string
	channelUUID         string
}

/*
Application: playback
Application-Data: test.wav
Application-Response: FILE%20NOT%20FOUND
Application-UUID: 0b492cff-0b48-4d7a-93cd-6629c9c28388
*/
func (msg ESLMessage) GetExecuteInfo() EslChannelExecuteResult {
	return EslChannelExecuteResult{
		ApplicationData:     msg.GetHeader("Application-Data"),
		ApplicationResponse: msg.GetHeader("Application-Response"),
		ApplicationUUID:     msg.GetHeader("Application-UUID"),
		Application:         msg.GetHeader("Application"),
		channelUUID:         msg.GetHeader(MessageHeaderUniqueID),
	}
}
