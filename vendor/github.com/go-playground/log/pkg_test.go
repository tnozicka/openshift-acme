package log

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// NOTES:
// - Run "go test" to run tests
// - Run "gocov test | gocov report" to report on test converage by file
// - Run "gocov test | gocov annotate -" to report on all code and functions, those ,marked with "MISS" were never called
//
// or
//
// -- may be a good idea to change to output path to somewherelike /tmp
// go test -coverprofile cover.out && go tool cover -html=cover.out -o cover.html
//

func TestConsoleLogger2(t *testing.T) {
	stackTraceLimit = 1000
	tests := getLogTests()
	buff := new(bytes.Buffer)

	th := &testHandler{
		writer: buff,
	}
	SetCallerInfoLevels(WarnLevel, ErrorLevel, PanicLevel, AlertLevel, FatalLevel)
	RegisterHandler(th, AllLevels...)
	SetExitFunc(os.Exit)
	if bl := HasHandlers(); !bl {
		t.Errorf("test HasHandlers: Expected '%t' Got '%t'", true, bl)
	}

	for i, tt := range tests {

		buff.Reset()
		var l LeveledLogger

		if tt.flds != nil {
			l = WithFields(tt.flds...)

			switch tt.lvl {
			case uint8(DebugLevel):
				if len(tt.printf) == 0 {
					l.Debug(tt.msg)
				} else {
					l.Debugf(tt.printf, tt.msg)
				}
			case uint8(TraceLevel):
				if len(tt.printf) == 0 {
					l.Trace(tt.msg).End()
				} else {
					l.Tracef(tt.printf, tt.msg).End()
				}
			case uint8(InfoLevel):
				if len(tt.printf) == 0 {
					l.Info(tt.msg)
				} else {
					l.Infof(tt.printf, tt.msg)
				}
			case uint8(NoticeLevel):
				if len(tt.printf) == 0 {
					l.Notice(tt.msg)
				} else {
					l.Noticef(tt.printf, tt.msg)
				}
			case uint8(WarnLevel):
				if len(tt.printf) == 0 {
					l.Warn(tt.msg)
				} else {
					l.Warnf(tt.printf, tt.msg)
				}
			case uint8(ErrorLevel):
				if len(tt.printf) == 0 {
					l.Error(tt.msg)
				} else {
					l.Errorf(tt.printf, tt.msg)
				}
			case uint8(PanicLevel):
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
			case uint8(AlertLevel):
				if len(tt.printf) == 0 {
					l.Alert(tt.msg)
				} else {
					l.Alertf(tt.printf, tt.msg)
				}

			case 100:
				Println(tt.msg)
			case 101:
				Printf(tt.printf, tt.msg)
			case 102:
				Print(tt.msg)
			}

		} else {

			switch tt.lvl {
			case uint8(DebugLevel):
				if len(tt.printf) == 0 {
					Debug(tt.msg)
				} else {
					Debugf(tt.printf, tt.msg)
				}
			case uint8(TraceLevel):
				if len(tt.printf) == 0 {
					Trace(tt.msg).End()
				} else {
					Tracef(tt.printf, tt.msg).End()
				}
			case uint8(InfoLevel):
				if len(tt.printf) == 0 {
					Info(tt.msg)
				} else {
					Infof(tt.printf, tt.msg)
				}
			case uint8(NoticeLevel):
				if len(tt.printf) == 0 {
					Notice(tt.msg)
				} else {
					Noticef(tt.printf, tt.msg)
				}
			case uint8(WarnLevel):
				if len(tt.printf) == 0 {
					Warn(tt.msg)
				} else {
					Warnf(tt.printf, tt.msg)
				}
			case uint8(ErrorLevel):
				if len(tt.printf) == 0 {
					Error(tt.msg)
				} else {
					Errorf(tt.printf, tt.msg)
				}
			case uint8(PanicLevel):
				func() {
					defer func() {
						recover()
					}()

					if len(tt.printf) == 0 {
						Panic(tt.msg)
					} else {
						Panicf(tt.printf, tt.msg)
					}
				}()
			case uint8(AlertLevel):
				if len(tt.printf) == 0 {
					Alert(tt.msg)
				} else {
					Alertf(tt.printf, tt.msg)
				}

			case 100:
				Println(tt.msg)
			case 101:
				Printf(tt.printf, tt.msg)
			case 102:
				Print(tt.msg)
			}
		}

		if buff.String() != tt.want {

			if tt.lvl == uint8(TraceLevel) {
				if !strings.HasPrefix(buff.String(), tt.want) {
					t.Errorf("test %d: Expected '%s' Got '%s'", i, tt.want, buff.String())
				}
				continue
			}

			t.Errorf("test %d: Expected '%s' Got '%s'", i, tt.want, buff.String())
		}
	}

	buff.Reset()
	// Test Custom Entry ( most common case is Unmarshalled from JSON when using centralized logging)
	entry := new(Entry)
	entry.ApplicationID = "APP"
	entry.Level = InfoLevel
	entry.Timestamp = time.Now().UTC()
	entry.Message = "Test Message"
	entry.Fields = make([]Field, 0)
	Logger.HandleEntry(entry)

	if buff.String() != "INFO Test Message\n" {
		t.Errorf("test Custom Entry: Expected '%s' Got '%s'", "INFO Test Message\n", buff.String())
	}
}

