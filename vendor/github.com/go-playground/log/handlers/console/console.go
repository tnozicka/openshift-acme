package console

import (
	"bufio"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"strconv"
	"strings"

	"github.com/go-playground/ansi"
	"github.com/go-playground/log"
)

// FormatFunc is the function that the workers use to create
// a new Formatter per worker allowing reusable go routine safe
// variable to be used within your Formatter function.
type FormatFunc func(c *Console) Formatter

// Formatter is the function used to format the Redis entry
type Formatter func(e *log.Entry) []byte

const (
	space   = byte(' ')
	equals  = byte('=')
	newLine = byte('\n')
	colon   = byte(':')
	base10  = 10
	v       = "%v"
	gopath  = "GOPATH"
)

// Console is an instance of the console logger
type Console struct {
	buffer          uint
	numWorkers      uint
	colors          [9]ansi.EscSeq
	writer          io.Writer
	formatFunc      FormatFunc
	timestampFormat string
	gopath          string
	fileDisplay     log.FilenameDisplay
	displayColor    bool
	redirStdOutput  bool
}

// Colors mapping.
var defaultColors = [...]ansi.EscSeq{
	log.DebugLevel:  ansi.Green,
	log.TraceLevel:  ansi.White,
	log.InfoLevel:   ansi.Blue,
	log.NoticeLevel: ansi.LightCyan,
	log.WarnLevel:   ansi.LightYellow,
	log.ErrorLevel:  ansi.LightRed,
	log.PanicLevel:  ansi.Red,
	log.AlertLevel:  ansi.Red + ansi.Underline,
	log.FatalLevel:  ansi.Red + ansi.Underline + ansi.Blink,
}

// New returns a new instance of the console logger
func New() *Console {
	return &Console{
		buffer:          3,
		numWorkers:      3,
		colors:          defaultColors,
		writer:          os.Stderr,
		timestampFormat: log.DefaultTimeFormat,
		displayColor:    true,
		fileDisplay:     log.Lshortfile,
		formatFunc:      defaultFormatFunc,
	}
}

// SetFilenameDisplay tells Console the filename, when present, how to display
func (c *Console) SetFilenameDisplay(fd log.FilenameDisplay) {
	c.fileDisplay = fd
}

// FilenameDisplay returns Console's current filename display setting
func (c *Console) FilenameDisplay() log.FilenameDisplay {
	return c.fileDisplay
}

// RedirectSTDLogOutput tells Console to redirect the std Logger output
// to the log package itself.
func (c *Console) RedirectSTDLogOutput(b bool) {
	c.redirStdOutput = b
}

// SetDisplayColor tells Console to output in color or not
// Default is : true
func (c *Console) SetDisplayColor(color bool) {
	c.displayColor = color
}

// DisplayColor returns if logging color or not
func (c *Console) DisplayColor() bool {
	return c.displayColor
}

// GetDisplayColor returns the color for the given log level
func (c *Console) GetDisplayColor(level log.Level) ansi.EscSeq {
	return c.colors[level]
}

// SetTimestampFormat sets Console's timestamp output format
// Default is : "2006-01-02T15:04:05.000000000Z07:00"
func (c *Console) SetTimestampFormat(format string) {
	c.timestampFormat = format
}

// TimestampFormat returns Console's current timestamp output format
func (c *Console) TimestampFormat() string {
	return c.timestampFormat
}

// GOPATH returns the GOPATH calculated by Console
func (c *Console) GOPATH() string {
	return c.gopath
}

// SetWriter sets Console's wriiter
// Default is : os.Stderr
func (c *Console) SetWriter(w io.Writer) {
	c.writer = w
}

// SetBuffersAndWorkers sets the channels buffer size and number of concurrent workers.
// These settings should be thought about together, hence setting both in the same function.
func (c *Console) SetBuffersAndWorkers(size uint, workers uint) {
	c.buffer = size

	if workers == 0 {
		// just in case no log registered yet
		stdlog.Println("Invalid number of workers specified, setting to 1")
		log.Warn("Invalid number of workers specified, setting to 1")

		workers = 1
	}

	c.numWorkers = workers
}

