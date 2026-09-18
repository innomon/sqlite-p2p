package main

import (
	"testing"
)

func TestAppVersion(t *testing.T) {
	expected := "0.1.0"
	if Version != expected {
		t.Fatalf("expected version %s, got %s", expected, Version)
	}
}

func TestMainExecution(t *testing.T) {
	main()
}
