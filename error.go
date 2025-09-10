package requestbus

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

type BusError struct {
	error string

	RequestName string
	ResultType  string

	stack string
}

func (e BusError) Error() string {
	if e.ResultType != "" {
		return e.error + " : " + e.RequestName + " -> " + e.ResultType
	}
	return e.error + " : " + e.RequestName
}

func (e BusError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			// Full detail with stack trace
			_, _ = fmt.Fprint(s, e.Error())

			// Print stack on a new line
			_, _ = fmt.Fprint(s, "\n")
			_, _ = fmt.Fprint(s, e.stack)

			return
		}
		fallthrough
	case 's':
		_, _ = fmt.Fprint(s, e.Error())
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", e.Error())
	}
}

// stackTrace returns a human-readable stack trace string.
func (e BusError) StackTrace() string {
	return e.stack
}

func NewBusError(msg, requestName, resultType string) BusError {
	return BusError{
		error:       msg,
		RequestName: requestName,
		ResultType:  resultType,
		stack:       stackTrace(2),
	}
}

func stackTrace(skip int) string {
	const maxDepth = 64
	var pcs [maxDepth]uintptr

	n := runtime.Callers(skip+2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	var b strings.Builder
	for {
		f, more := frames.Next()

		// Optional filtering of noisy frames
		if strings.HasPrefix(f.Function, "runtime.") {
			if !more {
				break
			}
			continue
		}

		funcName := shortFuncName(f.Function)
		file := filepath.Base(f.File)
		fmt.Fprintf(&b, "%s\n\t%s:%d\n", funcName, file, f.Line)

		if !more {
			break
		}
	}
	return b.String()
}

func shortFuncName(full string) string {
	// Trim package path, keep pkg.Func or receiver.Func
	if i := strings.LastIndex(full, "/"); i >= 0 {
		full = full[i+1:]
	}
	return full
}
