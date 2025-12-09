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
		if r.URL.Query().Get("messageIDs") != "1001,1002" {
			t.Errorf("Invalid messageIDs: %s", r.URL.Query().Get("messageIDs"))
		}

		response := StatResponse{
			Status:      0,
			Description: "Success",
			Data: struct {
				MessageID int `json:"messageID"`
				Statuses  []struct {
					MsID   string  `json:"msid"`
					Status string  `json:"status"`
					Cost   float64 `json:"cost"`
				} `json:"statuses"`
			}{
				MessageID: 1001,
				Statuses: []struct {
					MsID   string  `json:"msid"`
					Status string  `json:"status"`
					Cost   float64 `json:"cost"`
				}{
					{"79001234567", StatusDelivered, 1.5},
					{"79007654321", StatusPending, 0},
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

	statuses, err := GetSMSStatuses([]int{1001, 1002}, "valid-token")
	if err != nil {
		t.Fatalf("GetSMSStatuses failed: %v", err)
	}

	if len(statuses) != 2 {
		t.Fatalf("Expected 2 statuses, got %d", len(statuses))
	}

	if statuses[0].Status != StatusDelivered || statuses[1].Status != StatusPending {
		t.Errorf("Unexpected statuses: %v and %v", statuses[0].Status, statuses[1].Status)
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

func TestGetSMSStatus_Single(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := StatResponse{
			Status:      0,
			Description: "Success",
			Data: struct {
				MessageID int `json:"messageID"`
				Statuses  []struct {
					MsID   string  `json:"msid"`
					Status string  `json:"status"`
					Cost   float64 `json:"cost"`
				} `json:"statuses"`
			}{
				MessageID: 1001,
				Statuses: []struct {
					MsID   string  `json:"msid"`
					Status string  `json:"status"`
					Cost   float64 `json:"cost"`
				}{
					{"79001234567", StatusSent, 1.5},
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

	status, err := GetSMSStatus(1001, "valid-token")
	if err != nil {
		t.Fatalf("GetSMSStatus failed: %v", err)
	}

	if status.MessageID != "1001" || status.Status != StatusSent {
		t.Errorf("Unexpected status: %+v", status)
	}
}

func TestGetSMSStatus_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := StatResponse{
			Status:      0,
			Description: "Success",
			Data: struct {
				MessageID int `json:"messageID"`
				Statuses  []struct {
					MsID   string  `json:"msid"`
					Status string  `json:"status"`
					Cost   float64 `json:"cost"`
				} `json:"statuses"`
			}{
				MessageID: 9999,
				Statuses:  []struct {
					MsID   string  `json:"msid"`
					Status string  `json:"status"`
					Cost   float64 `json:"cost"`
				}{},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	originalEndpoint := MessageStatusEndpointTempl
	MessageStatusEndpointTempl = server.URL + "?messageIDs=%s"
	defer func() { MessageStatusEndpointTempl = originalEndpoint }()

	_, err := GetSMSStatus(9999, "valid-token")
	if err == nil {
		t.Fatal("Expected error for not found message")
	}
}

func TestStatusHelpers(t *testing.T) {
	tests := []struct {
		status      string
		isFinal     bool
		isDelivered bool
		isFailed    bool
	}{
		{StatusPending, false, false, false},
		{StatusSending, false, false, false},
		{StatusSent, false, false, false},
		{StatusDelivered, true, true, false},
		{StatusNotDelivered, true, false, true},
		{StatusNotSent, true, false, true},
		{"Unknown", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if IsFinalStatus(tt.status) != tt.isFinal {
				t.Errorf("IsFinalStatus(%q) = %v, want %v", tt.status, IsFinalStatus(tt.status), tt.isFinal)
			}
			if IsDeliveredStatus(tt.status) != tt.isDelivered {
				t.Errorf("IsDeliveredStatus(%q) = %v, want %v", tt.status, IsDeliveredStatus(tt.status), tt.isDelivered)
			}
			if IsFailedStatus(tt.status) != tt.isFailed {
				t.Errorf("IsFailedStatus(%q) = %v, want %v", tt.status, IsFailedStatus(tt.status), tt.isFailed)
			}
		})
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
