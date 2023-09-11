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
		fmt.Printf("got event %s", e.String())
	}
}

func OnNewConnection(esl *esl.ESLConnection) {

	esl.EventBindings = []string{}
	esl.Handlers["*"] = onEvent
	esl.InitEventBindings()
	channelData, err := esl.SendCMD("connect")
	if err != nil {
		panic(err)
	}
	l := logger.With("uuid", channelData.Headers["Unique-ID"])
	l.Info("connected")
	msg, err := esl.SendCMD("myevents")
	if err != nil {
		return
	}
	l.Debug(msg.String())
	msg, err = esl.Execute("answer", "", "", true, 0, "")
	if err != nil {
		return
	}
	l.Debug(msg.String())
	msg, err = esl.Execute("sleep", "5000", "", true, 0, "")
	if err != nil {
		return
	}
	l.Debug(msg.String())
	msg, err = esl.Execute("hangup", "NORMAL_CLEARING", "", true, 0, "")
	if err != nil {
		return
	}
	l.Debug(msg.String())
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
