package smsmts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSendSMS_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			t.Errorf("Invalid auth header")
		}

		body, _ := io.ReadAll(r.Body)
		var batch SubmitBatch
		json.Unmarshal(body, &batch)

		response := SendResponse{
			Status:      0,
			Description: "Success",
			Data: struct {
				SubmitResults []struct {
					MsID      string `json:"msid"`
					MessageID int    `json:"messageID"`
					Code      string `json:"code"`
				} `json:"submitResults"`
			}{
				SubmitResults: []struct {
					MsID      string `json:"msid"`
					MessageID int    `json:"messageID"`
					Code      string `json:"code"`
				}{
					{"79001234567", 1001, "OK"},
					{"79007654321", 1002, "OK"},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Save original values and restore after test
	originalEndpoint := SendMessageEndpoint
	SendMessageEndpoint = server.URL
	defer func() { SendMessageEndpoint = originalEndpoint }()

	batch := &SubmitBatch{
		Naming: "test",
		Submits: []SubmitMsg{
			{MsID: "79001234567", Message: "Hello"},
			{MsID: "79007654321", Message: "World"},
		},
	}

	err := SendSMS(batch, "valid-token")
	if err != nil {
		t.Fatalf("SendSMS failed: %v", err)
	}

	if batch.Submits[0].MessageID != 1001 || batch.Submits[1].MessageID != 1002 {
		t.Errorf("Message IDs not set correctly")
	}
}

func TestSendSMS_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := SendResponse{
			Status:      1,
			Description: "Invalid token",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	originalEndpoint := SendMessageEndpoint
	SendMessageEndpoint = server.URL
	defer func() { SendMessageEndpoint = originalEndpoint }()

	batch := &SubmitBatch{
		Submits: []SubmitMsg{{MsID: "79001234567", Message: "Test"}},
	}

	err := SendSMS(batch, "invalid-token")
	if err == nil {
		t.Fatal("Expected error")
	}
	if err.Error() != "error: Invalid token" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGetSMSStatuses_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"code":       0,
			"description": "Success",
			"data": []map[string]interface{}{
				{
					"messageID": 31227513,
					"statuses": []map[string]interface{}{
						{
							"msid":   "9222695251",
							"status": "NotSent",
							"date":   "2025-12-09T05:19:02Z",
							"userDeliveryDate": "",
							"partCount": 1,
							"isViber": false,
							"trafficPatternType": "Unknown",
							"cost": 0.0,
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	originalEndpoint := MessageStatusEndpointTempl
	MessageStatusEndpointTempl = server.URL + "?messageIDs=%s"
	defer func() { MessageStatusEndpointTempl = originalEndpoint }()

	statuses, err := GetSMSStatuses([]int{31227513}, "test-token")
	if err != nil {
		t.Fatalf("GetSMSStatuses failed: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 status, got %d", len(statuses))
	}

	if statuses[0].MessageID != "31227513" || statuses[0].Status != "NotSent" {
		t.Errorf("Unexpected status: %+v", statuses[0])
	}
}

func TestGetSMSStatuses_Empty(t *testing.T) {
	statuses, err := GetSMSStatuses([]int{}, "token")
	if err != nil {
		t.Fatalf("GetSMSStatuses with empty list should not error: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("Expected empty statuses, got %d", len(statuses))
	}
}


func TestSendSMS_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	originalEndpoint := SendMessageEndpoint
	SendMessageEndpoint = server.URL
	defer func() { SendMessageEndpoint = originalEndpoint }()

	batch := &SubmitBatch{
		Submits: []SubmitMsg{{MsID: "79001234567", Message: "Test"}},
	}

	err := SendSMS(batch, "token")
	if err == nil {
		t.Fatal("Expected HTTP error")
	}
}

func TestGetSMSStatuses_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	originalEndpoint := MessageStatusEndpointTempl
	MessageStatusEndpointTempl = server.URL + "?messageIDs=%s"
	defer func() { MessageStatusEndpointTempl = originalEndpoint }()

	_, err := GetSMSStatuses([]int{1001}, "token")
	if err == nil {
		t.Fatal("Expected HTTP error")
	}
}

func TestSendSMS_NetworkTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	originalEndpoint := SendMessageEndpoint
	originalTimeout := QueryTimeoutSec
	SendMessageEndpoint = server.URL
	QueryTimeoutSec = 0 // 0 second timeout for immediate failure
	defer func() {
		SendMessageEndpoint = originalEndpoint
		QueryTimeoutSec = originalTimeout
	}()

	batch := &SubmitBatch{
		Submits: []SubmitMsg{{MsID: "79001234567", Message: "Test"}},
	}

	err := SendSMS(batch, "token")
	if err == nil {
		t.Fatal("Expected timeout error")
	}
}
