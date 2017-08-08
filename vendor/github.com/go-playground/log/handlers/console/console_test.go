package console

import (
	"bytes"
	stdlog "log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-playground/log"
)

// NOTES:
// - Run "go test" to run tests
// - Run "gocov test | gocov report" to report on test converage by file
// - Run "gocov test | gocov annotate -" to report on all code and functions, those ,marked with "MISS" were never called
//
// or
// -- may be a good idea to change to output path to somewherelike /tmp
// go test -coverprofile cover.out && go tool cover -html=cover.out -o cover.html

func TestConsoleLogger(t *testing.T) {
	tests := getConsoleLoggerTests()
	buff := new(bytes.Buffer)

	cLog := New()
	cLog.SetWriter(buff)
	cLog.SetDisplayColor(false)
	cLog.SetBuffersAndWorkers(3, 0)
	cLog.SetTimestampFormat("MST")
	log.SetCallerInfoLevels(log.WarnLevel, log.ErrorLevel, log.PanicLevel, log.AlertLevel, log.FatalLevel)
	log.RegisterHandler(cLog, log.AllLevels...)

	for i, tt := range tests {

		buff.Reset()
		var l log.LeveledLogger

		if tt.flds != nil {
			l = log.WithFields(tt.flds...)
		} else {
			l = log.Logger
		}

		switch tt.lvl {
		case log.DebugLevel:
			if len(tt.printf) == 0 {
				l.Debug(tt.msg)
			} else {
				l.Debugf(tt.printf, tt.msg)
			}
		case log.TraceLevel:
			if len(tt.printf) == 0 {
				l.Trace(tt.msg).End()
			} else {
				l.Tracef(tt.printf, tt.msg).End()
			}
		case log.InfoLevel:
			if len(tt.printf) == 0 {
				l.Info(tt.msg)
			} else {
				l.Infof(tt.printf, tt.msg)
			}
		case log.NoticeLevel:
			if len(tt.printf) == 0 {
				l.Notice(tt.msg)
			} else {
				l.Noticef(tt.printf, tt.msg)
			}
		case log.WarnLevel:
			if len(tt.printf) == 0 {
				l.Warn(tt.msg)
			} else {
				l.Warnf(tt.printf, tt.msg)
			}
		case log.ErrorLevel:
			if len(tt.printf) == 0 {
				l.Error(tt.msg)
			} else {
				l.Errorf(tt.printf, tt.msg)
			}
		case log.PanicLevel:
			func() {
				defer func() {
					recover()
				}()

				if len(tt.printf) == 0 {
					l.Panic(tt.msg)
				} else {
					l.Panicf(tt.printf, tt.msg)
				}
			}()
		case log.AlertLevel:
			if len(tt.printf) == 0 {
				l.Alert(tt.msg)
			} else {
				l.Alertf(tt.printf, tt.msg)
			}
		}

		if buff.String() != tt.want {

			if tt.lvl == log.TraceLevel {
				if !strings.HasPrefix(buff.String(), tt.want) {
					t.Errorf("test %d: Expected '%s' Got '%s'", i, tt.want, buff.String())
				}
				continue
			}

			t.Errorf("test %d: Expected '%s' Got '%s'", i, tt.want, buff.String())
		}
	}
}

