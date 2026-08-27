package runv4

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/itouakirai/mp4ff/mp4"
	"github.com/schollz/progressbar/v3"
)

func TestFetchTemplateParsesValidKeyServerResponse(t *testing.T) {
	ctx := make([]byte, 0x8000)
	state := make([]byte, 0x2004)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("adamId") != "123" || request.URL.Query().Get("uri") != "skd://example/key" {
			t.Fatalf("unexpected key-server request: %s", request.URL.String())
		}
		_, _ = fmt.Fprintf(writer, `{"ctx":%q,"state":%q,"rcx":"0x1","rax":"0x2","rdx":"0x3","r9":"0x4","rbp":"0x5"}`,
			base64.StdEncoding.EncodeToString(ctx), base64.StdEncoding.EncodeToString(state))
	}))
	defer server.Close()

	template, err := fetchTemplate(strings.TrimPrefix(server.URL, "http://"), "123", "skd://example/key")
	if err != nil {
		t.Fatalf("fetch template: %v", err)
	}
	if template.entry.rcx != 1 || template.entry.rax != 2 || len(template.ctx) != len(ctx) {
		t.Fatalf("unexpected parsed template: %+v", template.entry)
	}
}

func TestFetchTemplateDoesNotExposeKeyServerBodyOnFailure(t *testing.T) {
	const secret = "contentKey=should-not-be-reported"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(secret))
	}))
	defer server.Close()

	_, err := fetchTemplate(strings.TrimPrefix(server.URL, "http://"), "123", "skd://example/key")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("key-server failure leaked a response body: %v", err)
	}
}

func TestFetchTemplateRejectsShortStateWithoutPanicking(t *testing.T) {
	ctx := make([]byte, 0x8000)
	state := make([]byte, 0x2000)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = fmt.Fprintf(writer, `{"ctx":%q,"state":%q,"rcx":"0x1","rax":"0x2","rdx":"0x3","r9":"0x4","rbp":"0x5"}`,
			base64.StdEncoding.EncodeToString(ctx), base64.StdEncoding.EncodeToString(state))
	}))
	defer server.Close()

	if _, err := fetchTemplate(strings.TrimPrefix(server.URL, "http://"), "123", "skd://example/key"); err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("expected an invalid-state error, got %v", err)
	}
}

func TestDownloadWithResumeRetriesInterruptedTransfer(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			writer.Header().Set("Content-Length", "4")
			_, _ = writer.Write([]byte("ab"))
			return
		}
		if request.Header.Get("Range") != "bytes=2-" {
			t.Fatalf("expected range retry, got %q", request.Header.Get("Range"))
		}
		writer.Header().Set("Content-Length", "2")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write([]byte("cd"))
	}))
	defer server.Close()

	bar := progressbar.NewOptions64(4, progressbar.OptionSetWriter(io.Discard))
	body, err := downloadWithResume(context.Background(), server.Client(), server.URL, make(http.Header), 4, bar, &resumeProgressWriter{})
	if err != nil {
		t.Fatalf("download with resume: %v", err)
	}
	if got := body.String(); got != "abcd" {
		t.Fatalf("unexpected resumed content: %q", got)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two requests, got %d", requests.Load())
	}
}

func TestDecryptFragmentReportsMissingTrackInfo(t *testing.T) {
	fragment, err := mp4.CreateFragment(1, 42)
	if err != nil {
		t.Fatalf("create fragment: %v", err)
	}
	if err := DecryptFragment(fragment, map[uint32]mp4.DecryptTrackInfo{}, &template{}); err == nil || !strings.Contains(err.Error(), "could not find decryption info") {
		t.Fatalf("expected track-info error, got %v", err)
	}
}
