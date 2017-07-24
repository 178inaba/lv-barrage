package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	uuid "github.com/satori/go.uuid"
)

func TestSaveSession(t *testing.T) {
	session := "foobar"
	fp := filepath.Join(os.TempDir(), uuid.NewV4().String())
	if err := saveSession(session, fp); err != nil {
		t.Fatalf("should not be fail: %v", err)
	}
	defer func() {
		if err := os.Remove(fp); err != nil {
			t.Fatalf("should not be fail: %v", err)
		}
	}()
	f, err := os.Open(fp)
	if err != nil {
		t.Fatalf("should not be fail: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("should not be fail: %v", err)
		}
	}()
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("should not be fail: %v", err)
	}
	mode := fi.Mode()
	if mode != 0600 {
		t.Fatalf("want %#o but %#o", 0600, mode)
	}
	bs, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatalf("should not be fail: %v", err)
	}
	if string(bs) != session {
		t.Fatalf("want %q but %q", session, string(bs))
	}

	if err := saveSession("", filepath.Join(fp, "foo")); err == nil {
		t.Fatalf("should be fail: %v", err)
	}

	errFP := filepath.Join(os.TempDir(), uuid.NewV4().String(), uuid.NewV4().String())
	if err := os.MkdirAll(errFP, 0700); err != nil {
		t.Fatalf("should not be fail: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(filepath.Dir(errFP)); err != nil {
			t.Fatalf("should not be fail: %v", err)
		}
	}()
	if err := saveSession("", errFP); err == nil {
		t.Fatalf("should be fail: %v", err)
	}
}
