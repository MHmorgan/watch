package main

import (
	"fmt"
	"io"
	"time"
)

var screens = map[string]WatchScreen{
	"plain": &plainScreen{},
	"vt100": &vt100Screen{},
}

const (
	CLEAR = "\033[H\033[2J"
	BOLD  = "\033[1m"
	RESET = "\033[0m"
	HIDE  = "\033[?25l"
	SHOW  = "\033[?25h"
)

type WatchScreen interface {
	io.Writer
	Status(string, ...any)
	Name(string)
	Setup()
	Teardown()
}

// -----------------------------------------------------------------------------
// PLAIN SCREEN

type plainScreen struct {
	name   string
	status string
}

func (s *plainScreen) Write(b []byte) (n int, err error) {
	header := fmt.Sprintf("WATCH %s [%s", s.name, timestamp())
	if s.status != "" {
		header += " " + s.status
	}
	header += "]"
	return fmt.Printf("%s\n\n%s", header, b)
}

func (s *plainScreen) Status(txt string, a ...any) {
	s.status = fmt.Sprintf(txt, a...)
}

func (s *plainScreen) Name(name string) {
	s.name = name
}

func (s *plainScreen) Setup() {}

func (s *plainScreen) Teardown() {}

// -----------------------------------------------------------------------------
// VT100 SCREEN

type vt100Screen struct {
	name   string
	status string
}

func (s *vt100Screen) Write(b []byte) (n int, err error) {
	header := fmt.Sprintf("%s%sWATCH %s%s [%s", CLEAR, BOLD, s.name, RESET, timestamp())
	if s.status != "" {
		header += " " + s.status
	}
	header += "]"
	return fmt.Printf("%s\n\n%s", header, b)
}

func (s *vt100Screen) Status(txt string, a ...any) {
	s.status = fmt.Sprintf(txt, a...)
}

func (s *vt100Screen) Name(name string) {
	s.name = name
}

func (s *vt100Screen) Setup() {
	fmt.Print(HIDE)
}

func (s *vt100Screen) Teardown() {
	fmt.Print(SHOW)
}

// -----------------------------------------------------------------------------
// HELPERS

func timestamp() string {
	now := time.Now()
	return now.Format("15:04:05")
}
