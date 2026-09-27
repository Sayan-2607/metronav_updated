package api

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

type bufioReadWriter = bufio.ReadWriter

func hijack(w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}
	return h.Hijack()
}
