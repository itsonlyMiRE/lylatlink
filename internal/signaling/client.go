package signaling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lylatlink/internal/slp"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

type MatchStartRequest struct {
	MatchID          string   `json:"matchId"`
	SessionID        string   `json:"sessionId"`
	GameNumber       uint32   `json:"gameNumber"`
	TiebreakerNumber uint32   `json:"tiebreakerNumber"`
	PlayerCodes      []string `json:"playerCodes"`
	ClientNonce      string   `json:"clientNonce"`
}

type MatchEndRequest struct {
	MatchID     string `json:"matchId"`
	ClientNonce string `json:"clientNonce"`
	Event       string `json:"event"`
}

type StartResponse struct {
	Status          string          `json:"status"`
	RoomToken       string          `json:"roomToken,omitempty"`
	SignalURL       string          `json:"signalUrl,omitempty"`
	Initiator       bool            `json:"initiator,omitempty"`
	TurnCredentials TurnCredentials `json:"turnCredentials,omitempty"`
}

type TurnCredentials struct {
	URLs       []string `json:"urls,omitempty"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 40 * time.Second,
		},
	}
}

func NewStartRequest(match *slp.Match, nonce string) MatchStartRequest {
	return MatchStartRequest{
		MatchID:          match.MatchID,
		SessionID:        match.SessionID,
		GameNumber:       match.GameNumber,
		TiebreakerNumber: match.TiebreakerNumber,
		PlayerCodes:      match.PlayerCodes,
		ClientNonce:      nonce,
	}
}

func (c *Client) StartMatch(ctx context.Context, req MatchStartRequest) (*StartResponse, error) {
	var resp StartResponse
	if err := c.post(ctx, "/match/start", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) EndMatch(ctx context.Context, matchID, nonce string) error {
	req := MatchEndRequest{
		MatchID:     matchID,
		ClientNonce: nonce,
		Event:       "match_end",
	}
	return c.post(ctx, "/match/end", req, nil)
}

func (c *Client) WebSocketURL(roomToken string) (string, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported signaling URL scheme: %s", u.Scheme)
	}
	u.Path = "/signal"
	q := u.Query()
	q.Set("roomToken", roomToken)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) post(ctx context.Context, path string, body any, into any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", path, resp.Status)
	}
	if into != nil {
		return json.NewDecoder(resp.Body).Decode(into)
	}
	return nil
}
