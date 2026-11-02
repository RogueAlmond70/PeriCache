package internal

type Logger interface {
	Error(string, ...any)
	Info(string, ...any)
	Warn(string, ...any)
}
