package main

import (
	// "bufio"
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/buger/goreplay/proto"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/printer"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		encoded := scanner.Bytes()
		buf := make([]byte, len(encoded)/2)
		hex.Decode(buf, encoded)

		process(buf)
	}
}

func process(buf []byte) {
	// First byte indicate payload type, possible values:
	//  1 - Request
	//  2 - Response
	//  3 - ReplayedResponse
	payloadType := buf[0]
	headerSize := bytes.IndexByte(buf, '\n') + 1
	// header := buf[:headerSize-1]

	// Header contains space separated values of: request type, request id, and request start time (or round-trip time for responses)
	// meta := bytes.Split(header, []byte(" "))
	// For each request you should receive 3 payloads (request, response, replayed response) with same request id
	// reqID := string(meta[1])
	payload := buf[headerSize:]

	Debug("Received payload:", string(buf))

	switch payloadType {
	case '1': // Request
		url := proto.Path(payload)
		Debug(string(url))
		if bytes.Equal(url, []byte("/graphql")) {
			p, err := GetGrapqhlPayload(payload)
			if err != nil {
				Debug(err)
				return
			}
			if !p.IsQuery {
				Debug("Ignore write traffic")
				return
			}
			newPayload := proto.SetHeader(payload, []byte("Canonical-Resource"), []byte(p.Operation))
			buf = append(buf[:headerSize], newPayload...)

			// Emitting data back
			os.Stdout.Write(encode(buf))
		}
	case '2': // Original response
	case '3': // Replayed response
	default:
	}
}

func encode(buf []byte) []byte {
	dst := make([]byte, len(buf)*2+1)
	hex.Encode(dst, buf)
	dst[len(dst)-1] = '\n'

	return dst
}

type postData struct {
	Query     string                 `json:"query"`
	Operation string                 `json:"operationName"`
	Variables map[string]interface{} `json:"variables"`
}

type GraphQLPayLoad struct {
	postData
	IsQuery bool // is query or mutation
}

func GetGrapqhlPayload(payload []byte) (*GraphQLPayLoad, error) {
	var p postData
	if err := json.Unmarshal(proto.Body(payload), &p); err != nil {
		fmt.Fprintf(os.Stderr, "Fail to decode payload to json %v\n", string(payload))
		return nil, err
	}

	result := GraphQLPayLoad{postData: p}
	astDoc, err := parser.Parse(parser.ParseParams{Source: result.Query})
	if err != nil {
		return nil, err
	}
	Debug(printer.Print(astDoc))

	for _, definition := range astDoc.Definitions {
		switch definition := definition.(type) {
		case *ast.OperationDefinition:
			if definition.Operation == "query" {
				result.IsQuery = true
				break
			}
		case *ast.FragmentDefinition:
		default:
			return nil, fmt.Errorf("GraphQL cannot execute a request containing a %v", definition.GetKind())
		}
	}

	return &result, nil
}

func Debug(args ...interface{}) {
	if os.Getenv("GOR_TEST") == "1" {
		fmt.Fprint(os.Stderr, "[DEBUG][TOKEN-MOD] ")
		fmt.Fprintln(os.Stderr, args...)
	}
}
