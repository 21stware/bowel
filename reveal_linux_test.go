package main

import (
	"slices"
	"testing"
)

// The path goes to the file manager as one file:// URI, escaped, inside
// a one-element array; the startup id after it is empty.
func TestShowItemsArgs(t *testing.T) {
	got := showItemsArgs("/home/a b/.config/bowel/core/config #1,2.yaml")
	want := []string{
		"--session", "--print-reply", "--reply-timeout=5000",
		"--dest=org.freedesktop.FileManager1",
		"/org/freedesktop/FileManager1",
		"org.freedesktop.FileManager1.ShowItems",
		"array:string:file:///home/a%20b/.config/bowel/core/config%20%231%2C2.yaml",
		"string:",
	}
	if !slices.Equal(got, want) {
		t.Errorf("showItemsArgs =\n%q\nwant\n%q", got, want)
	}
}