func TestConsoleLoggerCaller2(t *testing.T) {

	tests := getLogCallerTests()
	buff := new(bytes.Buffer)
	SetCallerInfoLevels(AllLevels...)
	SetCallerSkipDiff(0)

	th := &testHandler{
		writer: buff,
	}

	RegisterHandler(th, AllLevels...)

	if bl := HasHandlers(); !bl {
		t.Errorf("test HasHandlers: Expected '%t' Got '%t'", true, bl)
	}

	for i, tt := range tests {

		buff.Reset()
		var l LeveledLogger

		if tt.flds != nil {
			l = WithFields(tt.flds...)

			switch tt.lvl {
			case uint8(DebugLevel):
				if len(tt.printf) == 0 {
					l.Debug(tt.msg)
				} else {
					l.Debugf(tt.printf, tt.msg)
				}
			case uint8(TraceLevel):
				if len(tt.printf) == 0 {
					l.Trace(tt.msg).End()
				} else {
					l.Tracef(tt.printf, tt.msg).End()
				}
			case uint8(InfoLevel):
				if len(tt.printf) == 0 {
					l.Info(tt.msg)
				} else {
					l.Infof(tt.printf, tt.msg)
				}
			case uint8(NoticeLevel):
				if len(tt.printf) == 0 {
					l.Notice(tt.msg)
				} else {
					l.Noticef(tt.printf, tt.msg)
				}
			case uint8(WarnLevel):
				if len(tt.printf) == 0 {
					l.Warn(tt.msg)
				} else {
					l.Warnf(tt.printf, tt.msg)
				}
			case uint8(ErrorLevel):
				if len(tt.printf) == 0 {
					l.Error(tt.msg)
				} else {
					l.Errorf(tt.printf, tt.msg)
				}
			case uint8(PanicLevel):
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
			case uint8(AlertLevel):
				if len(tt.printf) == 0 {
					l.Alert(tt.msg)
				} else {
					l.Alertf(tt.printf, tt.msg)
				}

			case 100:
				Println(tt.msg)
			case 101:
				Printf(tt.printf, tt.msg)
			case 102:
				Print(tt.msg)
			}

		} else {

			switch tt.lvl {
			case uint8(DebugLevel):
				if len(tt.printf) == 0 {
					Debug(tt.msg)
				} else {
					Debugf(tt.printf, tt.msg)
				}
			case uint8(TraceLevel):
				if len(tt.printf) == 0 {
					Trace(tt.msg).End()
				} else {
					Tracef(tt.printf, tt.msg).End()
				}
			case uint8(InfoLevel):
				if len(tt.printf) == 0 {
					Info(tt.msg)
				} else {
					Infof(tt.printf, tt.msg)
				}
			case uint8(NoticeLevel):
				if len(tt.printf) == 0 {
					Notice(tt.msg)
				} else {
					Noticef(tt.printf, tt.msg)
				}
			case uint8(WarnLevel):
				if len(tt.printf) == 0 {
					Warn(tt.msg)
				} else {
					Warnf(tt.printf, tt.msg)
				}
			case uint8(ErrorLevel):
				if len(tt.printf) == 0 {
					Error(tt.msg)
				} else {
					Errorf(tt.printf, tt.msg)
				}
			case uint8(PanicLevel):
				func() {
					defer func() {
						recover()
					}()

					if len(tt.printf) == 0 {
						Panic(tt.msg)
					} else {
						Panicf(tt.printf, tt.msg)
					}
				}()
			case uint8(AlertLevel):
				if len(tt.printf) == 0 {
					Alert(tt.msg)
				} else {
					Alertf(tt.printf, tt.msg)
				}

			case 100:
				Println(tt.msg)
			case 101:
				Printf(tt.printf, tt.msg)
			case 102:
				Print(tt.msg)
			}
		}

		if buff.String() != tt.want {

			if tt.lvl == uint8(TraceLevel) {
				if !strings.HasPrefix(buff.String(), tt.want) {
					t.Errorf("test %d: Expected '%s' Got '%s'", i, tt.want, buff.String())
				}
				continue
			}

			t.Errorf("test %d: Expected '%s' Got '%s'", i, tt.want, buff.String())
		}
	}

	buff.Reset()
	func() {
		defer func() {
			recover()
		}()

		Panicln("panicln")
	}()

	if buff.String() != "PANIC pkg_test.go:382 panicln\n" {
		t.Errorf("test panicln: Expected '%s' Got '%s'", "PANIC pkg_test.go:382 panicln\n", buff.String())
	}

	buff.Reset()
	// Test Custom Entry ( most common case is Unmarshalled from JSON when using centralized logging)
	entry := new(Entry)
	entry.ApplicationID = "APP"
	entry.Level = InfoLevel
	entry.Timestamp = time.Now().UTC()
	entry.Message = "Test Message"
	entry.Fields = make([]Field, 0)
	entry.Line = 399
	entry.File = "pkg_test.go"
	Logger.HandleEntry(entry)

	if buff.String() != "INFO pkg_test.go:399 Test Message\n" {
		t.Errorf("test Custom Entry: Expected '%s' Got '%s'", "INFO pkg_test.go:399 Test Message\n", buff.String())
	}

	buff.Reset()
	StackTrace().Debug()

	expected := "DEBUG pkg_test.go:406  stack trace="
	if !strings.HasPrefix(buff.String(), expected) {
		t.Errorf("Expected Prefix '%s' Got '%s'", expected, buff.String())
	}

	buff.Reset()
	StackTrace().WithFields(F("key", "value")).Debug()

	expected = "DEBUG pkg_test.go:414  stack trace="
	if !strings.HasPrefix(buff.String(), expected) {
		t.Errorf("Expected Prefix '%s' Got '%s'", expected, buff.String())
	}
}

