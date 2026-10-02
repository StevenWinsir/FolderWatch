package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestBoundedRingAndDebug(t *testing.T) {
	l, err := Open("", t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	l.Record("debug", "hidden")
	if len(l.Entries()) != 0 {
		t.Fatal("debug not filtered")
	}
	for i := 0; i < Capacity+1; i++ {
		l.Record("warning", fmt.Sprint(i))
	}
	entries := l.Entries()
	if len(entries) != Capacity || entries[0].Message != "1" {
		t.Fatal(entries)
	}
	entries[0].Message = "mutated"
	if l.Entries()[0].Message == "mutated" {
		t.Fatal("shared ring")
	}
	l.Record("info", strings.Repeat("世", MaxMessageBytes))
	if len(l.Entries()[Capacity-1].Message) > MaxMessageBytes+20 {
		t.Fatal("unbounded record")
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.Record("warning", "concurrent")
				_ = l.Entries()
			}
		}()
	}
	wg.Wait()
}

func TestPrivateLogBoundariesAndCap(t *testing.T) {
	root, logs := t.TempDir(), t.TempDir()
	if _, err := Open(filepath.Join(root, "loop.log"), root, true); err == nil {
		t.Fatal("log feedback allowed")
	}
	path := filepath.Join(logs, "debug.jsonl")
	l, err := Open(path, root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.Record("debug", "event=changes generation=2\nforged")
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "\n") != 1 {
		t.Fatal(string(data), err)
	}
	if _, err := Open(path, root, true); err == nil {
		t.Fatal("overwrote existing log")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("private mode", info, err)
	}
	l.written = MaxFileBytes
	l.Record("warning", "limit")
	if l.Err() == nil || len(l.Entries()) != 3 {
		t.Fatal("cap did not warn")
	}
	for i := 0; i < 100; i++ {
		l.Record("warning", "after cap")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("continued growing file")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectSymlinkLogAndResolvedRootAlias(t *testing.T) {
	// On case-insensitive hosts, lexical casing must not bypass the root fence.
	caseParent := t.TempDir()
	caseRoot := filepath.Join(caseParent, "WatchedRoot")
	if err := os.Mkdir(caseRoot, 0700); err != nil {
		t.Fatal(err)
	}
	caseAlias := filepath.Join(caseParent, "watchedroot")
	if _, err := os.Stat(caseAlias); err == nil {
		if _, err := Open(filepath.Join(caseAlias, "case.log"), caseRoot, true); err == nil {
			t.Fatal("case alias feedback")
		}
	}

	root, logs := t.TempDir(), t.TempDir()
	target := filepath.Join(logs, "target")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(logs, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if _, err := Open(link, root, true); err == nil {
		t.Fatal("followed link")
	}
	alias := filepath.Join(logs, "root-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(alias, "loop"), root, true); err == nil {
		t.Fatal("resolved root feedback")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "keep" {
		t.Fatal("modified target")
	}
}