// SetFormatFunc sets FormatFunc each worker will call to get
// a Formatter func
func (c *Console) SetFormatFunc(fn FormatFunc) {
	c.formatFunc = fn
}

// Run starts the logger consuming on the returned channed
func (c *Console) Run() chan<- *log.Entry {

	// pre-setup
	if c.fileDisplay == log.Llongfile {
		// gather $GOPATH for use in stripping off of full name
		// if not found still ok as will be blank
		c.gopath = os.Getenv(gopath)
		if len(c.gopath) != 0 {
			c.gopath += string(os.PathSeparator) + "src" + string(os.PathSeparator)
		}
	}

	// in a big high traffic app, set a higher buffer
	ch := make(chan *log.Entry, c.buffer)

	for i := 0; i <= int(c.numWorkers); i++ {
		go c.handleLog(ch)
	}

	// let's check to see if we should hanlde the std log messages as well
	if c.redirStdOutput {

		done := make(chan struct{})

		go handleStdLogger(done)

		<-done // have to wait, it was running too quickly and some messages can be lost
	}

	return ch
}

// this will redirect the output of
func handleStdLogger(done chan<- struct{}) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()

	stdlog.SetOutput(w)

	scanner := bufio.NewScanner(r)

	go func() {
		done <- struct{}{}
	}()

	for scanner.Scan() {
		log.WithFields(log.F("stdlog", true)).Notice(scanner.Text())
	}
}

// handleLog consumes and logs any Entry's passed to the channel
func (c *Console) handleLog(entries <-chan *log.Entry) {

	var e *log.Entry
	// var b io.WriterTo
	var b []byte
	formatter := c.formatFunc(c)

	for e = range entries {

		b = formatter(e)
		c.writer.Write(b)

		e.Consumed()
	}
}

