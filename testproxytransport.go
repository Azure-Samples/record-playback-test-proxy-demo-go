// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.  All rights reserved.
// ------------------------------------------------------------

package testproxy

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// This is an example integration with the Azure record/playback test proxy,
// which requires a custom implementation of the http pipeline transport used
// by the Azure SDK service clients.
// This implementation assumes the test-proxy is already running.
// Your test framework should start and stop the test-proxy process as needed.

var client = http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

// Derived from policy.Transporter, TestProxyTransport provides custom
// implementations of the abstract methods defined in the base class
// described above in the HTTP Transport section of this article. These
// custom implementations allow us to intercept and reroute app traffic sent
// between an app and Azure to the test proxy.
type TestProxyTransport struct {
	transport   policy.Transporter
	host        string
	port        int
	mode        string
	recordingId string
}

func NewTestProxyTransport(transport policy.Transporter, host string, port int, recordingId string, mode string) *TestProxyTransport {
	return &TestProxyTransport{
		transport:   transport,
		host:        host,
		port:        port,
		recordingId: recordingId,
		mode:        mode,
	}
}

func (tpt *TestProxyTransport) Do(req *http.Request) (resp *http.Response, err error) {

	req.Header.Set("x-recording-id", tpt.recordingId)
	req.Header.Set("x-recording-mode", tpt.mode)

	scheme := req.URL.Scheme
	host := req.URL.Host
	baseUri := fmt.Sprintf("%v://%v", scheme, host)
	req.Header.Set("x-recording-upstream-base-uri", baseUri)

	req.URL.Host = fmt.Sprintf("%v:%v", tpt.host, tpt.port)

	return tpt.transport.Do(req)
}

// TestProxyVariables class	encapsulates variables that store values
// related to the test proxy, such as connection host (localhost),
// connection port (5001), and mode (record/playback).
type TestProxyVariables struct {
	Host        string
	Port        int
	Mode        string
	RecordingId string

	CurrentRecordingPath string
	// Maintain an http client for POST-ing to the test proxy to start and stop recording.
	// For your test client, you can either maintain the lack of certificate validation (the test-proxy
	// is making real HTTPS calls, so if your actual api call is having cert issues, those will still surface.
	HttpClient *http.Client
}

func NewTestProxyVariables(t *testing.T) *TestProxyVariables {
	return &TestProxyVariables{
		HttpClient:           &client,
		CurrentRecordingPath: getRecordingFilePath(t, GetCurrentDirectory()),
	}
}

func GetCurrentDirectory() string {
	root, err := filepath.Abs(".")
	if err != nil {
		log.Fatal(err)
	}
	return root
}

func getRecordingFilePath(t *testing.T, recordingPath string) string {
	return path.Join(recordingPath, "recordings", t.Name()+".json")
}

// StartTextProxy() will initiate a record or playback session by POST-ing a request
// to a running instance of the test proxy. The test proxy will return a recording ID
// value in the response header, which we pull out and save as 'x-recording-id'.
func StartTestProxy(tpv *TestProxyVariables) error {

	url := fmt.Sprintf("https://%v:%v/%v/start", tpv.Host, tpv.Port, tpv.Mode)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	marshalled, err := json.Marshal(map[string]string{"x-recording-file": tpv.CurrentRecordingPath})
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(marshalled))
	req.ContentLength = int64(len(marshalled))

	resp, err := tpv.HttpClient.Do(req)
	if err != nil {
		return err
	}

	tpv.RecordingId = resp.Header.Get("x-recording-id")

	return nil
}

// StopTextProxy() instructs the test proxy to stop recording or stop playback,
// depending on the mode it is running in. The instruction to stop is made by
// POST-ing a request to a running instance of the test proxy. We pass in the recording
// ID and a directive to save the recording (when recording is running).
//
// **Note that if you skip this step your recording WILL NOT be saved.**
func StopTestProxy(tpv *TestProxyVariables) error {

	url := fmt.Sprintf("https://%v:%v/%v/stop", tpv.Host, tpv.Port, tpv.Mode)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("x-recording-id", tpv.RecordingId)
	req.Header.Set("x-recording-save", strconv.FormatBool(true))

	_, err = tpv.HttpClient.Do(req)

	return err
}
