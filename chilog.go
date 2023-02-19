package chilog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// RequestIDHeader is the name of the HTTP Header which contains the request id.
// Exported so that it can be changed by developers
var RequestIDHeader = "X-Request-Id"

type logFields struct {
	RemoteIP   string
	Host       string
	Method     string
	Path       string
	Body       string
	StatusCode int
	Latency    float64
	Error      error
	Stack      []byte
}

func (l *logFields) MarshalZerologObject(e *zerolog.Event) {
	e.
		Str("remote_ip", l.RemoteIP).
		Str("host", l.Host).
		Str("method", l.Method).
		Str("path", l.Path).
		Str("body", l.Body).
		Int("status_code", l.StatusCode).
		Float64("latency", l.Latency).
		Str("tag", "request")

	if l.Error != nil {
		e.Err(l.Error)
	}

	if l.Stack != nil {
		e.Bytes("stack", l.Stack)
	}
}

// Middleware contains functionality of request_id, logger and recover for request traceability
func Middleware(filter func(w http.ResponseWriter, r *http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check filter
			if filter != nil && filter(w, r) {
				next.ServeHTTP(w, r)
				return
			}

			// Start timer
			start := time.Now()

			// Generate request ID
			// will search for a request ID header and set into the log context
			if r.Header.Get(RequestIDHeader) == "" {
				r.Header.Set(RequestIDHeader, uuid.New().String())
			}

			ctx := log.With().
				Str("request_id", r.Header.Get(RequestIDHeader)).
				Logger().
				WithContext(r.Context())

			// Wraps an http.ResponseWriter, returning a proxy that allows you to
			// hook into various parts of the response process.
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Read request body
			var buf []byte
			if r.Body != nil {
				buf, _ = io.ReadAll(r.Body)

				// Restore the io.ReadCloser to its original state
				r.Body = io.NopCloser(bytes.NewBuffer(buf))
			}

			// Create log fields
			fields := &logFields{
				RemoteIP: r.RemoteAddr,
				Method:   r.Method,
				Host:     r.Host,
				Path:     r.URL.Path,
				Body:     formatReqBody(buf),
			}

			defer func() {
				rvr := recover()

				if rvr != nil {
					if rvr == http.ErrAbortHandler {
						// We don't recover http.ErrAbortHandler so the response
						// to the client is aborted, this should not be logged
						panic(rvr)
					}

					err, ok := rvr.(error)
					if !ok {
						err = fmt.Errorf("%v", rvr)
					}

					fields.Error = err
					fields.Stack = debug.Stack()

					w.WriteHeader(http.StatusInternalServerError)
				}

				fields.StatusCode = ww.Status()
				fields.Latency = float64(time.Since(start).Nanoseconds()/1e4) / 100.0

				switch {
				case rvr != nil:
					log.Ctx(ctx).Error().EmbedObject(fields).Msg("panic recover")
				case fields.StatusCode >= 500:
					log.Ctx(ctx).Error().EmbedObject(fields).Msg("server error")
				case fields.StatusCode >= 400:
					log.Ctx(ctx).Error().EmbedObject(fields).Msg("client error")
				case fields.StatusCode >= 300:
					log.Ctx(ctx).Warn().EmbedObject(fields).Msg("redirect")
				case fields.StatusCode >= 200:
					log.Ctx(ctx).Info().EmbedObject(fields).Msg("success")
				case fields.StatusCode >= 100:
					log.Ctx(ctx).Info().EmbedObject(fields).Msg("informative")
				default:
					log.Ctx(ctx).Warn().EmbedObject(fields).Msg("unknown status")
				}

			}()

			next.ServeHTTP(ww, r.WithContext(ctx))

		})
	}
}

func formatReqBody(data []byte) string {
	var js map[string]interface{}
	if json.Unmarshal(data, &js) != nil {
		return string(data)
	}

	result := new(bytes.Buffer)
	if err := json.Compact(result, data); err != nil {
		log.Error().Err(err).Msg("error compacting body request json")
		return ""
	}

	return result.String()
}
