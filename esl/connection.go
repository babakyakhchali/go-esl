package esl

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
)

type ESLConfig struct {
	Host         string
	Port         int
	Password     string
	EnableBgJOBs bool
}

type ESLConnection struct {
	Handlers      map[string]func(ESLMessage)
	EventBindings []string
	Filters       map[string]string
	Config        ESLConfig
	scanner       *bufio.Scanner
	conn          net.Conn
	reader        *bufio.Reader
	writer        *bufio.Writer
	writeMutex    sync.Mutex
	replyChannel  chan ESLMessage
	errorChannel  chan error

	jobs map[string]chan ESLMessage
}

func (esl *ESLConnection) Connect() error {
	var err error
	esl.conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", esl.Config.Host, esl.Config.Port))
	if err != nil {
		return err
	}
	esl.reader = bufio.NewReader(esl.conn)
	esl.writer = bufio.NewWriter(esl.conn)

	esl.scanner = bufio.NewScanner(esl.conn)
	return nil
}

func (esl *ESLConnection) ReadLine() (string, error) {
	line, err := esl.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (esl *ESLConnection) Read(n int) (string, error) {
	bytes := make([]byte, n)
	bytesRead, err := io.ReadFull(esl.reader, bytes)
	if err != nil {
		return "", err
	}
	if n != bytesRead {
		return "", fmt.Errorf("insufficient bytes read")
	}
	return string(bytes), nil
}

func (esl *ESLConnection) readMSG() (ESLMessage, error) {
	return ParseESLStream(esl.reader)
}

func (esl *ESLConnection) API(api string, args string) (ESLMessage, error) {
	return esl.SendCMD(fmt.Sprintf("api %s %s", api, args))
}

func (esl *ESLConnection) BgAPI(api string, args string, jobUUID string) (ESLMessage, error) {
	if jobUUID == "" {
		jobUUID = uuid.New().String()
	}

	return esl.SendCMD(fmt.Sprintf("bgapi %s %s\nJob-UUID: %s", api, args, jobUUID))
}

func (esl *ESLConnection) BgAPIWithResult(api string, args string, timeout time.Duration) (ESLMessage, error) {
	jobUUID := uuid.New().String()
	jchan := make(chan ESLMessage)
	esl.jobs[jobUUID] = jchan
	esl.SendCMD(fmt.Sprintf("bgapi %s %s\nJob-UUID: %s", api, args, jobUUID))

	select {
	case nmsg := <-jchan:
		return nmsg, nil
	case <-time.After(timeout * time.Second):
		return ESLMessage{}, fmt.Errorf("job timeout")
	}
}

func (esl *ESLConnection) SendCMD(msg string) (ESLMessage, error) {
	return esl.SendCMDTimed(msg, 0)
}

func (esl *ESLConnection) SendCMDTimed(msg string, timeout time.Duration) (ESLMessage, error) {
	esl.writeMutex.Lock()
	defer esl.writeMutex.Unlock()
	err := esl.Write(msg + "\n\n")
	if err != nil {
		return ESLMessage{}, err
	}
	if timeout > 0 {
		select {
		case nmsg := <-esl.replyChannel:
			return nmsg, nil
		case <-time.After(timeout * time.Second):
			return ESLMessage{}, fmt.Errorf("esl timeout")
		}
	} else {
		nmsg := <-esl.replyChannel
		return nmsg, nil
	}

}

func (esl *ESLConnection) SendCMDf(msg string, a ...any) (ESLMessage, error) {
	cmd := fmt.Sprintf(msg, a...)
	return esl.SendCMD(cmd)
}

func (esl *ESLConnection) Writef(msg string, a ...any) error {
	_, err := fmt.Fprintf(esl.conn, msg, a...)
	if err != nil {
		return err
	}
	return nil
}

func (esl *ESLConnection) Write(msg string) error {
	_, err := fmt.Fprint(esl.conn, msg)
	if err != nil {
		return err
	}
	return nil
}

func (esl *ESLConnection) Authenticate() (ESLMessage, error) {
	msg := <-esl.replyChannel

	if msg.ContentType == "auth/request" {
		msg, err := esl.SendCMD("auth " + esl.Config.Password)
		if err != nil {
			return msg, err
		}
		if msg.GetReply() != "+OK accepted" {
			return msg, fmt.Errorf("authentication Error")
		}

	}
	return msg, nil
}

func (esl *ESLConnection) AddFilter(eventHeader string, headerValue string) (ESLMessage, error) {
	event, err := esl.SendCMDf("filter %s %s", eventHeader, headerValue)
	if err != nil {
		return event, err
	}
	if reply := event.GetReply(); !strings.HasPrefix(reply, "+OK") {
		return event, fmt.Errorf("adding filter [%s] [%s] failed, esl reply:%s", eventHeader, headerValue, reply)
	}
	return event, err
}

func (esl *ESLConnection) InitFilters() error {
	for k, v := range esl.Filters {
		event, err := esl.SendCMDf("filter %s %s", k, v)
		if err != nil {
			return err
		}
		if reply := event.GetReply(); !strings.HasPrefix(reply, "+OK") {
			return fmt.Errorf("adding filter [%s] [%s] failed, esl reply:%s", k, v, reply)
		}
	}
	return nil
}

func (esl *ESLConnection) InitEventBindings() error {
	if esl.Config.EnableBgJOBs && !slices.Contains(esl.EventBindings, "BACKGROUND_JOB") {
		esl.EventBindings = append(esl.EventBindings, "BACKGROUND_JOB")
	}
	if len(esl.EventBindings) == 0 {
		return nil
	}
	events := strings.Join(esl.EventBindings, " ")
	cmd := fmt.Sprintf("events plain %s", events)
	if events == "*" {
		cmd = "events plain all"
	}
	event, err := esl.SendCMDf(cmd)
	if err != nil {
		return err
	}
	if reply := event.GetReply(); !strings.HasPrefix(reply, "+OK") {
		return fmt.Errorf("adding event binding for [%s] failed, esl reply:%s", events, reply)
	}
	return err
}

func MergeEventBody(msg *ESLMessage) error {
	bodyMSG, err := ParseESLStream(bufio.NewReader(strings.NewReader(msg.Body)))
	if err != nil {
		return err
	}
	for header, value := range bodyMSG.Headers {
		if _, exists := msg.Headers[header]; !exists {
			value, err = url.QueryUnescape(value)
			if err != nil {
				return err
			}
			msg.Headers[header] = value
		}

	}
	if bodyMSG.Body != "" {
		msg.Body = bodyMSG.Body
	}
	return nil
}

func ParseESLStream(reader *bufio.Reader) (ESLMessage, error) {
	//reader := bufio.NewReader(strings.NewReader(s))

	msg := ESLMessage{
		msgString:     "",
		ContentType:   "",
		ContentLength: 0,
		Body:          "",
		Headers:       map[string]string{},
	}

	for {
		var header, value string
		line, err := reader.ReadString('\n')

		if err != nil {
			return msg, err
		}
		msg.msgString += line

		line = strings.TrimSpace(line)

		if idx := strings.Index(line, ":"); idx > 0 {
			header = line[:idx]
			value = line[idx+2:]
			msg.Headers[header] = value
		}
		if header == "Content-Type" {
			msg.ContentType = value
		}
		if header == "Content-Length" {
			msg.ContentLength, _ = strconv.ParseUint(value, 10, 64)
		}
		if line == "" {
			if msg.ContentLength > 0 {
				bytes := make([]byte, msg.ContentLength)
				readBytesCount, err := reader.Read(bytes)
				if msg.ContentLength != uint64(readBytesCount) {
					return msg, fmt.Errorf("not enough bytes read")
				}
				if err != nil {
					return msg, err
				}
				msg.Body = string(bytes)
			}
			return msg, nil
		}
	}
}

func (esl *ESLConnection) ReadEvents() {
	for {
		event, err := esl.readMSG() //EOF is returned if socket is closed
		if err != nil {
			esl.errorChannel <- err
			return
		}
		if event.ContentType == "auth/request" {
			esl.replyChannel <- event
		} else if event.ContentType == "text/disconnect-notice" {
			esl.errorChannel <- fmt.Errorf("disconnected with cause %s", event.Body)
			return
		} else if event.ContentType == "command/reply" {
			esl.replyChannel <- event
		} else if event.ContentType == "api/response" {
			esl.replyChannel <- event
		} else if event.ContentType == "text/event-plain" {
			if event.ContentLength > 0 {
				MergeEventBody(&event)
			}
			eventName := event.GetEventName()
			if eventName == "BACKGROUND_JOB" {
				jobUUID := event.Headers["Job-UUID"]
				if jchan, exists := esl.jobs[jobUUID]; exists {
					jchan <- event
				}
				delete(esl.jobs, jobUUID)
			}
			if handler, exists := esl.Handlers[eventName]; exists {
				handler(event)
			} else if handler, exists := esl.Handlers["*"]; exists {
				handler(event)
			}
		}
	}
}

func (esl *ESLConnection) Wait() error {
	err := <-esl.errorChannel
	return err
}

func (esl *ESLConnection) CLose() error {
	esl.writeMutex.Lock()
	defer esl.writeMutex.Unlock()
	return esl.conn.Close()
}

func NewESLConnection(config ESLConfig) ESLConnection {
	return ESLConnection{
		Handlers:      map[string]func(ESLMessage){},
		EventBindings: []string{},
		Filters:       map[string]string{},
		Config:        config,
		scanner:       &bufio.Scanner{},
		conn:          nil,
		reader:        &bufio.Reader{},
		writer:        &bufio.Writer{},
		writeMutex:    sync.Mutex{},
		replyChannel:  make(chan ESLMessage),
		errorChannel:  make(chan error),
		jobs:          map[string]chan ESLMessage{},
	}
}

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
	return s
}
