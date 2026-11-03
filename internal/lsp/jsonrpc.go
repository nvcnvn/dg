package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
)

// maxMessage bounds the body of one message, so a bad Content-Length cannot exhaust memory.
const maxMessage = 64 << 20

// readMessage reads the body of one message framed by a Content-Length header.
func readMessage(r *bufio.Reader) ([]byte, error) {
	header, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		if errors.Is(err, io.EOF) && len(header) == 0 {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("reading a message header: %w", err)
	}
	n, err := strconv.Atoi(header.Get("Content-Length"))
	if err != nil || n < 0 || n > maxMessage {
		return nil, fmt.Errorf("invalid Content-Length %q", header.Get("Content-Length"))
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("reading a message body: %w", err)
	}
	return body, nil
}

func writeMessage(w io.Writer, m any) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

// inMessage is a request or notification from the client; ID is nil for a notification.
type inMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

// errorResponse has no result, which JSON-RPC forbids beside an error.
type errorResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   *rpcError       `json:"error"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

// Error codes of JSON-RPC and the Language Server Protocol.
const (
	codeParseError           = -32700
	codeInvalidRequest       = -32600
	codeMethodNotFound       = -32601
	codeInvalidParams        = -32602
	codeServerNotInitialized = -32002
	codeRequestFailed        = -32803
)
