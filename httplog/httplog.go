package httplog

import (
	"../uuid"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type RequestLogger struct {
	child http.Handler
}

func NewRequestLogger(child http.Handler) *RequestLogger {
	return &RequestLogger{child}
}

func (self *RequestLogger) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	id, err := uuid.UniqueId()
	if err != nil {
		return
	}

	lw := newLoggingResponseWriter(w)
	// Maybe if we start using it somewhere else
	//req.Header.Set("X-Request-Id", id)

	var headerJson []byte

	startTime := time.Now()

	headerJson, _ = json.Marshal(req.Header)
	log.Printf("->[%s] %s %s %s %s", id, req.RemoteAddr, req.Method, req.RequestURI, headerJson)

	self.child.ServeHTTP(lw, req)

	headerJson, _ = json.Marshal(lw.Header())
	log.Printf("<-[%s] (d=%s) Status: %d - %s", id, time.Since(startTime), lw.StatusCode, headerJson)
}

type LoggingResponseWriter struct {
	responseWriter http.ResponseWriter
	StatusCode     int
}

func newLoggingResponseWriter(rw http.ResponseWriter) *LoggingResponseWriter {
	return &LoggingResponseWriter{
		responseWriter: rw,
		StatusCode:     -1,
	}
}

func (self *LoggingResponseWriter) Header() http.Header {
	return self.responseWriter.Header()
}

func (self *LoggingResponseWriter) Write(data []byte) (int, error) {
	return self.responseWriter.Write(data)
}

func (self *LoggingResponseWriter) WriteHeader(status int) {
	self.StatusCode = status
	self.responseWriter.WriteHeader(status)
}
