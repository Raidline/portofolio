package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	HttpVersion       = "HTTP/1.1"
	HttpPartSeperator = "\r\n"
)

var codeToReason = map[int]string{
	200: "OK",
	201: "Created",
	404: "Not Found",
}

type reqProps struct {
	method  string
	request *reqPath
	headers map[string]string
	body    []byte
}

type reqPath struct {
	path   string
	params []string
}

type server struct {
	listener net.Listener
	req      *reqProps
	paths    *tree
}

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	s := &server{
		listener: l,
		paths:    create(),
	}
	// no wildcards considered

	routesErr := registerRoutes(s)

	if routesErr != nil {
		fmt.Println("Error register routes : ", routesErr.Error())
		os.Exit(1)
	}

	for {
		conn, connErr := l.Accept()
		if connErr != nil {
			fmt.Println("Error accepting connection: ", connErr.Error())
			os.Exit(1)
		}
		go handleConnectionToServer(s, conn)
	}
}

func registerRoutes(s *server) error {
	handleRoot := s.registerHandler("", func(props *reqProps, conn net.Conn) {
		b := s.writeResponse(200, make(map[string]string), "", conn)

		if b == -1 {
			fmt.Println("we could not answer the request")
		}
	})

	if handleRoot != nil {
		fmt.Println("Handler has already been registered")
		return handleRoot
	}

	handleErr := s.registerHandler("index.html", func(props *reqProps, conn net.Conn) {
		b := s.writeResponse(404, make(map[string]string), "", conn)

		if b == -1 {
			fmt.Println("we could not answer the request")
		}
	})

	if handleErr != nil {
		fmt.Println("Handler has already been registered")
		return handleErr
	}

	handleErr2 := s.registerHandler("echo/{str}", func(props *reqProps, conn net.Conn) {

		if len(props.request.params) == 0 {
			panic("we need to receive a param for the str template")
		}
		body := props.request.params[0]

		encodings := props.headers["Accept-Encoding"]

		var headers = map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": strconv.Itoa(len(body)),
		}

		for _, encoding := range strings.Split(encodings, ",") {
			if strings.TrimSpace(encoding) == "gzip" {
				headers["Content-Encoding"] = "gzip"
				var buffer bytes.Buffer
				w := gzip.NewWriter(&buffer)
				_, err := w.Write([]byte(body))
				if err != nil {
					fmt.Println("we could not compress the body")
					return
				}
				w.Close()
				body = buffer.String()
				headers["Content-Length"] = strconv.Itoa(len(body))
			}
		}

		b := s.writeResponse(200, headers, body, conn)

		if b == -1 {
			fmt.Println("we could not answer the request")
		}
	})

	if handleErr2 != nil {
		fmt.Println("Handler has already been registered")
		return handleErr2
	}

	handleErr3 := s.registerHandler("user-agent", func(props *reqProps, conn net.Conn) {

		body := props.headers["User-Agent"]

		var headers = map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": strconv.Itoa(len(body)),
		}
		b := s.writeResponse(200, headers, body, conn)

		if b == -1 {
			fmt.Println("we could not answer the request")
		}
	})

	if handleErr3 != nil {
		fmt.Println("Handler has already been registered")
		return handleErr3
	}

	fileErr := s.registerHandler("files/{filename}", func(props *reqProps, conn net.Conn) {
		filename := props.request.params[0]
		directory := os.Args[2]

		if props.method == "GET" {
			f, err := os.Open(directory + filename)

			if err != nil {
				fmt.Printf("File %s was not found: %s", filename, err.Error())
				s.writeResponse(404, map[string]string{}, "", conn)
			}

			defer func(f *os.File) {
				err := f.Close()
				if err != nil {
					var errHeaders = map[string]string{
						"Content-Type": "text/plain",
					}
					s.writeResponse(500, errHeaders, "Internal Error", conn)
				}
			}(f)

			stat, _ := f.Stat()
			var headers = map[string]string{
				"Content-type":   "application/octet-stream",
				"Content-Length": strconv.FormatInt(stat.Size(), 10),
			}

			for {
				bs := make([]byte, 1024)

				r, e := f.Read(bs)

				if e != nil {
					var errHeaders = map[string]string{
						"Content-Type": "text/plain",
					}
					s.writeResponse(500, errHeaders, "Internal Error", conn)

					break
				}

				if r == -1 {
					break
				}

				b := s.writeResponse(200, headers, string(bs), conn)

				if b == -1 {
					fmt.Println("we could not answer the request")
				}
			}
		} else if props.method == "POST" {
			f, er := os.OpenFile(directory+"/"+filename, os.O_RDWR|os.O_CREATE, 0666)

			if er != nil {
				var errHeaders = map[string]string{
					"Content-Type": "text/plain",
				}
				s.writeResponse(500, errHeaders, "Internal Error", conn)

				return
			}

			defer func(f *os.File) {
				err := f.Close()
				if err != nil {
					var errHeaders = map[string]string{
						"Content-Type": "text/plain",
					}
					s.writeResponse(500, errHeaders, "Internal Error", conn)
				}
			}(f)

			s.writeResponse(201, map[string]string{}, "", conn)

			written, err := f.Write(props.body)

			if err != nil {
				fmt.Println("Error while creating the file!!")
			} else {
				fmt.Printf("Written %d bytes", written)
			}

		}
	})

	if fileErr != nil {
		fmt.Println("Handler has already been registered")
		return fileErr
	}

	return nil
}