func TestFatal2(t *testing.T) {
	var i int

	exitFunc = func(code int) {
		i = code
	}

	Fatal("fatal")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}

	Fatalf("fatalf")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}

	Fatalln("fatalln")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}

	WithFields(F("key", "value")).Fatal("fatal")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}

	WithFields(F("key", "value")).Fatalf("fatalf")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}
}

func TestSettings2(t *testing.T) {
	RegisterDurationFunc(func(d time.Duration) string {
		return fmt.Sprintf("%gs", d.Seconds())
	})

	SetTimeFormat(time.RFC1123)
}

func TestEntry2(t *testing.T) {

	SetApplicationID("app-log")

	// Resetting pool to ensure no Entries exist before setting the Application ID
	Logger.entryPool = &sync.Pool{New: func() interface{} {
		return &Entry{
			wg:            new(sync.WaitGroup),
			ApplicationID: Logger.getApplicationID(),
		}
	}}

	e := Logger.entryPool.Get().(*Entry)

	if e.ApplicationID != "app-log" {
		t.Errorf("test Fatals: Expected '%s' Got '%s'", "app-log", e.ApplicationID)
	}

	if e.wg == nil {
		t.Errorf("test Fatals: Expected '%v' Got '%v'", "not nil", e.wg)
	}

	e = newEntry(InfoLevel, "test", []Field{F("key", "value")}, 0)
	HandleEntry(e)
}

