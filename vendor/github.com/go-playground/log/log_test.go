package log

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-playground/ansi"
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

func TestConsoleLogger1(t *testing.T) {
	stackTraceLimit = 1000
	tests := getLogTests1()
	buff := new(bytes.Buffer)
	th := &testHandler{
		writer: buff,
	}
	Logger.SetCallerInfoLevels(WarnLevel, ErrorLevel, PanicLevel, AlertLevel, FatalLevel)
	Logger.RegisterHandler(th, AllLevels...)

	if bl := Logger.HasHandlers(); !bl {
		t.Errorf("test HasHandlers: Expected '%t' Got '%t'", true, bl)
	}

	for i, tt := range tests {

		buff.Reset()
		var l LeveledLogger

		if tt.flds != nil {
			l = Logger.WithFields(tt.flds...)
		} else {
			l = Logger
		}

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

func TestConsoleLoggerCaller1(t *testing.T) {

	tests := getLogCallerTests1()

	buff := new(bytes.Buffer)
	Logger.SetCallerInfoLevels(AllLevels...)
	Logger.SetCallerSkipDiff(0)

	th := &testHandler{
		writer: buff,
	}

	Logger.RegisterHandler(th, AllLevels...)

	if bl := Logger.HasHandlers(); !bl {
		t.Errorf("test HasHandlers: Expected '%t' Got '%t'", true, bl)
	}

	for i, tt := range tests {

		buff.Reset()
		var l LeveledLogger

		if tt.flds != nil {
			l = Logger.WithFields(tt.flds...)
		} else {
			l = Logger
		}

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
	entry.Line = 256
	entry.File = "log_test.go"
	Logger.HandleEntry(entry)

	if buff.String() != "INFO log_test.go:256 Test Message\n" {
		t.Errorf("test Custom Entry: Expected '%s' Got '%s'", "INFO log_test.go:256 Test Message\n", buff.String())
	}

	buff.Reset()
	Logger.StackTrace().Debug()

	expected := "DEBUG log_test.go:265  stack trace="
	if !strings.HasPrefix(buff.String(), expected) {
		t.Errorf("Expected Prefix '%s' Got '%s'", expected, buff.String())
	}

	buff.Reset()
	Logger.WithFields(Logger.F("key", "value")).StackTrace().Debug()

	expected = "DEBUG log_test.go:273  key=value stack trace="
	if !strings.HasPrefix(buff.String(), expected) {
		t.Errorf("Expected Prefix '%s' Got '%s'", expected, buff.String())
	}
}

func TestLevel(t *testing.T) {

	tests := []struct {
		value string
		want  string
	}{
		{
			value: Level(255).String(),
			want:  "Unknow Level",
		},
		{
			value: DebugLevel.String(),
			want:  "DEBUG",
		},
		{
			value: TraceLevel.String(),
			want:  "TRACE",
		},
		{
			value: InfoLevel.String(),
			want:  "INFO",
		},
		{
			value: NoticeLevel.String(),
			want:  "NOTICE",
		},
		{
			value: WarnLevel.String(),
			want:  "WARN",
		},
		{
			value: ErrorLevel.String(),
			want:  "ERROR",
		},
		{
			value: PanicLevel.String(),
			want:  "PANIC",
		},
		{
			value: AlertLevel.String(),
			want:  "ALERT",
		},
		{
			value: FatalLevel.String(),
			want:  "FATAL",
		},
	}

	for i, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("Test %d: Expected '%s' Got '%s'", i, tt.want, tt.value)
		}
	}
}

func TestSettings(t *testing.T) {
	Logger.RegisterDurationFunc(func(d time.Duration) string {
		return fmt.Sprintf("%gs", d.Seconds())
	})

	Logger.SetTimeFormat(time.RFC1123)
}

func TestEntry(t *testing.T) {

	Logger.SetApplicationID("app-log")

	// Resetting pool to ensure no Entries exist before setting the Application ID
	Logger.entryPool = &sync.Pool{New: func() interface{} {
		return &Entry{
			wg:            new(sync.WaitGroup),
			ApplicationID: Logger.getApplicationID(),
		}
	}}

	e := Logger.entryPool.Get().(*Entry)
	if e.ApplicationID != "app-log" {
		t.Errorf("Test App ID: Expected '%s' Got '%s'", "app-log", e.ApplicationID)
	}

	if e.wg == nil {
		t.Errorf("Test WaitGroup: Expected '%s' Got '%v'", "Not Nil", e.wg)
	}

	e = newEntry(InfoLevel, "test", []Field{F("key", "value")}, 0)
	Logger.HandleEntry(e)
}

func TestFatal(t *testing.T) {
	var i int

	Logger.SetExitFunc(func(code int) {
		i = code
	})

	Logger.Fatal("fatal")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}

	Logger.Fatalf("fatalf")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}

	Logger.WithFields(F("key", "value")).Fatal("fatal")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}

	Logger.WithFields(F("key", "value")).Fatalf("fatalf")
	if i != 1 {
		t.Errorf("test Fatals: Expected '%d' Got '%d'", 1, i)
	}
}

