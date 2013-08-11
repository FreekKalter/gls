package main

import (
	"errors"
	"testing"
)

var hmTests = []struct {
	in  int64
	out string
	err error
}{
	{6, "6.0 b", nil},
	{10000, "9.8 kb", nil},
	{1000, "1000.0 b", nil},
	{-10, "", errors.New("negative input")},
}

func TestHumanReadable(t *testing.T) {
	for _, hm := range hmTests {
		out, err := humanReadable(hm.in)
		if out != hm.out {
			t.Fatal("Unexpected output:", out, "!=", hm.out)
		}
		if err != hm.err {
			if err.Error() != hm.err.Error() {
				t.Fatal("Unexpected error:", err.Error(), "!=", hm.err.Error())
			}
		}
	}
}
