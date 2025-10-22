package tonbalance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Service implements balance checks via TonAPI HTTP.
type Service struct {
	tonapiBase  string
	tonapiToken string
	httpClient  *http.Client
}

// NewService initializes TonAPI-based service.
func NewService(baseURL, apiToken string) *Service {
	if baseURL == "" {
		baseURL = "https://tonapi.io"
	}
	return &Service{tonapiBase: strings.TrimRight(baseURL, "/"), tonapiToken: apiToken, httpClient: &http.Client{Timeout: 8 * time.Second}}
}

// GetAddressBalanceNano returns native TON balance in nanoTONs for the address via TonAPI.
func (s *Service) GetAddressBalanceNano(ctx context.Context, address string) (int64, error) {
	var out struct {
		Balance string `json:"balance"`
	}
	url := s.tonapiBase + "/v2/accounts/" + address
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/json")
	if s.tonapiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.tonapiToken)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("tonapi http %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	var n int64
	for i := 0; i < len(out.Balance); i++ {
		c := out.Balance[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid balance format")
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// GetJettonBalanceNano returns jetton balance in smallest units for given owner wallet via TonAPI.
func (s *Service) GetJettonBalanceNano(ctx context.Context, walletAddress, jettonMaster string) (int64, error) {
	type jettonItem struct {
		Balance string `json:"balance"`
		Jetton  struct {
			Address string `json:"address"`
		} `json:"jetton"`
	}
	var out struct {
		Balances []jettonItem `json:"balances"`
	}
	url := s.tonapiBase + "/v2/accounts/" + walletAddress + "/jettons"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/json")
	if s.tonapiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.tonapiToken)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("tonapi http %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	jm := strings.ToLower(jettonMaster)
	for _, b := range out.Balances {
		if strings.ToLower(b.Jetton.Address) == jm {
			var n int64
			for i := 0; i < len(b.Balance); i++ {
				c := b.Balance[i]
				if c < '0' || c > '9' {
					return 0, fmt.Errorf("invalid jetton balance format")
				}
				n = n*10 + int64(c-'0')
			}
			return n, nil
		}
	}
	return 0, nil
}
