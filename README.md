# Chilog
The [Chi](https://github.com/go-chi/chi) middleware that contains functionality of requestid, recover and logs HTTP request/response details for traceability.

## Installation
Install chilog with:

```sh
go get -u github.com/Qiscus-Integration/chilog
```

## Usage
```go
package main

import (
	"net/http"

	"github.com/Qiscus-Integration/chilog"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func filter(w http.ResponseWriter, r *http.Request) bool {
	return r.URL.Path == "/healthcheck"
}

func main() {
	r := chi.NewRouter()
	r.Use(chilog.Middleware(filter))
	// Output: {"level":"info","request_id":"9627a4a0-9d94-4ab6-844c-9599c0a15cd0","remote_ip":"[::1]:62542","host":"localhost:8080","method":"GET","path":"/","body":"","status_code":200,"latency":0,"tag":"request","time":"2023-02-19T14:07:37+07:00","message":"success"}

	// Or without filter
	// r.Use(chilog.Middleware(nil))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		// Log with request context to get request_id
		ctx := r.Context()
		log.Ctx(ctx).Info().Msg("hello world")
		// Output: {"level":"info","request_id":"9627a4a0-9d94-4ab6-844c-9599c0a15cd0","time":"2023-02-19T15:06:39+07:00","message":"hello world"}

		w.WriteHeader(http.StatusOK)
	})

	srv := http.Server{Addr: ":8080", Handler: r}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal().Msgf("listen:%+s\n", err)
	}
}
```