func TestColors(t *testing.T) {

	fmt.Printf("%sBlack%s\n", ansi.Black, ansi.Reset)
	fmt.Printf("%sDarkGray%s\n", ansi.DarkGray, ansi.Reset)
	fmt.Printf("%sBlue%s\n", ansi.Blue, ansi.Reset)
	fmt.Printf("%sLightBlue%s\n", ansi.LightBlue, ansi.Reset)
	fmt.Printf("%sGreen%s\n", ansi.Green, ansi.Reset)
	fmt.Printf("%sLightGreen%s\n", ansi.LightGreen, ansi.Reset)
	fmt.Printf("%sCyan%s\n", ansi.Cyan, ansi.Reset)
	fmt.Printf("%sLightCyan%s\n", ansi.LightCyan, ansi.Reset)
	fmt.Printf("%sRed%s\n", ansi.Red, ansi.Reset)
	fmt.Printf("%sLightRed%s\n", ansi.LightRed, ansi.Reset)
	fmt.Printf("%sMagenta%s\n", ansi.Magenta, ansi.Reset)
	fmt.Printf("%sLightMagenta%s\n", ansi.LightMagenta, ansi.Reset)
	fmt.Printf("%Yellow%s\n", ansi.Yellow, ansi.Reset)
	fmt.Printf("%sLightYellow%s\n", ansi.LightYellow, ansi.Reset)
	fmt.Printf("%sGray%s\n", ansi.Gray, ansi.Reset)
	fmt.Printf("%sWhite%s\n", ansi.White, ansi.Reset)

	fmt.Printf("%s%sUnderscoreRed%s\n", ansi.Red, ansi.Underline, ansi.Reset)
	fmt.Printf("%s%sBlinkRed%s\n", ansi.Red, ansi.Blink, ansi.Reset)
	fmt.Printf("%s%s%sBlinkUnderscoreRed%s\n", ansi.Red, ansi.Blink, ansi.Underline, ansi.Reset)

	fmt.Printf("%s%sRedInverse%s\n", ansi.Red, ansi.Inverse, ansi.Reset)
	fmt.Printf("%sGreenInverse%s\n", ansi.Green+ansi.Inverse, ansi.Reset)
}

