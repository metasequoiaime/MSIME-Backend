package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativeProcessContractAndFailures(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "helper.go")
	binary := filepath.Join(dir, "helper")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	code := `package main
import("fmt";"os";"path/filepath";"strings";"time")
func main(){switch os.Args[1] {
case "success":fmt.Print("{\"result\":\"ok\"}")
case "snapshot":b,e:=os.ReadFile(filepath.Join(os.Args[2],"snapshot.jsonl"));if e!=nil||string(b)!="fixture"{os.Exit(2)};fmt.Print("{}")
case "oversize":fmt.Print(strings.Repeat("x",1048577))
case "malformed":fmt.Print("not json")
case "array":fmt.Print("[]")
case "exit":fmt.Fprint(os.Stderr,"private provider detail");os.Exit(2)
case "wait":time.Sleep(time.Minute)
default:fmt.Printf("{\"error\":%q}",os.Args[1])}}
`
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "build", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for _, tc := range []struct {
		mode string
		want error
	}{{"success", nil}, {"snapshot", nil}, {"resources_unavailable", ErrUnavailable}, {"invalid_request", ErrInvalid}, {"invalid_dictionary_entry", ErrInvalid}, {"unknown", ErrFailure}, {"oversize", ErrFailure}, {"malformed", ErrFailure}, {"array", ErrFailure}, {"exit", ErrFailure}} {
		t.Run(tc.mode, func(t *testing.T) {
			cfg := Config{Binary: binary, Resources: tc.mode}
			var snapshot func(context.Context, io.Writer) error
			if tc.mode == "snapshot" {
				snapshot = func(_ context.Context, w io.Writer) error { _, e := io.WriteString(w, "fixture"); return e }
			}
			out, err := cfg.QuerySnapshot(t.Context(), map[string]string{"operation": "test"}, snapshot)
			if !errors.Is(err, tc.want) {
				t.Fatal(tc.mode, err)
			}
			if err == nil && len(out) == 0 {
				t.Fatal("missing response")
			}
		})
	}
	cfg := Config{Binary: binary, Resources: "wait"}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := cfg.Query(ctx, struct{}{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("process not canceled", err)
	}
	for _, input := range []any{make(chan int), strings.Repeat("a", 65537)} {
		if _, err := cfg.Query(t.Context(), input); err != ErrInvalid {
			t.Fatal(err)
		}
	}
	if _, err := (Config{}).Query(t.Context(), nil); err != ErrUnavailable {
		t.Fatal(err)
	}
	if _, err := (Config{Binary: filepath.Join(dir, "missing")}).Query(t.Context(), nil); err != ErrFailure {
		t.Fatal(err)
	}
	sentinel := errors.New("snapshot failed")
	if _, err := cfg.QuerySnapshot(t.Context(), nil, func(context.Context, io.Writer) error { return sentinel }); err != sentinel {
		t.Fatal(err)
	}
	for i := 0; i < cap(nativeSlots); i++ {
		nativeSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(nativeSlots); i++ {
			<-nativeSlots
		}
	}()
	ctx, cancel = context.WithCancel(t.Context())
	cancel()
	if _, err := cfg.Query(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestNativeConfigAndBoundedOutput(t *testing.T) {
	for _, cfg := range []Config{{Binary: "relative"}, {Resources: "relative"}} {
		if cfg.Validate() == nil {
			t.Fatal("relative path accepted")
		}
	}
	if err := (Config{Binary: filepath.Join(t.TempDir(), "binary"), Resources: t.TempDir()}).Validate(); err != nil {
		t.Fatal(err)
	}
	var out boundedOutput
	for _, chunk := range []string{strings.Repeat("a", 1<<20), "more", "again"} {
		n, err := out.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatal(n, err)
		}
	}
	if out.Len() != 1<<20 || !out.exceeded {
		t.Fatal("unbounded process output")
	}
}
