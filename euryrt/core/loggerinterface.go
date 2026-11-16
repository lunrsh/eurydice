package core

type Logger interface {
	Debug(msg any, keyvals ...any)
	Debugf(msg any, keyvals ...any)
	Info(msg any, keyvals ...any)
	Infof(msg any, keyvals ...any)
	Warn(msg any, keyvals ...any)
	Warnf(msg any, keyvals ...any)
	Error(msg any, keyvals ...any)
	Errorf(msg any, keyvals ...any)
	Print(msg any, keyvals ...any)
	Printf(msg any, keyvals ...any)
}