func TestConsoleLoggerColor(t *testing.T) {

	tests := getConsoleLoggerColorTests()
	buff := new(bytes.Buffer)

	cLog := New()
	cLog.SetWriter(buff)
	cLog.SetDisplayColor(true)
	cLog.SetBuffersAndWorkers(3, 3)
	cLog.SetTimestampFormat("MST")

	log.RegisterHandler(cLog, log.AllLevels...)

	for i, tt := range tests {

		buff.Reset()
		var l log.LeveledLogger

		if tt.flds != nil {
			l = log.WithFields(tt.flds...)
		} else {
			l = log.Logger
		}

		switch tt.lvl {
		case log.DebugLevel:
			if len(tt.printf) == 0 {
				l.Debug(tt.msg)
			} else {
				l.Debugf(tt.printf, tt.msg)
			}
		case log.TraceLevel:
			if len(tt.printf) == 0 {
				l.Trace(tt.msg).End()
			} else {
				l.Tracef(tt.printf, tt.msg).End()
			}
		case log.InfoLevel:
			if len(tt.printf) == 0 {
				l.Info(tt.msg)
			} else {
				l.Infof(tt.printf, tt.msg)
			}
		case log.NoticeLevel:
			if len(tt.printf) == 0 {
				l.Notice(tt.msg)
			} else {
				l.Noticef(tt.printf, tt.msg)
			}
		case log.WarnLevel:
			if len(tt.printf) == 0 {
				l.Warn(tt.msg)
			} else {
				l.Warnf(tt.printf, tt.msg)
			}
		case log.ErrorLevel:
			if len(tt.printf) == 0 {
				l.Error(tt.msg)
			} else {
				l.Errorf(tt.printf, tt.msg)
			}
		case log.PanicLevel:
			func() {
				defer func() {
					recover()
				}()

				if len(tt.printf) == 0 {
					l.Panic(tt.msg)
				} else {
					l.Panicf(tt.printf, tt.msg)
				}
			}()
		case log.AlertLevel:
			if len(tt.printf) == 0 {
				l.Alert(tt.msg)
			} else {
				l.Alertf(tt.printf, tt.msg)
			}
		}

		if buff.String() != tt.want {

			if tt.lvl == log.TraceLevel {
				if !strings.HasPrefix(buff.String(), tt.want) {
					t.Errorf("test %d: Expected '%s' Got '%s'", i, tt.want, buff.String())
				}
				continue
			}

			t.Errorf("test %d: Expected '%s' Got '%s'", i, tt.want, buff.String())
		}
	}
}

func TestCustomFormatFunc(t *testing.T) {

	buff := new(bytes.Buffer)
	cLog := New()
	cLog.SetWriter(buff)
	cLog.SetTimestampFormat("2006")
	cLog.SetBuffersAndWorkers(3, 2)
	cLog.SetFormatFunc(func(c *Console) Formatter {

		var b []byte

		return func(e *log.Entry) []byte {
			b = b[0:0]
			b = append(b, e.Message...)
			return b
		}
	})

	log.RegisterHandler(cLog, log.AllLevels...)

	log.Debug("debug")
	if buff.String() != "debug" {
		log.Errorf("Expected '%s' Got '%s'", "debug", buff.String())
	}
	buff.Reset()
}

func TestSetFilename(t *testing.T) {
	buff := new(bytes.Buffer)

	cLog := New()
	cLog.SetWriter(buff)
	cLog.SetDisplayColor(false)
	cLog.SetBuffersAndWorkers(3, 1)
	cLog.SetTimestampFormat("MST")
	cLog.SetFilenameDisplay(log.Llongfile)

	log.RegisterHandler(cLog, log.AllLevels...)

	log.Error("error")
	if !strings.Contains(buff.String(), "log/handlers/console/console_test.go:251 error") {
		t.Errorf("Expected '%s' Got '%s'", "log/handlers/console/console_test.go:251 error", buff.String())
	}
	buff.Reset()
}

func TestSetFilenameColor(t *testing.T) {
	buff := new(bytes.Buffer)

	cLog := New()
	cLog.SetWriter(buff)
	cLog.SetDisplayColor(true)
	cLog.SetBuffersAndWorkers(3, 1)
	cLog.SetTimestampFormat("MST")
	cLog.SetFilenameDisplay(log.Llongfile)

	log.RegisterHandler(cLog, log.AllLevels...)

	log.Error("error")
	if !strings.Contains(buff.String(), "log/handlers/console/console_test.go:270 error") {
		t.Errorf("Expected '%s' Got '%s'", "log/handlers/console/console_test.go:270 error", buff.String())
	}
	buff.Reset()
}

