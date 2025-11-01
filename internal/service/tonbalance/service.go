package tonbalance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	rplatform "github.com/open-builders/giveaway-backend/internal/platform/redis"
	tongo "github.com/tonkeeper/tongo/ton"
)

// stringOrNumber is a helper type that unmarshals a JSON value that can be either
// a number (unquoted) or a string containing digits. We normalize it to a string.
type stringOrNumber string

func (sn *stringOrNumber) UnmarshalJSON(b []byte) error {
	// Quoted string case
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*sn = stringOrNumber(s)
		return nil
	}
	// Number token case
	var num json.Number
	if err := json.Unmarshal(b, &num); err != nil {
		return err
	}
	*sn = stringOrNumber(num.String())
	return nil
}

// Service implements balance checks via TonAPI HTTP.
type Service struct {
	tonapiBase  string
	tonapiToken string
	httpClient  *http.Client
	// Optional Redis cache for jetton metadata
	cache    *rplatform.Client
	cacheTTL time.Duration
}

// JettonMeta contains commonly used jetton metadata fields.
type JettonMeta struct {
	Decimals int
	Symbol   string
	Image    string
}

// NewService initializes TonAPI-based service.
func NewService(baseURL, apiToken string) *Service {
	if baseURL == "" {
		baseURL = "https://tonapi.io"
	}
	return &Service{tonapiBase: strings.TrimRight(baseURL, "/"), tonapiToken: apiToken, httpClient: &http.Client{Timeout: 8 * time.Second}}
}

// WithCache enables Redis-based caching for metadata lookups.
func (s *Service) WithCache(cache *rplatform.Client, ttl time.Duration) *Service {
	s.cache = cache
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	s.cacheTTL = ttl
	return s
}

// GetAddressBalanceNano returns native TON balance in nanoTONs for the address via TonAPI.
func (s *Service) GetAddressBalanceNano(ctx context.Context, address string) (int64, error) {
	var out struct {
		Balance stringOrNumber `json:"balance"`
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
	bs := string(out.Balance)
	for i := 0; i < len(bs); i++ {
		c := bs[i]
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
		Balance stringOrNumber `json:"balance"`
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

	// Normalize provided jetton master to raw form (workchain:hex) for reliable comparison
	// Accept both friendly/base64 and raw formats. If parsing fails, fallback to lowercased input.
	var jm string
	if addr, err := tongo.ParseAccountID(jettonMaster); err == nil {
		jm = strings.ToLower(addr.ToRaw())
	} else {
		jm = strings.ToLower(jettonMaster)
	}

	for _, b := range out.Balances {
		// TonAPI is expected to return raw address for jetton master; normalize just in case
		bj := strings.ToLower(b.Jetton.Address)
		if parsed, err := tongo.ParseAccountID(bj); err == nil {
			bj = strings.ToLower(parsed.ToRaw())
		}
		if bj == jm {
			var n int64
			bs := string(b.Balance)
			for i := 0; i < len(bs); i++ {
				c := bs[i]
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

// GetJettonDecimals returns decimals for a jetton master, using cache when available.
func (s *Service) GetJettonDecimals(ctx context.Context, jettonMaster string) (int, error) {
	meta, err := s.GetJettonMeta(ctx, jettonMaster)
	if err != nil {
		return 0, err
	}
	return meta.Decimals, nil
}

// GetJettonMeta returns jetton metadata (decimals, symbol, image), using cache when available.
func (s *Service) GetJettonMeta(ctx context.Context, jettonMaster string) (*JettonMeta, error) {
	// Normalize to raw form for consistent keys
	var jm string
	if addr, err := tongo.ParseAccountID(jettonMaster); err == nil {
		jm = strings.ToLower(addr.ToRaw())
	} else {
		jm = strings.ToLower(jettonMaster)
	}

	// Try cache first
	var cached JettonMeta
	hit := false
	if s.cache != nil {
		if n, err := s.cache.Get(ctx, "jetton:meta:"+jm+":decimals").Int(); err == nil && n >= 0 {
			cached.Decimals = n
			hit = true
		}
		if sym, err := s.cache.Get(ctx, "jetton:meta:"+jm+":symbol").Result(); err == nil && sym != "" {
			cached.Symbol = sym
			hit = true
		}
		if img, err := s.cache.Get(ctx, "jetton:meta:"+jm+":image").Result(); err == nil && img != "" {
			cached.Image = img
			hit = true
		}
		if hit && cached.Decimals > 0 {
			// Return cached when we have at least decimals.
			return &cached, nil
		}
	}

	// Fetch from TonAPI
	url := s.tonapiBase + "/v2/jettons/" + jm
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/json")
	if s.tonapiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.tonapiToken)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tonapi http %d", resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	var meta JettonMeta
	// Prefer nested metadata
	if md, ok := out["metadata"].(map[string]any); ok {
		if v, ok := md["decimals"]; ok {
			switch t := v.(type) {
			case float64:
				meta.Decimals = int(t)
			case string:
				if n, e := strconv.Atoi(t); e == nil {
					meta.Decimals = n
				}
			}
		}
		if v, ok := md["symbol"].(string); ok {
			meta.Symbol = v
		}
		if v, ok := md["image"].(string); ok {
			meta.Image = v
		}
	}
	// Fallbacks (top-level)
	if meta.Decimals == 0 {
		if v, ok := out["decimals"]; ok {
			switch t := v.(type) {
			case float64:
				meta.Decimals = int(t)
			case string:
				if n, e := strconv.Atoi(t); e == nil {
					meta.Decimals = n
				}
			}
		}
	}
	if meta.Symbol == "" {
		if v, ok := out["symbol"].(string); ok {
			meta.Symbol = v
		}
	}
	if meta.Image == "" {
		if v, ok := out["image"].(string); ok {
			meta.Image = v
		}
	}

	if meta.Decimals < 0 {
		meta.Decimals = 0
	}

	// Cache results
	if s.cache != nil {
		_ = s.cache.Set(ctx, "jetton:meta:"+jm+":decimals", meta.Decimals, s.cacheTTL).Err()
		if meta.Symbol != "" {
			_ = s.cache.Set(ctx, "jetton:meta:"+jm+":symbol", meta.Symbol, s.cacheTTL).Err()
		}
		if meta.Image != "" {
			_ = s.cache.Set(ctx, "jetton:meta:"+jm+":image", meta.Image, s.cacheTTL).Err()
		}
	}
	return &meta, nil
}
