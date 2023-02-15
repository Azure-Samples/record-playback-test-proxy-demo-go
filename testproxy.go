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
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type testProxyHeader struct {
	RecordingIdHeader          string
	RecordingModeHeader        string
	RecordingUpstreamURIHeader string
}

var TestProxyHeader = testProxyHeader{
	RecordingIdHeader:          "x-recording-id",
	RecordingModeHeader:        "x-recording-mode",
	RecordingUpstreamURIHeader: "x-recording-upstream-base-uri",
}

// Maintain an http client for POST-ing to the test proxy to start and stop recording.
// For your test client, you can either maintain the lack of certificate validation (the test-proxy
// is making real HTTPS calls, so if your actual api call is having cert issues, those will still surface.
var client = http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

// TestProxy encapsulates variables that store values
// related to the test proxy, such as connection host (localhost),
// connection port (5001), and mode (record/playback).
type TestProxyVariable struct {
	Client        *http.Client
	Host          string
	Port          int
	Mode          string
	RecordingId   string
	RecordingPath string
}

func NewTestProxy() *TestProxyVariable {
	return &TestProxyVariable{
		Client: &client,
	}
}

func (tpv TestProxyVariable) host() string {
	return fmt.Sprintf("%s:%d", tpv.Host, tpv.Port)
}

func (tpv TestProxyVariable) scheme() string {
	if tpv.Port == 5001 {
		return "https"
	}
	return "http"
}

func (tpv TestProxyVariable) baseURL() string {
	return fmt.Sprintf("%s://%s:%d", tpv.scheme(), tpv.Host, tpv.Port)
}

func (tpv *TestProxyVariable) Do(req *http.Request) (resp *http.Response, err error) {
	oriSchema := req.URL.Scheme
	oriHost := req.URL.Host
	req.URL.Scheme = tpv.scheme()
	req.URL.Host = tpv.host()
	req.Host = tpv.host()

	// replace request target to use test proxy
	req.Header.Set(TestProxyHeader.RecordingUpstreamURIHeader, fmt.Sprintf("%v://%v", oriSchema, oriHost))
	req.Header.Set(TestProxyHeader.RecordingModeHeader, tpv.Mode)
	req.Header.Set(TestProxyHeader.RecordingIdHeader, tpv.RecordingId)

	resp, err = tpv.Client.Do(req)

	// for any lro operation, need to change back to the original target to prevent
	if resp != nil {
		resp.Request.URL.Scheme = oriSchema
		resp.Request.URL.Host = oriHost
	}

	return resp, err
}

func GetClientOption(tpv *TestProxyVariable) (*arm.ClientOptions, error) {
	options := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: tpv,
		},
	}

	return options, nil
}

func GetCurrentDirectory() string {
	root, err := filepath.Abs(".")
	if err != nil {
		log.Fatal(err)
	}
	return root
}

func getRecordingFilePath(recordingPath string, t *testing.T) string {
	return path.Join(recordingPath, "recordings", t.Name()+".json")
}

// StartTextProxy() will initiate a record or playback session by POST-ing a request
// to a running instance of the test proxy. The test proxy will return a recording ID
// value in the response header, which we pull out and save as 'x-recording-id'.
func StartTestProxy(t *testing.T, tpv *TestProxyVariable) error {
	if tpv == nil {
		return fmt.Errorf("TestProxy not empty")
	}

	recordingFilePath := getRecordingFilePath(tpv.RecordingPath, t)
	url := fmt.Sprintf("%s/%s/start", tpv.baseURL(), tpv.Mode)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	marshalled, err := json.Marshal(map[string]string{"x-recording-file": recordingFilePath})
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(marshalled))
	req.ContentLength = int64(len(marshalled))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	recId := resp.Header.Get(TestProxyHeader.RecordingIdHeader)
	if recId == "" {
		b, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			return err
		}
		return fmt.Errorf("recording ID was not returned by the response. Response body: %s", b)
	}

	tpv.RecordingId = recId

	// Unmarshal any variables returned by the proxy
	var m map[string]interface{}
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if len(body) > 0 {
		err = json.Unmarshal(body, &m)
		if err != nil {
			return err
		}
	}

	return nil
}

// StopTextProxy() instructs the test proxy to stop recording or stop playback,
// depending on the mode it is running in. The instruction to stop is made by
// POST-ing a request to a running instance of the test proxy. We pass in the recording
// ID and a directive to save the recording (when recording is running).
//
// **Note that if you skip this step your recording WILL NOT be saved.**
func StopTestProxy(t *testing.T, tpv *TestProxyVariable) error {
	if tpv == nil {
		return fmt.Errorf("TestProxy not empty")
	}

	url := fmt.Sprintf("%v/%v/stop", tpv.baseURL(), tpv.Mode)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set(TestProxyHeader.RecordingIdHeader, tpv.RecordingId)

	resp, err := client.Do(req)
	if resp.StatusCode != 200 {
		b, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err == nil {
			return fmt.Errorf("proxy did not stop the recording properly: %s", string(b))
		}
		return fmt.Errorf("proxy did not stop the recording properly: %s", err.Error())
	}
	_ = resp.Body.Close()
	return err
}