type testHandler struct {
	writer io.Writer
}

// Run runs handler
func (th *testHandler) Run() chan<- *Entry {
	ch := make(chan *Entry, 0)

	go th.handleLogEntry(ch)

	return ch
}

func (th *testHandler) handleLogEntry(entries <-chan *Entry) {

	var e *Entry
	var file string

	for e = range entries {
		s := e.Level.String() + " "
		file = ""

		if e.Line != 0 {

			file = e.File

			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/' {
					file = file[i+1:]
					break
				}
			}

			s += file + ":" + fmt.Sprintf("%d", e.Line) + " "
		}

		s += e.Message

		for _, f := range e.Fields {
			s += fmt.Sprintf(" %s=%v", f.Key, f.Value)
		}

		s += "\n"

		if _, err := th.writer.Write([]byte(s)); err != nil {
			panic(err)
		}

		e.Consumed()
	}
}

type test struct {
	lvl    uint8
	msg    string
	flds   []Field
	want   string
	printf string
}

func getLogTests() []test {
	return []test{
		{
			lvl:  uint8(102),
			msg:  "print",
			flds: nil,
			want: "INFO print\n",
		},
		{
			lvl:    uint8(101),
			msg:    "printf",
			printf: "%s",
			flds:   nil,
			want:   "INFO printf\n",
		},
		{
			lvl:  uint8(100),
			msg:  "println",
			flds: nil,
			want: "INFO println\n",
		},
		{
			lvl:  uint8(PanicLevel),
			msg:  "panicln",
			flds: nil,
			want: "PANIC pkg_test.go:158 panicln\n",
		},
		{
			lvl:  uint8(DebugLevel),
			msg:  "debug",
			flds: nil,
			want: "DEBUG debug\n",
		},
		{
			lvl:    uint8(DebugLevel),
			msg:    "debugf",
			printf: "%s",
			flds:   nil,
			want:   "DEBUG debugf\n",
		},
		{
			lvl:  uint8(InfoLevel),
			msg:  "info",
			flds: nil,
			want: "INFO info\n",
		},
		{
			lvl:    uint8(InfoLevel),
			msg:    "infof",
			printf: "%s",
			flds:   nil,
			want:   "INFO infof\n",
		},
		{
			lvl:  uint8(NoticeLevel),
			msg:  "notice",
			flds: nil,
			want: "NOTICE notice\n",
		},
		{
			lvl:    uint8(NoticeLevel),
			msg:    "noticef",
			printf: "%s",
			flds:   nil,
			want:   "NOTICE noticef\n",
		},
		{
			lvl:  uint8(WarnLevel),
			msg:  "warn",
			flds: nil,
			want: "WARN pkg_test.go:141 warn\n",
		},
		{
			lvl:    uint8(WarnLevel),
			msg:    "warnf",
			printf: "%s",
			flds:   nil,
			want:   "WARN pkg_test.go:143 warnf\n",
		},
		{
			lvl:  uint8(ErrorLevel),
			msg:  "error",
			flds: nil,
			want: "ERROR pkg_test.go:147 error\n",
		},
		{
			lvl:    uint8(ErrorLevel),
			msg:    "errorf",
			printf: "%s",
			flds:   nil,
			want:   "ERROR pkg_test.go:149 errorf\n",
		},
		{
			lvl:  uint8(AlertLevel),
			msg:  "alert",
			flds: nil,
			want: "ALERT pkg_test.go:165 alert\n",
		},
		{
			lvl:    uint8(AlertLevel),
			msg:    "alertf",
			printf: "%s",
			flds:   nil,
			want:   "ALERT pkg_test.go:167 alertf\n",
		},
		{
			lvl:  uint8(PanicLevel),
			msg:  "panic",
			flds: nil,
			want: "PANIC pkg_test.go:158 panic\n",
		},
		{
			lvl:    uint8(PanicLevel),
			msg:    "panicf",
			printf: "%s",
			flds:   nil,
			want:   "PANIC pkg_test.go:160 panicf\n",
		},
		{
			lvl: uint8(DebugLevel),
			msg: "debug",
			flds: []Field{
				F("key", "value"),
			},
			want: "DEBUG debug key=value\n",
		},
		{
			lvl:    uint8(DebugLevel),
			msg:    "debugf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "DEBUG debugf key=value\n",
		},
		{
			lvl: uint8(InfoLevel),
			msg: "info",
			flds: []Field{
				F("key", "value"),
			},
			want: "INFO info key=value\n",
		},
		{
			lvl:    uint8(InfoLevel),
			msg:    "infof",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "INFO infof key=value\n",
		},
		{
			lvl: uint8(NoticeLevel),
			msg: "notice",
			flds: []Field{
				F("key", "value"),
			},
			want: "NOTICE notice key=value\n",
		},
		{
			lvl:    uint8(NoticeLevel),
			msg:    "noticef",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "NOTICE noticef key=value\n",
		},
		{
			lvl: uint8(WarnLevel),
			msg: "warn",
			flds: []Field{
				F("key", "value"),
			},
			want: "WARN pkg_test.go:75 warn key=value\n",
		},
		{
			lvl:    uint8(WarnLevel),
			msg:    "warnf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "WARN pkg_test.go:77 warnf key=value\n",
		},
		{
			lvl: uint8(ErrorLevel),
			msg: "error",
			flds: []Field{
				F("key", "value"),
			},
			want: "ERROR pkg_test.go:81 error key=value\n",
		},
		{
			lvl:    uint8(ErrorLevel),
			msg:    "errorf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "ERROR pkg_test.go:83 errorf key=value\n",
		},
		{
			lvl: uint8(AlertLevel),
			msg: "alert",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT pkg_test.go:99 alert key=value\n",
		},
		{
			lvl: uint8(AlertLevel),
			msg: "alert",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT pkg_test.go:99 alert key=value\n",
		},
		{
			lvl:    uint8(AlertLevel),
			msg:    "alertf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT pkg_test.go:101 alertf key=value\n",
		},
		{
			lvl:    uint8(PanicLevel),
			msg:    "panicf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "PANIC pkg_test.go:94 panicf key=value\n",
		},
		{
			lvl: uint8(PanicLevel),
			msg: "panic",
			flds: []Field{
				F("key", "value"),
			},
			want: "PANIC pkg_test.go:92 panic key=value\n",
		},
		{
			lvl:  uint8(TraceLevel),
			msg:  "trace",
			flds: nil,
			want: "TRACE trace",
		},
		{
			lvl:    uint8(TraceLevel),
			msg:    "tracef",
			printf: "%s",
			flds:   nil,
			want:   "TRACE tracef",
		},
		{
			lvl: uint8(TraceLevel),
			msg: "trace",
			flds: []Field{
				F("key", "value"),
			},
			want: "TRACE trace key=value",
		},
		{
			lvl:    uint8(TraceLevel),
			msg:    "tracef",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "TRACE tracef key=value",
		},
		{
			lvl: uint8(DebugLevel),
			msg: "debug",
			flds: []Field{
				F("key", "string"),
				F("key", int(1)),
				F("key", int8(2)),
				F("key", int16(3)),
				F("key", int32(4)),
				F("key", int64(5)),
				F("key", uint(1)),
				F("key", uint8(2)),
				F("key", uint16(3)),
				F("key", uint32(4)),
				F("key", uint64(5)),
				F("key", true),
				F("key", struct{ value string }{"struct"}),
			},
			want: "DEBUG debug key=string key=1 key=2 key=3 key=4 key=5 key=1 key=2 key=3 key=4 key=5 key=true key={struct}\n",
		},
	}
}

