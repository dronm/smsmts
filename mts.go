// Package smsmts
package smsmts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// https://support.mts.ru/mts_marketolog/rassilki-po-svoei-baze-pro-i-api-k-nim/dokumentatsiya-rest-api
var (
	SendMessageEndpoint        = "https://api.mts.ru/client-omni-adapter_production/1.0.2/mcom/messageManagement/messages"
	MessageStatusEndpointTempl = "https://api.mts.ru/client-omni-adapter_production/1.0.2/mcom/messageManagement/messages/status?messageIDs=%s"
	QueryTimeoutSec            = 10
)

// Constants for statuses
const (
	StatusPending      = "Pending"
	StatusNotSent      = "NotSent"
	StatusSent         = "Sent"
	StatusSending      = "Sending"
	StatusDelivered    = "Delivered"
	StatusNotDelivered = "NotDelivered"
)

// MessageStatus represents the status of a single message
type MessageStatus struct {
	MessageID string  `json:"message_id"`
	MsID      string  `json:"msid"`   // tel
	Status    string  `json:"status"` // e.g., "Pending", "NotSent", "Delivered", etc.
	Cost      float64 `json:"cost"`
	Error     string  `json:"error,omitempty"`
}

// StatResponse is the API response for status check
type StatResponse struct {
	Status           int      `json:"status"`
	Description      string   `json:"description"`
	ValidationErrors []string `json:"validationErrors"`
	Data             struct {
		MessageID int `json:"messageID"`
		Statuses  []struct {
			MsID   string  `json:"msid"`   // tel
			Status string  `json:"status"` // can be NotSent, Pending, Delivered, etc.
			Cost   float64 `json:"cost"`
		} `json:"statuses"`
	} `json:"data"`
}

// SendResponse is the API response for sending messages
type SendResponse struct {
	Status           int      `json:"status"`
	Description      string   `json:"description"`
	ValidationErrors []string `json:"validationErrors"`
	Data             struct {
		SubmitResults []struct {
			MsID      string `json:"msid"`
			MessageID int    `json:"messageID"`
			Code      string `json:"code"`
		} `json:"submitResults"`
	} `json:"data"`
}

// SubmitMsg represents a single message to send
type SubmitMsg struct {
	MsID      string `json:"msid"`
	Message   string `json:"message"`
	SendError bool   `json:"send_error,omitempty"` // set after query
	MessageID int    `json:"message_id,omitempty"` // set after query
}

// SubmitBatch represents a batch of messages to send
type SubmitBatch struct {
	Submits []SubmitMsg `json:"submits"`
	Naming  string      `json:"naming"`
}

// SendSMS sends a batch of SMS messages
func SendSMS(batch *SubmitBatch, token string) error {
	payload, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("json.Marshal(): %v", err)
	}

	client := &http.Client{
		Timeout: time.Duration(QueryTimeoutSec) * time.Second,
	}
	req, err := http.NewRequest(
		"POST",
		SendMessageEndpoint,
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("NewRequest(): %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("client.Do(): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status: %d with token: %s", resp.StatusCode, token)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("io.ReadAll(): %v", err)
	}

	var respStruct SendResponse
	if err := json.Unmarshal(body, &respStruct); err != nil {
		return fmt.Errorf("json.Unmarshal(): %v", err)
	}

	// iterate submit result and set message data
	for _, sbRes := range respStruct.Data.SubmitResults {
		// set result and message ID
		// find by MsID (tel)
		for i := range batch.Submits {
			if batch.Submits[i].MsID == sbRes.MsID {
				batch.Submits[i].MessageID = sbRes.MessageID
				if sbRes.Code != "OK" {
					batch.Submits[i].SendError = true
				}
				break
			}
		}
	}

	if respStruct.Status != 0 {
		// some error
		return fmt.Errorf("error: %s", respStruct.Description)
	}

	return nil
}

// GetSMSStatuses returns statuses for multiple message IDs
func GetSMSStatuses(messageIDs []int, token string) ([]MessageStatus, error) {
	if len(messageIDs) == 0 {
		return []MessageStatus{}, nil
	}

	// Convert int IDs to strings for URL
	idStrs := make([]string, len(messageIDs))
	for i, id := range messageIDs {
		idStrs[i] = strconv.Itoa(id)
	}
	idsParam := strings.Join(idStrs, ",")

	client := &http.Client{
		Timeout: time.Duration(QueryTimeoutSec) * time.Second,
	}
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf(MessageStatusEndpointTempl, idsParam),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(): %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do(): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var respStruct StatResponse
	if err := json.Unmarshal(body, &respStruct); err != nil {
		return nil, fmt.Errorf("json.Unmarshal(): %v", err)
	}
	if respStruct.Status != 0 {
		// some error
		return nil, fmt.Errorf("error: %s", respStruct.Description)
	}

	// Map statuses to MessageStatus objects
	statuses := make([]MessageStatus, len(respStruct.Data.Statuses))
	for i, stRes := range respStruct.Data.Statuses {
		statuses[i] = MessageStatus{
			MessageID: strconv.Itoa(respStruct.Data.MessageID),
			MsID:      stRes.MsID,
			Status:    stRes.Status,
			Cost:      stRes.Cost,
		}
	}

	return statuses, nil
}

// GetSMSStatus returns status for a single message ID
func GetSMSStatus(messageID int, token string) (*MessageStatus, error) {
	statuses, err := GetSMSStatuses([]int{messageID}, token)
	if err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return nil, fmt.Errorf("no status found for message ID %d", messageID)
	}
	return &statuses[0], nil
}

// IsFinalStatus checks if a status is final (won't change)
func IsFinalStatus(status string) bool {
	switch status {
	case StatusDelivered, StatusNotDelivered, StatusNotSent:
		return true
	default:
		return false
	}
}

// IsDeliveredStatus checks if a status indicates successful delivery
func IsDeliveredStatus(status string) bool {
	return status == StatusDelivered
}

// IsFailedStatus checks if a status indicates failure
func IsFailedStatus(status string) bool {
	return status == StatusNotDelivered || status == StatusNotSent
}
