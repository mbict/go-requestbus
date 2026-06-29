package requestbus

import (
	"fmt"
	"runtime"
	"strings"
)

// stackTrace returns a panic-style stack trace string from its caller.
// The first captured frame is the direct caller of stackTrace; skip hides
// additional frames from that point (for example, skip=1 hides Invoke).
func stackTrace(skip int) string {
	const maxFrames = 64
	pcs := make([]uintptr, maxFrames)

	// Skip runtime.Callers and StackTrace itself.
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])

	var b strings.Builder
	toSkip := skip

	for {
		frame, more := frames.Next()
		if toSkip > 0 {
			toSkip--
			if !more {
				break
			}
			continue
		}

		if frame.Function == "runtime.goexit" || frame.Function == "runtime.main" {
			if !more {
				break
			}
			continue
		}

		b.WriteString(frame.Function)
		b.WriteByte('\n')
		_, _ = fmt.Fprintf(&b, "\t%s:%d\n", frame.File, frame.Line)

		if !more {
			break
		}
	}

	return strings.TrimSuffix(b.String(), "\n")
}
