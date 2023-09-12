package esl

import (
	"bufio"
	"fmt"
	"os"
	"sync"

	"golang.org/x/exp/slog"
)

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

func NewInboundESLConnection(config ESLConfig) ESLConnection {
	return ESLConnection{
		Handlers:      map[string]func(ESLMessage){},
		EventBindings: []string{},
		Filters:       map[string]string{},
		Config:        config,
		conn:          nil,
		reader:        &bufio.Reader{},
		writer:        &bufio.Writer{},
		writeMutex:    sync.Mutex{},
		replyChannel:  make(chan ESLMessage),
		errorChannel:  make(chan error, 1),
		jobs:          map[string]AsyncEslAction{},
		LogMessages:   false,
		Logger:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
		status:        "created",
		replyTimeout:  0,
	}
}
