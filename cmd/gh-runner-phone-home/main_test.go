package main

import "testing"

func TestRunRejectsMissingArguments(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("run() succeeded without arguments")
	}
}
