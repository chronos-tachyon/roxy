package mainutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	multierror "github.com/hashicorp/go-multierror"
	getopt "github.com/pborman/getopt/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/journald"
	"github.com/rs/zerolog/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

var unixZero = time.Unix(0, 0)

var gLogger *RotatingLogWriter

var (
	flagVersion     bool
	flagDebug       bool
	flagTrace       bool
	flagLogStderr   bool
	flagLogJournald bool
	flagLogFile     string
)

func RegisterVersionFlag() {
	getopt.FlagLong(&flagVersion, "version", 'V', "print version and exit")
}

func RegisterLoggingFlags() {
	getopt.FlagLong(&flagDebug, "verbose", 'v', "enable debug logging")
	getopt.FlagLong(&flagTrace, "debug", 'd', "enable debug and trace logging")
	getopt.FlagLong(&flagLogStderr, "log-stderr", 'S', "log JSON to stderr")
	getopt.FlagLong(&flagLogJournald, "log-journald", 'J', "log to journald")
	getopt.FlagLong(&flagLogFile, "log-file", 'l', "log JSON to file")
}

func InitVersion() {
	if flagVersion {
		fmt.Println(AppVersion())
		os.Exit(0)
	}
}

func InitLogging() {
	if flagLogStderr && flagLogJournald {
		fmt.Fprintln(os.Stderr, "fatal: flags '--log-stderr' and '--log-journald' are mutually exclusive")
		os.Exit(1)
	}
	if flagLogStderr && flagLogFile != "" {
		fmt.Fprintln(os.Stderr, "fatal: flags '--log-stderr' and '--log-file' are mutually exclusive")
		os.Exit(1)
	}
	if flagLogJournald && flagLogFile != "" {
		fmt.Fprintln(os.Stderr, "fatal: flags '--log-journald' and '--log-file' are mutually exclusive")
		os.Exit(1)
	}

	if flagLogFile != "" {
		abs, err := roxyutil.ExpandPath(flagLogFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
		flagLogFile = abs
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.DurationFieldUnit = time.Second
	zerolog.DurationFieldInteger = false
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if flagDebug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	if flagTrace {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	}

	switch {
	case flagLogStderr:
		// do nothing

	case flagLogJournald:
		log.Logger = log.Output(journald.NewJournalDWriter())

	case flagLogFile != "":
		var err error
		gLogger, err = NewRotatingLogWriter(flagLogFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: failed to open log file for append: %q: %v\n", flagLogFile, err)
			os.Exit(1)
		}
		log.Logger = log.Output(gLogger)

	default:
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	stdlog.SetFlags(0)
	stdlog.SetOutput(log.Logger)
}

func DoneLogging() {
	if gLogger != nil {
		_ = gLogger.Close()
	}
}

func RotateLogs() error {
	if gLogger != nil {
		if err := gLogger.Rotate(); err != nil {
			log.Logger.Error().
				Err(err).
				Msg("failed to rotate logs")
			return err
		}
	}
	return nil
}

// type RotatingLogWriter {{{

type RotatingLogWriter struct {
	fileName   string
	mu         sync.Mutex
	cv         *sync.Cond
	file       *os.File
	numWriters int
}

func NewRotatingLogWriter(fileName string) (*RotatingLogWriter, error) {
	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	w := &RotatingLogWriter{
		fileName:   fileName,
		file:       file,
		numWriters: 0,
	}
	w.cv = sync.NewCond(&w.mu)
	return w, nil
}

func (w *RotatingLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	file := w.file
	w.numWriters++
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.numWriters--
		if w.numWriters <= 0 {
			w.cv.Signal()
		}
		w.mu.Unlock()
	}()

	return file.Write(p)
}

func (w *RotatingLogWriter) Close() error {
	w.mu.Lock()
	defer func() {
		w.cv.Signal()
		w.mu.Unlock()
	}()

	for w.numWriters > 0 {
		w.cv.Wait()
	}

	var errs multierror.Error
	if err := w.file.Sync(); err != nil {
		errs.Errors = append(errs.Errors, err)
	}
	if err := w.file.Close(); err != nil {
		errs.Errors = append(errs.Errors, err)
	}
	return errs.ErrorOrNil()
}

func (w *RotatingLogWriter) Rotate() error {
	newFile, err := os.OpenFile(w.fileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer func() {
		w.cv.Signal()
		w.mu.Unlock()
	}()

	for w.numWriters > 0 {
		w.cv.Wait()
	}

	oldFile := w.file
	w.file = newFile

	if e := oldFile.Sync(); e != nil {
		err = multierror.Append(err, e)
	}
	if e := oldFile.Close(); e != nil {
		err = multierror.Append(err, e)
	}
	return err
}

var _ io.WriteCloser = (*RotatingLogWriter)(nil)

// }}}

// type PromLoggerBridge {{{

type PromLoggerBridge struct{}

func (PromLoggerBridge) Println(v ...interface{}) {
	log.Logger.Log().Msg("prometheus: " + fmt.Sprint(v...))
}

var _ promhttp.Logger = PromLoggerBridge{}

// }}}

// type ZKLoggerBridge {{{

type ZKLoggerBridge struct{}

func (ZKLoggerBridge) Printf(fmt string, args ...interface{}) {
	log.Logger.Log().Msgf("zookeeper: "+fmt, args...)
}

var _ zk.Logger = ZKLoggerBridge{}

// }}}

// type ZapLoggerBridge {{{

type ZapLoggerBridge struct{}

func (ZapLoggerBridge) Write(p []byte) (int, error) {
	var data map[string]interface{}
	d := json.NewDecoder(bytes.NewReader(p))
	d.UseNumber()
	if err := d.Decode(&data); err != nil {
		log.Logger.Error().Err(err).Msg("ZapLoggerBridge.Write: json.Decoder.Decode")
		return len(p), nil
	}

	var e *zerolog.Event

	if rawLevelStr, found := data[zerolog.LevelFieldName]; found {
		if levelStr, ok := rawLevelStr.(string); ok {
			delete(data, zerolog.LevelFieldName)
			l, err := zerolog.ParseLevel(levelStr)
			if err == nil {
				e = log.Logger.WithLevel(l)
			} else {
				log.Error().Str("str", levelStr).Err(err).Msg("ZapLoggerBridge.Write: ParseLevel")
			}
		}
	}
	if e == nil {
		e = log.Logger.Log()
	}

	var (
		hasMessage bool
		message    string
	)
	if rawMessageStr, found := data[zerolog.MessageFieldName]; found {
		if messageStr, ok := rawMessageStr.(string); ok {
			delete(data, zerolog.MessageFieldName)
			hasMessage = true
			message = messageStr
		}
	}

	e = e.Interface("zap", data)

	if hasMessage {
		e.Msg(message)
	} else {
		e.Send()
	}

	return len(p), nil
}

func (ZapLoggerBridge) Sync() error {
	return nil
}

func (ZapLoggerBridge) Close() error {
	return nil
}

var _ zap.Sink = ZapLoggerBridge{}

// }}}

func NewDummyZapConfig() *zap.Config {
	return &zap.Config{
		Level:    zap.NewAtomicLevelAt(zapcore.InfoLevel),
		Encoding: "json",
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:    zerolog.MessageFieldName,
			LevelKey:      zerolog.LevelFieldName,
			TimeKey:       zerolog.TimestampFieldName,
			NameKey:       "name",
			CallerKey:     zerolog.CallerFieldName,
			FunctionKey:   "function",
			StacktraceKey: "stackTrace",
			LineEnding:    "lineEnding",
			EncodeLevel: func(l zapcore.Level, out zapcore.PrimitiveArrayEncoder) {
				var str string
				switch l {
				case zapcore.DebugLevel:
					str = zerolog.DebugLevel.String()
				case zapcore.InfoLevel:
					str = zerolog.InfoLevel.String()
				case zapcore.WarnLevel:
					str = zerolog.WarnLevel.String()
				case zapcore.ErrorLevel:
					str = zerolog.ErrorLevel.String()
				case zapcore.DPanicLevel:
					str = zerolog.PanicLevel.String()
				case zapcore.PanicLevel:
					str = zerolog.PanicLevel.String()
				case zapcore.FatalLevel:
					str = zerolog.FatalLevel.String()
				default:
					str = zerolog.NoLevel.String()
				}
				out.AppendString(str)
			},
			EncodeTime: func(t time.Time, out zapcore.PrimitiveArrayEncoder) {
				out.AppendFloat64(t.Sub(unixZero).Seconds())
			},
			EncodeDuration: func(d time.Duration, out zapcore.PrimitiveArrayEncoder) {
				out.AppendFloat64(d.Seconds())
			},
			EncodeCaller: zapcore.FullCallerEncoder,
			EncodeName:   zapcore.FullNameEncoder,
		},
		OutputPaths:      []string{"dummy:///"},
		ErrorOutputPaths: []string{"dummy:///"},
		InitialFields:    nil,
	}
}

func init() {
	_ = zap.RegisterSink("dummy", func(u *url.URL) (zap.Sink, error) {
		return ZapLoggerBridge{}, nil
	})

	zaplogger, err := NewDummyZapConfig().Build()
	if err != nil {
		panic(err)
	}

	zap.ReplaceGlobals(zaplogger)
}
