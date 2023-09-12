package esl

type EslSession struct {
	Conn *ESLConnection
}

func (session *EslSession) Execute(app string, args string) (msg ESLMessage, err error) {
	asyncResult, err := session.Conn.Execute(ExecutionOptions{
		App:         app,
		Args:        args,
		ChannelUUID: "",
		Lock:        false,
		Loops:       0,
		AppUUID:     "",
		Timeout:     0,
	})
	if err != nil {
		return
	}
	msg, err = asyncResult.Wait()
	return
}

func (session *EslSession) Connect() (msg ESLMessage, err error) {
	return session.Conn.SendCMD("connect")
}

func (session *EslSession) MyEvents() (msg ESLMessage, err error) {
	return session.Conn.SendCMD("myevents")
}

func (session *EslSession) Answer() (msg ESLMessage, err error) {
	return session.Execute("answer", "")
}

func (session *EslSession) Ready() bool {
	return session.Conn.IsReady()
}

func (session *EslSession) Set(name string, value string) (msg ESLMessage, err error) {
	return session.Execute("set", name+"="+value)
}
