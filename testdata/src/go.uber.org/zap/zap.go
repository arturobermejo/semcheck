// Package zap is a minimal double of go.uber.org/zap: just enough declarations
// for the fixtures to type-check.
package zap

type Logger struct{}

type Field struct{}

func L() *Logger { return &Logger{} }

func (l *Logger) Info(msg string, fields ...Field) {}

func (l *Logger) With(fields ...Field) *Logger { return l }

func String(key, val string) Field { return Field{} }

func Any(key string, val any) Field { return Field{} }
