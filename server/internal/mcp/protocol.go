package mcp

import (
	"bytes"
	"encoding/json"
)

var nullID = json.RawMessage("null")

type rpcRequest struct {
	JSONRPC string
	ID      json.RawMessage
	HasID   bool
	Method  string
	Params  json.RawMessage
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type requestValidationError struct {
	id      json.RawMessage
	code    int
	message string
	data    any
}

func parseRequest(raw json.RawMessage) (rpcRequest, *requestValidationError) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return rpcRequest{}, invalidRequest(nullID)
	}

	request := rpcRequest{ID: nullID}
	if rawID, ok := object["id"]; ok {
		if !validRequestID(rawID) {
			return rpcRequest{}, invalidRequest(nullID)
		}
		request.ID = cloneRaw(rawID)
		request.HasID = true
	}

	if rawVersion, ok := object["jsonrpc"]; !ok || json.Unmarshal(rawVersion, &request.JSONRPC) != nil || request.JSONRPC != "2.0" {
		return rpcRequest{}, invalidRequest(request.ID)
	}
	if rawMethod, ok := object["method"]; !ok || json.Unmarshal(rawMethod, &request.Method) != nil || request.Method == "" {
		return rpcRequest{}, invalidRequest(request.ID)
	}
	if rawParams, ok := object["params"]; ok {
		trimmed := bytes.TrimSpace(rawParams)
		if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
			return rpcRequest{}, &requestValidationError{id: request.ID, code: -32602, message: "Invalid params"}
		}
		request.Params = cloneRaw(rawParams)
	}
	return request, nil
}

func validRequestID(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	switch trimmed[0] {
	case '"':
		var value string
		return json.Unmarshal(trimmed, &value) == nil
	case 'n':
		return bytes.Equal(trimmed, nullID)
	default:
		var number json.Number
		decoder := json.NewDecoder(bytes.NewReader(trimmed))
		decoder.UseNumber()
		return decoder.Decode(&number) == nil
	}
}

func invalidRequest(id json.RawMessage) *requestValidationError {
	return &requestValidationError{id: id, code: -32600, message: "Invalid Request"}
}

func rpcResultResponse(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: cloneRaw(id), Result: result}
}

func rpcErrorResponse(id json.RawMessage, code int, message string, data any) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      cloneRaw(id),
		Error:   &rpcError{Code: code, Message: message, Data: data},
	}
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return cloneRaw(nullID)
	}
	return append(json.RawMessage(nil), raw...)
}