func TestConsoleSTDLogCapturing(t *testing.T) {

	var m sync.Mutex
	buff := new(bytes.Buffer)

	cLog := New()
	cLog.SetDisplayColor(false)
	cLog.SetBuffersAndWorkers(3, 3)
	cLog.SetTimestampFormat("MST")
	cLog.RedirectSTDLogOutput(true)
	cLog.SetFormatFunc(func(c *Console) Formatter {
		return func(e *log.Entry) []byte {
			m.Lock()
			defer m.Unlock()
			buff.Write([]byte(e.Message))

			return buff.Bytes()
		}
	})

	log.RegisterHandler(cLog, log.AllLevels...)

	stdlog.Println("STD LOG message")

	time.Sleep(500 * time.Millisecond)

	m.Lock()
	s := buff.String()
	m.Unlock()

	// expected := "STD LOG message stdlog=true"
	expected := "STD LOG message"

	if !strings.Contains(s, expected) {
		t.Errorf("Expected '%s' Got '%s'", expected, s)
	}
}

type test struct {
	lvl    log.Level
	msg    string
	flds   []log.Field
	want   string
	printf string
}

func getConsoleLoggerTests() []test {
	return []test{
		{
			lvl:  log.DebugLevel,
			msg:  "debug",
			flds: nil,
			want: "UTC  DEBUG debug\n",
		},
		{
			lvl:    log.DebugLevel,
			msg:    "debugf",
			printf: "%s",
			flds:   nil,
			want:   "UTC  DEBUG debugf\n",
		},
		{
			lvl:  log.InfoLevel,
			msg:  "info",
			flds: nil,
			want: "UTC   INFO info\n",
		},
		{
			lvl:    log.InfoLevel,
			msg:    "infof",
			printf: "%s",
			flds:   nil,
			want:   "UTC   INFO infof\n",
		},
		{
			lvl:  log.NoticeLevel,
			msg:  "notice",
			flds: nil,
			want: "UTC NOTICE notice\n",
		},
		{
			lvl:    log.NoticeLevel,
			msg:    "noticef",
			printf: "%s",
			flds:   nil,
			want:   "UTC NOTICE noticef\n",
		},
		{
			lvl:  log.WarnLevel,
			msg:  "warn",
			flds: nil,
			want: "UTC   WARN console_test.go:73 warn\n",
		},
		{
			lvl:    log.WarnLevel,
			msg:    "warnf",
			printf: "%s",
			flds:   nil,
			want:   "UTC   WARN console_test.go:75 warnf\n",
		},
		{
			lvl:  log.ErrorLevel,
			msg:  "error",
			flds: nil,
			want: "UTC  ERROR console_test.go:79 error\n",
		},
		{
			lvl:    log.ErrorLevel,
			msg:    "errorf",
			printf: "%s",
			flds:   nil,
			want:   "UTC  ERROR console_test.go:81 errorf\n",
		},
		{
			lvl:  log.AlertLevel,
			msg:  "alert",
			flds: nil,
			want: "UTC  ALERT console_test.go:97 alert\n",
		},
		{
			lvl:    log.AlertLevel,
			msg:    "alertf",
			printf: "%s",
			flds:   nil,
			want:   "UTC  ALERT console_test.go:99 alertf\n",
		},
		{
			lvl:  log.PanicLevel,
			msg:  "panic",
			flds: nil,
			want: "UTC  PANIC console_test.go:90 panic\n",
		},
		{
			lvl:    log.PanicLevel,
			msg:    "panicf",
			printf: "%s",
			flds:   nil,
			want:   "UTC  PANIC console_test.go:92 panicf\n",
		},
		{
			lvl: log.DebugLevel,
			msg: "debug",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  DEBUG debug key=value\n",
		},
		{
			lvl:    log.DebugLevel,
			msg:    "debugf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  DEBUG debugf key=value\n",
		},
		{
			lvl: log.InfoLevel,
			msg: "info",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC   INFO info key=value\n",
		},
		{
			lvl:    log.InfoLevel,
			msg:    "infof",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC   INFO infof key=value\n",
		},
		{
			lvl: log.NoticeLevel,
			msg: "notice",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC NOTICE notice key=value\n",
		},
		{
			lvl:    log.NoticeLevel,
			msg:    "noticef",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC NOTICE noticef key=value\n",
		},
		{
			lvl: log.WarnLevel,
			msg: "warn",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC   WARN console_test.go:73 warn key=value\n",
		},
		{
			lvl:    log.WarnLevel,
			msg:    "warnf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC   WARN console_test.go:75 warnf key=value\n",
		},
		{
			lvl: log.ErrorLevel,
			msg: "error",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  ERROR console_test.go:79 error key=value\n",
		},
		{
			lvl:    log.ErrorLevel,
			msg:    "errorf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  ERROR console_test.go:81 errorf key=value\n",
		},
		{
			lvl: log.AlertLevel,
			msg: "alert",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  ALERT console_test.go:97 alert key=value\n",
		},
		{
			lvl: log.AlertLevel,
			msg: "alert",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  ALERT console_test.go:97 alert key=value\n",
		},
		{
			lvl:    log.AlertLevel,
			msg:    "alertf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  ALERT console_test.go:99 alertf key=value\n",
		},
		{
			lvl:    log.PanicLevel,
			msg:    "panicf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  PANIC console_test.go:92 panicf key=value\n",
		},
		{
			lvl: log.PanicLevel,
			msg: "panic",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  PANIC console_test.go:90 panic key=value\n",
		},
		{
			lvl:  log.TraceLevel,
			msg:  "trace",
			flds: nil,
			want: "UTC  TRACE trace",
		},
		{
			lvl:    log.TraceLevel,
			msg:    "tracef",
			printf: "%s",
			flds:   nil,
			want:   "UTC  TRACE tracef",
		},
		{
			lvl: log.TraceLevel,
			msg: "trace",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  TRACE trace key=value",
		},
		{
			lvl:    log.TraceLevel,
			msg:    "tracef",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC  TRACE tracef key=value",
		},
		{
			lvl: log.DebugLevel,
			msg: "debug",
			flds: []log.Field{
				log.F("key", "string"),
				log.F("key", int(1)),
				log.F("key", int8(2)),
				log.F("key", int16(3)),
				log.F("key", int32(4)),
				log.F("key", int64(5)),
				log.F("key", uint(1)),
				log.F("key", uint8(2)),
				log.F("key", uint16(3)),
				log.F("key", uint32(4)),
				log.F("key", uint64(5)),
				log.F("key", true),
				log.F("key", struct{ value string }{"struct"}),
			},
			want: "UTC  DEBUG debug key=string key=1 key=2 key=3 key=4 key=5 key=1 key=2 key=3 key=4 key=5 key=true key={struct}\n",
		},
	}
}

