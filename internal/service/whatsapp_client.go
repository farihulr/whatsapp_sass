package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WhatsAppClient menangani komunikasi dengan Meta WhatsApp Cloud API.
type WhatsAppClient struct {
	phoneNumberID string
	accessToken   string
	apiVersion    string
	httpClient    *http.Client
}

// NewWhatsAppClient membuat WhatsApp API client.
func NewWhatsAppClient(
	phoneNumberID string,
	accessToken string,
	apiVersion string,
) (*WhatsAppClient, error) {

	phoneNumberID = strings.TrimSpace(phoneNumberID)
	accessToken = strings.TrimSpace(accessToken)
	apiVersion = strings.TrimSpace(apiVersion)

	if phoneNumberID == "" {
		return nil, fmt.Errorf("WHATSAPP_PHONE_NUMBER_ID belum diatur")
	}

	if accessToken == "" {
		return nil, fmt.Errorf("WHATSAPP_ACCESS_TOKEN belum diatur")
	}

	if apiVersion == "" {
		return nil, fmt.Errorf("WHATSAPP_API_VERSION belum diatur")
	}

	apiVersion = strings.TrimPrefix(apiVersion, "/")

	return &WhatsAppClient{
		phoneNumberID: phoneNumberID,
		accessToken:   accessToken,
		apiVersion:    apiVersion,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, nil
}

// SendTextMessageRequest adalah request untuk mengirim pesan teks.
type SendTextMessageRequest struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

// WhatsAppAPIRequest adalah struktur request ke Meta API.
type WhatsAppAPIRequest struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type,omitempty"`
	To               string `json:"to"`
	Type             string `json:"type"`

	Text *WhatsAppText `json:"text,omitempty"`
}

type WhatsAppText struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

// WhatsAppAPIResponse adalah response dari Meta API.
type WhatsAppAPIResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input string `json:"input"`
		WaID  string `json:"wa_id"`
	} `json:"contacts"`

	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

// WhatsAppAPIError merepresentasikan error dari Meta.
type WhatsAppAPIError struct {
	Message      string `json:"message"`
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	FBTraceID    string `json:"fbtrace_id"`
}

type WhatsAppAPIErrorResponse struct {
	Error WhatsAppAPIError `json:"error"`
}

// SendTextMessage mengirim pesan teks melalui WhatsApp Cloud API.
func (c *WhatsAppClient) SendTextMessage(
	ctx context.Context,
	input SendTextMessageRequest,
) (*WhatsAppAPIResponse, error) {

	if strings.TrimSpace(input.To) == "" {
		return nil, fmt.Errorf("nomor penerima tidak boleh kosong")
	}

	if strings.TrimSpace(input.Body) == "" {
		return nil, fmt.Errorf("isi pesan tidak boleh kosong")
	}

	payload := WhatsAppAPIRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               input.To,
		Type:             "text",
		Text: &WhatsAppText{
			PreviewURL: false,
			Body:       input.Body,
		},
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat JSON request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s/messages",
		c.apiVersion,
		c.phoneNumberID,
	)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat HTTP request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi WhatsApp API: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("gagal membaca response WhatsApp API: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiError WhatsAppAPIErrorResponse

		if err := json.Unmarshal(responseBody, &apiError); err == nil &&
			apiError.Error.Message != "" {
			return nil, fmt.Errorf(
				"WhatsApp API error (HTTP %d, code %d): %s",
				resp.StatusCode,
				apiError.Error.Code,
				apiError.Error.Message,
			)
		}

		return nil, fmt.Errorf(
			"WhatsApp API mengembalikan HTTP %d",
			resp.StatusCode,
		)
	}

	var result WhatsAppAPIResponse

	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("gagal membaca JSON response: %w", err)
	}

	if len(result.Messages) == 0 || result.Messages[0].ID == "" {
		return nil, fmt.Errorf("WhatsApp API tidak mengembalikan message ID")
	}

	return &result, nil
}