func getLogCallerTests() []test {
	return []test{
		{
			lvl:  uint8(102),
			msg:  "print",
			flds: nil,
			want: "INFO pkg_test.go:359 print\n",
		},
		{
			lvl:    uint8(101),
			msg:    "printf",
			printf: "%s",
			flds:   nil,
			want:   "INFO pkg_test.go:357 printf\n",
		},
		{
			lvl:  uint8(100),
			msg:  "println",
			flds: nil,
			want: "INFO pkg_test.go:355 println\n",
		},
		{
			lvl:  uint8(PanicLevel),
			msg:  "panicln",
			flds: nil,
			want: "PANIC pkg_test.go:342 panicln\n",
		},
		{
			lvl:  uint8(DebugLevel),
			msg:  "debug",
			flds: nil,
			want: "DEBUG pkg_test.go:301 debug\n",
		},
		{
			lvl:    uint8(DebugLevel),
			msg:    "debugf",
			printf: "%s",
			flds:   nil,
			want:   "DEBUG pkg_test.go:303 debugf\n",
		},
		{
			lvl:  uint8(InfoLevel),
			msg:  "info",
			flds: nil,
			want: "INFO pkg_test.go:313 info\n",
		},
		{
			lvl:    uint8(InfoLevel),
			msg:    "infof",
			printf: "%s",
			flds:   nil,
			want:   "INFO pkg_test.go:315 infof\n",
		},
		{
			lvl:  uint8(NoticeLevel),
			msg:  "notice",
			flds: nil,
			want: "NOTICE pkg_test.go:319 notice\n",
		},
		{
			lvl:    uint8(NoticeLevel),
			msg:    "noticef",
			printf: "%s",
			flds:   nil,
			want:   "NOTICE pkg_test.go:321 noticef\n",
		},
		{
			lvl:  uint8(WarnLevel),
			msg:  "warn",
			flds: nil,
			want: "WARN pkg_test.go:325 warn\n",
		},
		{
			lvl:    uint8(WarnLevel),
			msg:    "warnf",
			printf: "%s",
			flds:   nil,
			want:   "WARN pkg_test.go:327 warnf\n",
		},
		{
			lvl:  uint8(ErrorLevel),
			msg:  "error",
			flds: nil,
			want: "ERROR pkg_test.go:331 error\n",
		},
		{
			lvl:    uint8(ErrorLevel),
			msg:    "errorf",
			printf: "%s",
			flds:   nil,
			want:   "ERROR pkg_test.go:333 errorf\n",
		},
		{
			lvl:  uint8(AlertLevel),
			msg:  "alert",
			flds: nil,
			want: "ALERT pkg_test.go:349 alert\n",
		},
		{
			lvl:    uint8(AlertLevel),
			msg:    "alertf",
			printf: "%s",
			flds:   nil,
			want:   "ALERT pkg_test.go:351 alertf\n",
		},
		{
			lvl:  uint8(PanicLevel),
			msg:  "panic",
			flds: nil,
			want: "PANIC pkg_test.go:342 panic\n",
		},
		{
			lvl:    uint8(PanicLevel),
			msg:    "panicf",
			printf: "%s",
			flds:   nil,
			want:   "PANIC pkg_test.go:344 panicf\n",
		},
		{
			lvl: uint8(DebugLevel),
			msg: "debug",
			flds: []Field{
				F("key", "value"),
			},
			want: "DEBUG pkg_test.go:235 debug key=value\n",
		},
		{
			lvl:    uint8(DebugLevel),
			msg:    "debugf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "DEBUG pkg_test.go:237 debugf key=value\n",
		},
		{
			lvl: uint8(InfoLevel),
			msg: "info",
			flds: []Field{
				F("key", "value"),
			},
			want: "INFO pkg_test.go:247 info key=value\n",
		},
		{
			lvl:    uint8(InfoLevel),
			msg:    "infof",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "INFO pkg_test.go:249 infof key=value\n",
		},
		{
			lvl: uint8(NoticeLevel),
			msg: "notice",
			flds: []Field{
				F("key", "value"),
			},
			want: "NOTICE pkg_test.go:253 notice key=value\n",
		},
		{
			lvl:    uint8(NoticeLevel),
			msg:    "noticef",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "NOTICE pkg_test.go:255 noticef key=value\n",
		},
		{
			lvl: uint8(WarnLevel),
			msg: "warn",
			flds: []Field{
				F("key", "value"),
			},
			want: "WARN pkg_test.go:259 warn key=value\n",
		},
		{
			lvl:    uint8(WarnLevel),
			msg:    "warnf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "WARN pkg_test.go:261 warnf key=value\n",
		},
		{
			lvl: uint8(ErrorLevel),
			msg: "error",
			flds: []Field{
				F("key", "value"),
			},
			want: "ERROR pkg_test.go:265 error key=value\n",
		},
		{
			lvl:    uint8(ErrorLevel),
			msg:    "errorf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "ERROR pkg_test.go:267 errorf key=value\n",
		},
		{
			lvl: uint8(AlertLevel),
			msg: "alert",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT pkg_test.go:283 alert key=value\n",
		},
		{
			lvl: uint8(AlertLevel),
			msg: "alert",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT pkg_test.go:283 alert key=value\n",
		},
		{
			lvl:    uint8(AlertLevel),
			msg:    "alertf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT pkg_test.go:285 alertf key=value\n",
		},
		{
			lvl:    uint8(PanicLevel),
			msg:    "panicf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "PANIC pkg_test.go:278 panicf key=value\n",
		},
		{
			lvl: uint8(PanicLevel),
			msg: "panic",
			flds: []Field{
				F("key", "value"),
			},
			want: "PANIC pkg_test.go:276 panic key=value\n",
		},
		{
			lvl:  uint8(TraceLevel),
			msg:  "trace",
			flds: nil,
			want: "TRACE pkg_test.go:307 trace ",
		},
		{
			lvl:    uint8(TraceLevel),
			msg:    "tracef",
			printf: "%s",
			flds:   nil,
			want:   "TRACE pkg_test.go:309 tracef ",
		},
		{
			lvl: uint8(TraceLevel),
			msg: "trace",
			flds: []Field{
				F("key", "value"),
			},
			want: "TRACE pkg_test.go:241 trace ",
		},
		{
			lvl:    uint8(TraceLevel),
			msg:    "tracef",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "TRACE pkg_test.go:243 tracef ",
		},
		{
			lvl: uint8(DebugLevel),
			msg: "debug",
			flds: []Field{
				F("key", "string"),
				F("key", int(1)),
				F("key", int8(2)),
				F("key", int16(3)),
				F("key", int32(4)),
				F("key", int64(5)),
				F("key", uint(1)),
				F("key", uint8(2)),
				F("key", uint16(3)),
				F("key", uint32(4)),
				F("key", uint64(5)),
				F("key", true),
				F("key", struct{ value string }{"struct"}),
			},
			want: "DEBUG pkg_test.go:235 debug key=string key=1 key=2 key=3 key=4 key=5 key=1 key=2 key=3 key=4 key=5 key=true key={struct}\n",
		},
	}
}
