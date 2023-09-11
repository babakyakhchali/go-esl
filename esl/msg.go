package esl

import "fmt"

type ESLMessage struct {
	msgString     string
	ContentType   string
	ContentLength uint64
	Body          string
	Headers       map[string]string
}

func (e *ESLMessage) GetReply() string {
	return e.Headers["Reply-Text"]
}

func (e *ESLMessage) GetEventName() string {
	return e.Headers["Event-Name"]
}

func (e *ESLMessage) Serialize() string {
	s := ""
	for hdr, value := range e.Headers {
		s += fmt.Sprintf("[%s] : [%s]\n", hdr, value)
	}
	if e.Body != "" {
		s += "\n\n" + e.Body
	}
	return s
}

func (e *ESLMessage) String() string {
	return e.Serialize()
}