func getConsoleLoggerColorTests() []test {
	return []test{
		{
			lvl:    log.DebugLevel,
			msg:    "debugf",
			printf: "%s",
			flds:   nil,
			want: "UTC [32m DEBUG[0m debugf\n",
		},
		{
			lvl:  log.DebugLevel,
			msg:  "debug",
			flds: nil,
			want: "UTC [32m DEBUG[0m debug\n",
		},
		{
			lvl:    log.InfoLevel,
			msg:    "infof",
			printf: "%s",
			flds:   nil,
			want: "UTC [34m  INFO[0m infof\n",
		},
		{
			lvl:  log.InfoLevel,
			msg:  "info",
			flds: nil,
			want: "UTC [34m  INFO[0m info\n",
		},
		{
			lvl:    log.NoticeLevel,
			msg:    "noticef",
			printf: "%s",
			flds:   nil,
			want: "UTC [36;1mNOTICE[0m noticef\n",
		},
		{
			lvl:  log.NoticeLevel,
			msg:  "notice",
			flds: nil,
			want: "UTC [36;1mNOTICE[0m notice\n",
		},
		{
			lvl:    log.WarnLevel,
			msg:    "warnf",
			printf: "%s",
			flds:   nil,
			want: "UTC [33;1m  WARN[0m console_test.go:170 warnf\n",
		},
		{
			lvl:  log.WarnLevel,
			msg:  "warn",
			flds: nil,
			want: "UTC [33;1m  WARN[0m console_test.go:168 warn\n",
		},
		{
			lvl:    log.ErrorLevel,
			msg:    "errorf",
			printf: "%s",
			flds:   nil,
			want: "UTC [31;1m ERROR[0m console_test.go:176 errorf\n",
		},
		{
			lvl:  log.ErrorLevel,
			msg:  "error",
			flds: nil,
			want: "UTC [31;1m ERROR[0m console_test.go:174 error\n",
		},
		{
			lvl:    log.AlertLevel,
			msg:    "alertf",
			printf: "%s",
			flds:   nil,
			want: "UTC [31m[4m ALERT[0m console_test.go:194 alertf\n",
		},
		{
			lvl:  log.AlertLevel,
			msg:  "alert",
			flds: nil,
			want: "UTC [31m[4m ALERT[0m console_test.go:192 alert\n",
		},
		{
			lvl:    log.PanicLevel,
			msg:    "panicf",
			printf: "%s",
			flds:   nil,
			want: "UTC [31m PANIC[0m console_test.go:187 panicf\n",
		},
		{
			lvl:  log.PanicLevel,
			msg:  "panic",
			flds: nil,
			want: "UTC [31m PANIC[0m console_test.go:185 panic\n",
		},
		{
			lvl:    log.DebugLevel,
			msg:    "debugf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [32m DEBUG[0m debugf [32mkey[0m=value\n",
		},
		{
			lvl: log.DebugLevel,
			msg: "debug",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [32m DEBUG[0m debug [32mkey[0m=value\n",
		},
		{
			lvl:    log.InfoLevel,
			msg:    "infof",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [34m  INFO[0m infof [34mkey[0m=value\n",
		},
		{
			lvl: log.InfoLevel,
			msg: "info",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [34m  INFO[0m info [34mkey[0m=value\n",
		},
		{
			lvl:    log.NoticeLevel,
			msg:    "noticef",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [36;1mNOTICE[0m noticef [36;1mkey[0m=value\n",
		},
		{
			lvl: log.NoticeLevel,
			msg: "notice",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [36;1mNOTICE[0m notice [36;1mkey[0m=value\n",
		},
		{
			lvl:    log.WarnLevel,
			msg:    "warnf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [33;1m  WARN[0m console_test.go:170 warnf [33;1mkey[0m=value\n",
		},
		{
			lvl: log.WarnLevel,
			msg: "warn",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [33;1m  WARN[0m console_test.go:168 warn [33;1mkey[0m=value\n",
		},
		{
			lvl:    log.ErrorLevel,
			msg:    "errorf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [31;1m ERROR[0m console_test.go:176 errorf [31;1mkey[0m=value\n",
		},
		{
			lvl: log.ErrorLevel,
			msg: "error",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [31;1m ERROR[0m console_test.go:174 error [31;1mkey[0m=value\n",
		},
		{
			lvl:    log.AlertLevel,
			msg:    "alertf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [31m[4m ALERT[0m console_test.go:194 alertf [31m[4mkey[0m=value\n",
		},
		{
			lvl: log.AlertLevel,
			msg: "alert",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [31m[4m ALERT[0m console_test.go:192 alert [31m[4mkey[0m=value\n",
		},
		{
			lvl:    log.PanicLevel,
			msg:    "panicf",
			printf: "%s",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [31m PANIC[0m console_test.go:187 panicf [31mkey[0m=value\n",
		},
		{
			lvl: log.PanicLevel,
			msg: "panic",
			flds: []log.Field{
				log.F("key", "value"),
			},
			want: "UTC [31m PANIC[0m console_test.go:185 panic [31mkey[0m=value\n",
		},
		{
			lvl: log.DebugLevel,
			msg: "debug",
			flds: []log.Field{
				log.F("key", "string"),
				log.F("key", int(1)),
				log.F("key", int8(2)),
				log.F("key", int16(3)),
				log.F("key", int32(4)),
				log.F("key", int64(5)),
				log.F("key", uint(1)),
				log.F("key", uint8(2)),
				log.F("key", uint16(3)),
				log.F("key", uint32(4)),
				log.F("key", uint64(5)),
				log.F("key", float32(5.33)),
				log.F("key", float64(5.34)),
				log.F("key", true),
				log.F("key", struct{ value string }{"struct"}),
			},
			want: "UTC [32m DEBUG[0m debug [32mkey[0m=string [32mkey[0m=1 [32mkey[0m=2 [32mkey[0m=3 [32mkey[0m=4 [32mkey[0m=5 [32mkey[0m=1 [32mkey[0m=2 [32mkey[0m=3 [32mkey[0m=4 [32mkey[0m=5 [32mkey[0m=5.33 [32mkey[0m=5.34 [32mkey[0m=true [32mkey[0m={struct}\n",
		},
	}
}
