package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var unixZero = time.Unix(0, 0)

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

	var err error
	if e := w.file.Sync(); e != nil {
		err = multierror.Append(err, e)
	}
	if e := w.file.Close(); e != nil {
		err = multierror.Append(err, e)
	}
	return err
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

// type ZKLoggerBridge {{{

type ZKLoggerBridge struct{}

func (_ ZKLoggerBridge) Printf(fmt string, args ...interface{}) {
	log.Logger.Log().Msgf("zookeeper: "+fmt, args...)
}

var _ zk.Logger = ZKLoggerBridge{}

// }}}

// type ZapLoggerBridge {{{

type ZapLoggerBridge struct {
}

func (_ ZapLoggerBridge) Write(p []byte) (int, error) {
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

func (_ ZapLoggerBridge) Sync() error {
	return nil
}

func (_ ZapLoggerBridge) Close() error {
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
	zap.RegisterSink("dummy", func(u *url.URL) (zap.Sink, error) {
		return ZapLoggerBridge{}, nil
	})

	zaplogger, err := NewDummyZapConfig().Build()
	if err != nil {
		panic(err)
	}

	zap.ReplaceGlobals(zaplogger)
}
