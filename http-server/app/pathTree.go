package main

import (
	"net"
)

type node struct {
	path       string
	template   bool
	childPaths map[string]*node
	handler    func(props *reqProps, conn net.Conn)
}

type tree struct {
	root *node
}

func create() *tree {
	return &tree{root: nil}
}

func (t *tree) addRoot(root string, h func(props *reqProps, conn net.Conn)) *node {

	if t.root != nil {
		panic("you can only register one root path")
	}

	r := &node{
		path:       root,
		template:   false,
		childPaths: make(map[string]*node),
		handler:    h,
	}

	t.root = r

	return r
}

func (n *node) addChild(path string, template bool, h func(props *reqProps, conn net.Conn)) *node {
	if n.childPaths == nil {
		n.childPaths = make(map[string]*node)
	}

	newNode := &node{
		path:       path,
		template:   template,
		childPaths: nil,
		handler:    h,
	}

	if _, ok := n.childPaths[path]; !ok {
		n.childPaths[path] = newNode
	} else {
		panic("this path has already been registered, please verify your application configurations")
	}

	return newNode
}