func getLogTests1() []test {
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
			want: "PANIC log_test.go:94 panicln\n",
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
			want: "WARN log_test.go:77 warn\n",
		},
		{
			lvl:    uint8(WarnLevel),
			msg:    "warnf",
			printf: "%s",
			flds:   nil,
			want:   "WARN log_test.go:79 warnf\n",
		},
		{
			lvl:  uint8(ErrorLevel),
			msg:  "error",
			flds: nil,
			want: "ERROR log_test.go:83 error\n",
		},
		{
			lvl:    uint8(ErrorLevel),
			msg:    "errorf",
			printf: "%s",
			flds:   nil,
			want:   "ERROR log_test.go:85 errorf\n",
		},
		{
			lvl:  uint8(AlertLevel),
			msg:  "alert",
			flds: nil,
			want: "ALERT log_test.go:101 alert\n",
		},
		{
			lvl:    uint8(AlertLevel),
			msg:    "alertf",
			printf: "%s",
			flds:   nil,
			want:   "ALERT log_test.go:103 alertf\n",
		},
		{
			lvl:  uint8(PanicLevel),
			msg:  "panic",
			flds: nil,
			want: "PANIC log_test.go:94 panic\n",
		},
		{
			lvl:    uint8(PanicLevel),
			msg:    "panicf",
			printf: "%s",
			flds:   nil,
			want:   "PANIC log_test.go:96 panicf\n",
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
			want: "WARN log_test.go:77 warn key=value\n",
		},
		{
			lvl:    uint8(WarnLevel),
			msg:    "warnf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "WARN log_test.go:79 warnf key=value\n",
		},
		{
			lvl: uint8(ErrorLevel),
			msg: "error",
			flds: []Field{
				F("key", "value"),
			},
			want: "ERROR log_test.go:83 error key=value\n",
		},
		{
			lvl:    uint8(ErrorLevel),
			msg:    "errorf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "ERROR log_test.go:85 errorf key=value\n",
		},
		{
			lvl: uint8(AlertLevel),
			msg: "alert",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT log_test.go:101 alert key=value\n",
		},
		{
			lvl: uint8(AlertLevel),
			msg: "alert",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT log_test.go:101 alert key=value\n",
		},
		{
			lvl:    uint8(AlertLevel),
			msg:    "alertf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT log_test.go:103 alertf key=value\n",
		},
		{
			lvl:    uint8(PanicLevel),
			msg:    "panicf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "PANIC log_test.go:96 panicf key=value\n",
		},
		{
			lvl: uint8(PanicLevel),
			msg: "panic",
			flds: []Field{
				F("key", "value"),
			},
			want: "PANIC log_test.go:94 panic key=value\n",
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

func getLogCallerTests1() []test {
	return []test{
		{
			lvl:  uint8(102),
			msg:  "print",
			flds: nil,
			want: "INFO log_test.go:232 print\n",
		},
		{
			lvl:    uint8(101),
			msg:    "printf",
			printf: "%s",
			flds:   nil,
			want:   "INFO log_test.go:230 printf\n",
		},
		{
			lvl:  uint8(100),
			msg:  "println",
			flds: nil,
			want: "INFO log_test.go:228 println\n",
		},
		{
			lvl:  uint8(PanicLevel),
			msg:  "panicln",
			flds: nil,
			want: "PANIC log_test.go:215 panicln\n",
		},
		{
			lvl:  uint8(DebugLevel),
			msg:  "debug",
			flds: nil,
			want: "DEBUG log_test.go:174 debug\n",
		},
		{
			lvl:    uint8(DebugLevel),
			msg:    "debugf",
			printf: "%s",
			flds:   nil,
			want:   "DEBUG log_test.go:176 debugf\n",
		},
		{
			lvl:  uint8(InfoLevel),
			msg:  "info",
			flds: nil,
			want: "INFO log_test.go:186 info\n",
		},
		{
			lvl:    uint8(InfoLevel),
			msg:    "infof",
			printf: "%s",
			flds:   nil,
			want:   "INFO log_test.go:188 infof\n",
		},
		{
			lvl:  uint8(NoticeLevel),
			msg:  "notice",
			flds: nil,
			want: "NOTICE log_test.go:192 notice\n",
		},
		{
			lvl:    uint8(NoticeLevel),
			msg:    "noticef",
			printf: "%s",
			flds:   nil,
			want:   "NOTICE log_test.go:194 noticef\n",
		},
		{
			lvl:  uint8(WarnLevel),
			msg:  "warn",
			flds: nil,
			want: "WARN log_test.go:198 warn\n",
		},
		{
			lvl:    uint8(WarnLevel),
			msg:    "warnf",
			printf: "%s",
			flds:   nil,
			want:   "WARN log_test.go:200 warnf\n",
		},
		{
			lvl:  uint8(ErrorLevel),
			msg:  "error",
			flds: nil,
			want: "ERROR log_test.go:204 error\n",
		},
		{
			lvl:    uint8(ErrorLevel),
			msg:    "errorf",
			printf: "%s",
			flds:   nil,
			want:   "ERROR log_test.go:206 errorf\n",
		},
		{
			lvl:  uint8(AlertLevel),
			msg:  "alert",
			flds: nil,
			want: "ALERT log_test.go:222 alert\n",
		},
		{
			lvl:    uint8(AlertLevel),
			msg:    "alertf",
			printf: "%s",
			flds:   nil,
			want:   "ALERT log_test.go:224 alertf\n",
		},
		{
			lvl:  uint8(PanicLevel),
			msg:  "panic",
			flds: nil,
			want: "PANIC log_test.go:215 panic\n",
		},
		{
			lvl:    uint8(PanicLevel),
			msg:    "panicf",
			printf: "%s",
			flds:   nil,
			want:   "PANIC log_test.go:217 panicf\n",
		},
		{
			lvl: uint8(DebugLevel),
			msg: "debug",
			flds: []Field{
				F("key", "value"),
			},
			want: "DEBUG log_test.go:174 debug key=value\n",
		},
		{
			lvl:    uint8(DebugLevel),
			msg:    "debugf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "DEBUG log_test.go:176 debugf key=value\n",
		},
		{
			lvl: uint8(InfoLevel),
			msg: "info",
			flds: []Field{
				F("key", "value"),
			},
			want: "INFO log_test.go:186 info key=value\n",
		},
		{
			lvl:    uint8(InfoLevel),
			msg:    "infof",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "INFO log_test.go:188 infof key=value\n",
		},
		{
			lvl: uint8(NoticeLevel),
			msg: "notice",
			flds: []Field{
				F("key", "value"),
			},
			want: "NOTICE log_test.go:192 notice key=value\n",
		},
		{
			lvl:    uint8(NoticeLevel),
			msg:    "noticef",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "NOTICE log_test.go:194 noticef key=value\n",
		},
		{
			lvl: uint8(WarnLevel),
			msg: "warn",
			flds: []Field{
				F("key", "value"),
			},
			want: "WARN log_test.go:198 warn key=value\n",
		},
		{
			lvl:    uint8(WarnLevel),
			msg:    "warnf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "WARN log_test.go:200 warnf key=value\n",
		},
		{
			lvl: uint8(ErrorLevel),
			msg: "error",
			flds: []Field{
				F("key", "value"),
			},
			want: "ERROR log_test.go:204 error key=value\n",
		},
		{
			lvl:    uint8(ErrorLevel),
			msg:    "errorf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "ERROR log_test.go:206 errorf key=value\n",
		},
		{
			lvl: uint8(AlertLevel),
			msg: "alert",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT log_test.go:222 alert key=value\n",
		},
		{
			lvl: uint8(AlertLevel),
			msg: "alert",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT log_test.go:222 alert key=value\n",
		},
		{
			lvl:    uint8(AlertLevel),
			msg:    "alertf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "ALERT log_test.go:224 alertf key=value\n",
		},
		{
			lvl:    uint8(PanicLevel),
			msg:    "panicf",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "PANIC log_test.go:217 panicf key=value\n",
		},
		{
			lvl: uint8(PanicLevel),
			msg: "panic",
			flds: []Field{
				F("key", "value"),
			},
			want: "PANIC log_test.go:215 panic key=value\n",
		},
		{
			lvl:  uint8(TraceLevel),
			msg:  "trace",
			flds: nil,
			want: "TRACE log_test.go:180 trace ",
		},
		{
			lvl:    uint8(TraceLevel),
			msg:    "tracef",
			printf: "%s",
			flds:   nil,
			want:   "TRACE log_test.go:182 tracef ",
		},
		{
			lvl: uint8(TraceLevel),
			msg: "trace",
			flds: []Field{
				F("key", "value"),
			},
			want: "TRACE log_test.go:180 trace ",
		},
		{
			lvl:    uint8(TraceLevel),
			msg:    "tracef",
			printf: "%s",
			flds: []Field{
				F("key", "value"),
			},
			want: "TRACE log_test.go:182 tracef ",
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
			want: "DEBUG log_test.go:174 debug key=string key=1 key=2 key=3 key=4 key=5 key=1 key=2 key=3 key=4 key=5 key=true key={struct}\n",
		},
	}
}
