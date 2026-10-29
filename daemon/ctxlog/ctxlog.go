// Adds information from ctx.Value() to log lines. The following keys are supported:
//   - tag (string)
//   - traceId (int32)
//   - clientId (int32)
//   - alarmId (float64)
//   - clientTask (string)
//
// Keys are wrapped using CtxLogKey
package ctxlog

import (
	"context"
	"log"
	"os"
	"runtime"
	"strings"
)

type CtxLogKey struct {
	key      string
	inLog    string
	validate func(v any) bool
}

var (
	TagKey        = &CtxLogKey{"tag", "tag", HasType[string]}
	TraceIdKey    = &CtxLogKey{"traceId", "tId", HasType[int32]}
	ClientIdKey   = &CtxLogKey{"clientId", "cId", HasType[int32]}
	AlarmIdKey    = &CtxLogKey{"alarmId", "aId", HasType[float64]}
	ClientTaskKey = &CtxLogKey{"clientTask", "ctask", HasType[string]}
	// defines order in logs
	keys []*CtxLogKey = []*CtxLogKey{TagKey, ClientTaskKey, TraceIdKey, ClientIdKey, AlarmIdKey}
)

func HasType[T any](v any) bool {
	_, ok := v.(T)
	return ok
}

func Printf(ctx context.Context, fmt string, v ...any) {
	fmtBuilder := strings.Builder{}
	vx := make([]any, 0, 4+len(v))

	if _, file, line, ok := runtime.Caller(1); ok {
		fmtBuilder.WriteString("%v:%v ")
		if cwd, err := os.Getwd(); err == nil {
			file, _ = strings.CutPrefix(file, cwd+string(os.PathSeparator))
		}
		vx = append(vx, file, line)
	}
	for _, key := range keys {
		value := ctx.Value(key)
		if value != nil && key.validate(value) {
			fmtBuilder.WriteString(key.inLog)
			fmtBuilder.WriteByte('=')
			fmtBuilder.WriteString("%v ")
			vx = append(vx, value)
		}
	}
	fmtBuilder.WriteString("-- ")
	fmtBuilder.WriteString(fmt)
	vx = append(vx, v...)

	log.Printf(fmtBuilder.String(), vx...)
}

func WithAlarmId(ctx context.Context, alarmId float64) context.Context {
	return context.WithValue(ctx, AlarmIdKey, alarmId)
}
