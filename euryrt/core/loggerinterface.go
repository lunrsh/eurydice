package core

type Logger interface {
	Debug(msg any, keyvals ...any)
	Debugf(msg string, keyvals ...any)
	Info(msg any, keyvals ...any)
	Infof(msg string, keyvals ...any)
	Warn(msg any, keyvals ...any)
	Warnf(msg string, keyvals ...any)
	Error(msg any, keyvals ...any)
	Errorf(msg string, keyvals ...any)
	Print(msg any, keyvals ...any)
	Printf(msg string, keyvals ...any)
}
