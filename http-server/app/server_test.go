package main

import (
	"fmt"
	"net"
	"testing"
)

func TestRootRegisterHandler(t *testing.T) {

	t.Run("Should be able to register a root handler", func(t *testing.T) {

		s := &server{
			listener: nil,
			req:      nil,
			paths:    create(),
		}

		err := s.registerHandler("", func(props *reqProps, conn net.Conn) {
		})

		if err != nil {
			t.Log("There should be no error")
			t.Fail()
		}

		if s.paths.root == nil {
			t.Log("There should have been a root creation")
			t.Fail()
		}

		if s.paths.root.childPaths == nil {
			t.Log("There should have been a map creation")
			t.Fail()
		}
	})
}

func TestRegisterHandler(t *testing.T) {

	s := &server{
		listener: nil,
		req:      nil,
		paths:    create(),
	}

	rootCreation(s)

	t.Run("Should be able to register a handler for direct route", func(t *testing.T) {

		err := s.registerHandler("echo", func(props *reqProps, conn net.Conn) {
		})

		if err != nil {
			t.Log("There should be no error")
			t.Fail()
			t.Cleanup(func() {
				serverCleanup(s)
			})
		}

		if p, ok := s.paths.root.childPaths["echo"]; !ok {
			t.Log("There should be a root for echo")
			t.Fail()

			if p.childPaths == nil {
				t.Log("There should have been a map creation")
				t.Fail()
			}
		}

		t.Cleanup(func() {
			serverCleanup(s)
		})
	})

	t.Run("Should be able to register a handler for template route", func(t *testing.T) {

		err := s.registerHandler("echo/{str}", func(props *reqProps, conn net.Conn) {
		})

		if err != nil {
			t.Log("There should be no error")
			t.Fail()
			t.Cleanup(func() {
				serverCleanup(s)
			})
		}

		if p, ok := s.paths.root.childPaths["echo"]; !ok {
			t.Log("There should be a root for echo")
			t.Fail()

			if p.childPaths == nil {
				t.Log("There should have been a map creation")
				t.Fail()
			}

			if c, cok := p.childPaths["{str}"]; !cok {
				t.Log("There should be a root for a template inside echo")
				t.Fail()

				if c.childPaths == nil {
					t.Log("There should have been a map creation")
					t.Fail()
				}
			}
		}

		t.Cleanup(func() {
			serverCleanup(s)
		})
	})
}

func TestHandle(t *testing.T) {

	s := &server{
		listener: nil,
		req:      nil,
		paths:    create(),
	}

	rootCreation(s)

	t.Run("Should be handle routes for templates", func(t *testing.T) {

		err := s.registerHandler("echo/{str}", func(props *reqProps, conn net.Conn) {

			if props.request.params[0] != "abc" {
				t.Log("there as been a error in parsing")
				t.Fail()
			}

		})

		if err != nil {
			t.Log("There should be no error")
			t.Fail()
			t.Cleanup(func() {
				serverCleanup(s)
			})
		}

		s.req = &reqProps{
			method: "GET",
			request: &reqPath{
				path:   "echo/abc",
				params: nil,
			},
			headers: make(map[string]string),
		}

		handleErr := s.handle(nil)

		if handleErr != nil {
			t.Log("There should be no error for handling request")
			t.Fail()
			t.Cleanup(func() {
				serverCleanup(s)
			})
		}
	})
}

func TestReadRequest(t *testing.T) {

	t.Run("Should be able to read a request with headers", func(t *testing.T) {
		request := "GET /user-agent HTTP/1.1\r\nHost: localhost:4221\r\nUser-Agent: foobar/1.2.3\r\nAccept: */*\r\n\r\n"

		props, err := readRequest([]byte(request))

		if err != nil {
			t.Log("There should be no error")
			t.Fail()
		}

		if props.request.path != "user-agent" {
			t.Log("Request should be user-agent")
			t.Fail()
		}

		if len(props.headers) != 3 {
			t.Log("Should have 3 headers")
			t.Fail()
		}

		value := props.headers["User-Agent"]

		if value != "foobar/1.2.3" {
			t.Log("Header value should be foobar/1.2.3")
			t.Fail()
		}
	})

	t.Run("Should be able to read a request with body", func(t *testing.T) {
		request := "POST /user-agent HTTP/1.1\r\nHost: localhost:4221\r\nUser-Agent: foobar/1.2.3\r\nAccept: */*\r\n\r\n12345"

		props, err := readRequest([]byte(request))

		if err != nil {
			t.Log("There should be no error")
			t.Fail()
		}

		expected := []byte("12345")

		if len(props.body) != len(expected) {
			t.Logf("Bodies have diferent sizes, original %d :: expected %d", len(props.body), len(expected))
		}

		for i, b := range props.body {
			if b != expected[i] {
				t.Log(fmt.Sprintf("Different byte original : %c, expected : %c at position %d", b, expected[i], i))
				t.Fail()
			}
		}
	})
}

func serverCleanup(s *server) {
	s.paths = create()
	rootCreation(s)
}

func rootCreation(s *server) {
	err := s.registerHandler("", func(props *reqProps, conn net.Conn) {
	})

	if err != nil {
		fmt.Println("There should be no error")
		panic("root should be able to be registered")
	}
}
