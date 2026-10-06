package logging

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteJSONString(t *testing.T) {
	for _, in := range []string{
		"plain",
		`quote " and backslash \`,
		"newline\nreturn\rtab\t",
		"control \x00\x01\x1f and DEL \x7f",
		"line separators   ",
		"html <b>&</b>",
		"unicode ünïcödé 神経",
	} {
		var buf bytes.Buffer
		buf.WriteByte('"')
		if _, err := writeJSONString(&buf, in); err != nil {
			t.Fatal(err)
		}
		buf.WriteByte('"')
		var out string
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatalf("%q encoded as invalid JSON %s: %v", in, buf.String(), err)
		}
		if out != in {
			t.Fatalf("round trip %q -> %s -> %q", in, buf.String(), out)
		}
		if bytes.ContainsAny(buf.Bytes(), "\n\r") {
			t.Fatalf("%q encoded with a raw line break: %s", in, buf.String())
		}
	}

	var buf bytes.Buffer
	buf.WriteByte('"')
	writeJSONString(&buf, "bad \xff utf8")
	buf.WriteByte('"')
	var out string
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil || out != "bad \ufffd utf8" {
		t.Fatalf("invalid UTF-8 encoded as %s (err=%v), want U+FFFD", buf.String(), err)
	}
}
