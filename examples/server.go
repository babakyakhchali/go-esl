package main

import (
	"fmt"
	"os"

	"github.com/babakyakhchali/go-esl/esl"
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

func OnNewConnection(conn *esl.EslOutboundConnection) {

	conn.EnableMessageLogging(esl.MessageLogFormatBrief)
	err := conn.Connect()
	if err != nil {
		conn.CLose()
		return
	}
	conn.EnableAsync() //to use Wait for execute result enable async operations
	//this starts reading from socket in a separte goroutin

	session := esl.EslSession{
		Conn: conn,
	}

	if err != nil {
		conn.CLose()
		return
	}
	l := logger.With("uuid", session.GetChannelData().GetHeader(esl.MessageHeaderUniqueID))
	l.Info("connected")
	msg, err := session.Conn.MyEvents(map[string]func(esl.ESLMessage){"*": onEvent})
	if err != nil {
		return
	}
	l.Debug(msg.String())

	_, err = session.Answer()
	if err != nil {
		return
	}
	session.Set("tts_voice", "kal")
	_, err = session.Set("tts_engine", "flite")
	if err != nil {
		return
	}

	_, err = session.Execute("speak", "hi, this is a test ivr tts, bye!")
	if err != nil {
		return
	}
	_, err = session.Execute("sleep", "1000")
	if err != nil {
		return
	}
	_, err = session.Execute("hangup", "NORMAL_CLEARING")
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