func defaultFormatFunc(c *Console) Formatter {

	var b []byte
	var file string
	var lvl string
	var i int

	gopath := c.GOPATH()
	tsFormat := c.TimestampFormat()
	fnameDisplay := c.FilenameDisplay()

	if c.DisplayColor() {

		var color ansi.EscSeq

		return func(e *log.Entry) []byte {
			b = b[0:0]
			color = c.GetDisplayColor(e.Level)

			if e.Line == 0 {

				b = append(b, e.Timestamp.Format(tsFormat)...)
				b = append(b, space)
				b = append(b, color...)

				lvl = e.Level.String()

				for i = 0; i < 6-len(lvl); i++ {
					b = append(b, space)
				}
				b = append(b, lvl...)
				b = append(b, ansi.Reset...)
				b = append(b, space)
				b = append(b, e.Message...)

			} else {
				file = e.File

				if fnameDisplay == log.Lshortfile {

					for i = len(file) - 1; i > 0; i-- {
						if file[i] == '/' {
							file = file[i+1:]
							break
						}
					}
				} else {
					file = file[len(gopath):]
				}

				b = append(b, e.Timestamp.Format(tsFormat)...)
				b = append(b, space)
				b = append(b, color...)

				lvl = e.Level.String()

				for i = 0; i < 6-len(lvl); i++ {
					b = append(b, space)
				}

				b = append(b, lvl...)
				b = append(b, ansi.Reset...)
				b = append(b, space)
				b = append(b, file...)
				b = append(b, colon)
				b = strconv.AppendInt(b, int64(e.Line), base10)
				b = append(b, space)
				b = append(b, e.Message...)
			}

			for _, f := range e.Fields {
				b = append(b, space)
				b = append(b, color...)
				b = append(b, f.Key...)
				b = append(b, ansi.Reset...)
				b = append(b, equals)

				switch f.Value.(type) {
				case string:
					b = append(b, f.Value.(string)...)
				case int:
					b = strconv.AppendInt(b, int64(f.Value.(int)), base10)
				case int8:
					b = strconv.AppendInt(b, int64(f.Value.(int8)), base10)
				case int16:
					b = strconv.AppendInt(b, int64(f.Value.(int16)), base10)
				case int32:
					b = strconv.AppendInt(b, int64(f.Value.(int32)), base10)
				case int64:
					b = strconv.AppendInt(b, f.Value.(int64), base10)
				case uint:
					b = strconv.AppendUint(b, uint64(f.Value.(uint)), base10)
				case uint8:
					b = strconv.AppendUint(b, uint64(f.Value.(uint8)), base10)
				case uint16:
					b = strconv.AppendUint(b, uint64(f.Value.(uint16)), base10)
				case uint32:
					b = strconv.AppendUint(b, uint64(f.Value.(uint32)), base10)
				case uint64:
					b = strconv.AppendUint(b, f.Value.(uint64), base10)
				case float32:
					b = strconv.AppendFloat(b, float64(f.Value.(float32)), 'f', -1, 32)
				case float64:
					b = strconv.AppendFloat(b, f.Value.(float64), 'f', -1, 64)
				case bool:
					b = strconv.AppendBool(b, f.Value.(bool))
				default:
					b = append(b, fmt.Sprintf(v, f.Value)...)
				}
			}

			b = append(b, newLine)

			return b
		}
	}

	return func(e *log.Entry) []byte {
		b = b[0:0]

		if e.Line == 0 {

			b = append(b, e.Timestamp.Format(tsFormat)...)
			b = append(b, space)

			lvl = e.Level.String()

			for i = 0; i < 6-len(lvl); i++ {
				b = append(b, space)
			}

			b = append(b, lvl...)
			b = append(b, space)
			b = append(b, e.Message...)

		} else {
			file = e.File

			if fnameDisplay == log.Lshortfile {

				for i = len(file) - 1; i > 0; i-- {
					if file[i] == '/' {
						file = file[i+1:]
						break
					}
				}
			} else {

				// additional check, just in case user does
				// have a $GOPATH but code isn't under it.
				if strings.HasPrefix(file, gopath) {
					file = file[len(gopath):]
				}
			}

			b = append(b, e.Timestamp.Format(tsFormat)...)
			b = append(b, space)

			lvl = e.Level.String()

			for i = 0; i < 6-len(lvl); i++ {
				b = append(b, space)
			}

			b = append(b, lvl...)
			b = append(b, space)
			b = append(b, file...)
			b = append(b, colon)
			b = strconv.AppendInt(b, int64(e.Line), base10)
			b = append(b, space)
			b = append(b, e.Message...)
		}

		for _, f := range e.Fields {
			b = append(b, space)
			b = append(b, f.Key...)
			b = append(b, equals)

			switch f.Value.(type) {
			case string:
				b = append(b, f.Value.(string)...)
			case int:
				b = strconv.AppendInt(b, int64(f.Value.(int)), base10)
			case int8:
				b = strconv.AppendInt(b, int64(f.Value.(int8)), base10)
			case int16:
				b = strconv.AppendInt(b, int64(f.Value.(int16)), base10)
			case int32:
				b = strconv.AppendInt(b, int64(f.Value.(int32)), base10)
			case int64:
				b = strconv.AppendInt(b, f.Value.(int64), base10)
			case uint:
				b = strconv.AppendUint(b, uint64(f.Value.(uint)), base10)
			case uint8:
				b = strconv.AppendUint(b, uint64(f.Value.(uint8)), base10)
			case uint16:
				b = strconv.AppendUint(b, uint64(f.Value.(uint16)), base10)
			case uint32:
				b = strconv.AppendUint(b, uint64(f.Value.(uint32)), base10)
			case uint64:
				b = strconv.AppendUint(b, f.Value.(uint64), base10)
			case bool:
				b = strconv.AppendBool(b, f.Value.(bool))
			default:
				b = append(b, fmt.Sprintf(v, f.Value)...)
			}
		}

		b = append(b, newLine)

		return b
	}
}
