package main

import (
	"fmt"
	"os"

	"github.com/babakyakhchali/go-fsesl/esl"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

func onEvent(e esl.ESLMessage) {
	//fmt.Printf("got event %v", e)
	wantedEvents := []string{"CHANNEL_ANSWER", "CHANNEL_EXECUTE", "CHANNEL_EXECUTE_COMPLETE"}

	if slices.Contains(wantedEvents, e.Headers["Event-Name"]) {
		fmt.Printf("got event %s\n", e.Headers["Event-Name"])
	}
}

/*
<action application="set" data="tts_engine=flite"/>
        <action application="set" data="tts_voice=kal"/>
        <action application="speak" data="This is flite on FreeSWITCH"/>*/

func OnNewConnection(conn *esl.ESLConnection) {
	conn.LogMessages = true
	conn.LogMessagesFormat = esl.MessageLogFormatBrief
	conn.EventBindings = []string{}
	conn.Handlers["*"] = onEvent
	conn.InitEventBindings()
	go conn.ReadMessages()
	session := esl.EslSession{
		Conn: conn,
	}
	channelData, err := session.Connect()
	if err != nil {
		panic(err)
	}
	l := logger.With("uuid", channelData.Headers["Unique-ID"])
	l.Info("connected")
	msg, err := session.MyEvents()
	if err != nil {
		return
	}
	l.Debug(msg.String())

	msg, err = session.Answer()
	if err != nil {
		return
	}
	session.Set("tts_voice", "kal")
	msg, err = session.Set("tts_engine", "flite")
	if err != nil {
		return
	}

	msg, err = session.Execute("speak", "hi, this is a test ivr tts, bye!")
	if err != nil {
		return
	}
	msg, err = session.Execute("sleep", "1000")
	if err != nil {
		return
	}
	msg, err = session.Execute("hangup", "NORMAL_CLEARING")
	if err != nil {
		return
	}
}

var logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

func main() {
	server := esl.ESLServer{
		Address:           "127.0.0.1:8020",
		ConnectionHandler: OnNewConnection,
	}
	server.Logger = logger

	err := server.Run()
	fmt.Printf("Server ended with result:%s", err)
}
