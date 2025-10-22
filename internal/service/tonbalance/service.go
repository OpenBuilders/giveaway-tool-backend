package tonbalance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Service struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

func NewService(baseURL, apiToken string) *Service {
	if baseURL == "" {
		baseURL = "https://tonapi.io"
	}
	return &Service{baseURL: strings.TrimRight(baseURL, "/"), apiToken: apiToken, httpClient: &http.Client{Timeout: 8 * time.Second}}
}

func (s *Service) doJSON(ctx context.Context, method, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if s.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiToken)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tonapi http %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// GetAddressBalanceNano returns native TON balance in nanoTONs for the address.
func (s *Service) GetAddressBalanceNano(ctx context.Context, address string) (int64, error) {
	var out struct {
		Balance string `json:"balance"`
	}
	url := fmt.Sprintf("%s/v2/accounts/%s", s.baseURL, address)
	if err := s.doJSON(ctx, http.MethodGet, url, &out); err != nil {
		return 0, err
	}
	// balance may come as string integer
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

// GetJettonBalanceNano returns jetton balance in smallest units for a given wallet and jetton master address.
func (s *Service) GetJettonBalanceNano(ctx context.Context, walletAddress, jettonMaster string) (int64, error) {
	var out struct {
		Balances []struct {
			Balance string `json:"balance"`
			Jetton  struct {
				Address string `json:"address"`
			} `json:"jetton"`
		} `json:"balances"`
	}
	url := fmt.Sprintf("%s/v2/accounts/%s/jettons", s.baseURL, walletAddress)
	if err := s.doJSON(ctx, http.MethodGet, url, &out); err != nil {
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