func handleConnectionToServer(s *server, conn net.Conn) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Print("Error while closing connection : ", err.Error())
		}
	}(conn)

	requestBuffer, errR := s.readBytes(conn)

	if errR != nil {
		fmt.Println("Error while reading the request : ", errR.Error())
		os.Exit(1)
	}

	props, reqErr := readRequest(requestBuffer)

	if reqErr != nil {
		fmt.Println("Error while processing the request : ", reqErr.Error())
		os.Exit(1)
	}

	s.req = props

	hErr := s.handle(conn)

	if hErr != nil {
		fmt.Println("error handling request: ", hErr.Error())
		s.writeResponse(404, make(map[string]string), "", conn)
	}
}

func (s *server) writeResponse(status int, headers map[string]string, body string, conn net.Conn) int {
	write, writeErr := conn.Write(buildHttpResponse(status, headers, body))
	if writeErr != nil {
		fmt.Println("Error sending response in connection: ", writeErr.Error())
		return -1
	}
	return write
}

func (s *server) registerHandler(path string, handle func(props *reqProps, conn net.Conn)) error {
	//todo: we could validate the path validity and that would in facto return an error

	pathParts := strings.Split(path, "/")

	if len(pathParts) == 1 {
		p := pathParts[0]
		if p == "" {
			s.paths.addRoot(path, handle)
			return nil
		}
		if child, ok := s.paths.root.childPaths[p]; ok { // path already created, maybe a child as been registered first
			if child.handler != nil {
				panic(fmt.Sprintf("this path %s has already a handler associated", p))
			}

			child.handler = handle
		} else {
			s.paths.root.addChild(p, p[0] == '{', handle)
		}

		return nil
	}

	currNode := s.paths.root
	if currNode == nil {
		panic("you cannot add child paths that have no root yet, please define a global root")
	}

	for _, part := range pathParts {
		if p, ok := currNode.childPaths[part]; ok {
			currNode = p
		} else {
			currNode = currNode.addChild(part, part[0] == '{', nil)
		}
	}

	currNode.handler = handle

	return nil
}

func (s *server) handle(conn net.Conn) error {
	r := s.req.request

	root := s.paths.root
	if root.path == r.path {
		root.handler(s.req, conn)

		return nil
	}

	currNode := root

	pathParts := strings.Split(r.path, "/")

	found := true
	for _, part := range pathParts {
		if p, ok := currNode.childPaths[part]; ok {
			currNode = p
		} else {

			for _, n := range currNode.childPaths {
				if n.template {
					found = true
					r.params = append(r.params, part)
					currNode = n
					break
				}

				found = false
				break
			}
		}
	}

	if !found {
		return errors.New(fmt.Sprintf("no handler found for request %s", s.req.request))
	}

	currNode.handler(s.req, conn)

	return nil
}

func (s *server) readBytes(conn net.Conn) ([]byte, error) {
	requestBuffer := make([]byte, 1024)
	var requestData []byte

	for {
		r, errR := conn.Read(requestBuffer)

		if errR != nil {
			return nil, errR
		}

		requestData = append(requestData, requestBuffer[:r]...)

		if r < len(requestBuffer) {
			break
		}
	}

	return requestData, nil
}

func readRequest(buffer []byte) (*reqProps, error) {
	req := string(buffer)

	fmt.Println(req)

	firstSplit := strings.Index(req, "\r\n")

	requestLine := req[:firstSplit]

	requestLineParts := strings.Split(requestLine, " ")
	httpMethod := requestLineParts[0]

	fmt.Println("Http Method: ", httpMethod)
	path := strings.TrimPrefix(requestLineParts[1], "/")

	remainingHttpReq := req[firstSplit:]

	endHeadersIdx := strings.Index(remainingHttpReq, "\r\n\r\n")
	headersPart := remainingHttpReq[:endHeadersIdx]

	headersLine := strings.Split(strings.TrimPrefix(headersPart, "\r\n"), "\r\n")

	headers := make(map[string]string, len(headersLine))
	for _, s := range headersLine {
		firstSepIdx := strings.Index(s, ":")
		headers[s[:firstSepIdx]] = strings.TrimSpace(s[firstSepIdx+1:])
	}

	bodyLine := remainingHttpReq[endHeadersIdx+4:] // 4 here is the \r\n\r\n found at the end of headers

	return &reqProps{
		method: httpMethod,
		request: &reqPath{
			path:   path,
			params: nil,
		},
		headers: headers,
		body:    []byte(bodyLine),
	}, nil
}

func buildHttpResponse(status int, headers map[string]string, body string) []byte {
	statusLine := fmt.Sprintf("%s %d %s%s", HttpVersion, status, codeToReason[status], HttpPartSeperator)

	var headerPart string
	if len(headers) == 0 {
		headerPart = HttpPartSeperator
	} else {
		for k, header := range headers {
			headerPart += k + ":" + header + HttpPartSeperator
		}
		headerPart += HttpPartSeperator
	}

	res := fmt.Sprintf("%s%s%s", statusLine, headerPart, body)

	return []byte(res)
}
