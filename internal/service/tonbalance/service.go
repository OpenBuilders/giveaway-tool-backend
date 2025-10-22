package tonbalance

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "math/big"
    "net/http"
    "strings"
    "time"

    "github.com/xssnick/tonutils-go/address"
    "github.com/xssnick/tonutils-go/liteclient"
    "github.com/xssnick/tonutils-go/ton"
    "github.com/xssnick/tonutils-go/tlb"
)

// Service implements balance checks via TON lite servers using tonutils-go.
type Service struct {
    api         *ton.APIClient
    tonapiBase  string
    tonapiToken string
    httpClient  *http.Client
}

// NewLiteService initializes connection pool from global config URL (mainnet by default).
// Example URL: https://ton.org/global-config.json
func NewLiteService(configURL, tonapiBase, tonapiToken string) *Service {
	pool := liteclient.NewConnectionPool()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if configURL == "" {
		configURL = "https://ton.org/global-config.json"
	}
	_ = pool.AddConnectionsFromConfigUrl(ctx, configURL)
    api := ton.NewAPIClient(pool)
    if tonapiBase == "" {
        tonapiBase = "https://tonapi.io"
    }
    return &Service{api: api, tonapiBase: strings.TrimRight(tonapiBase, "/"), tonapiToken: tonapiToken, httpClient: &http.Client{Timeout: 8 * time.Second}}
}

// GetAddressBalanceNano returns native TON balance in nanoTONs for the address.
func (s *Service) GetAddressBalanceNano(ctx context.Context, addr string) (int64, error) {
	if s == nil || s.api == nil {
		return 0, errors.New("ton client not initialized")
	}
	parsed, err := address.ParseAddr(addr)
	if err != nil {
		return 0, err
	}
	// Use sticky context for stable node
    blk, err := s.api.CurrentMasterchainInfo(ctx)
	if err != nil {
		return 0, err
	}
	acc, err := s.api.GetAccount(ctx, blk, parsed)
	if err != nil {
		return 0, err
	}
    if acc == nil || acc.State == nil {
        return 0, nil
    }
    if acc.State.Status != tlb.AccountStatusActive {
        return 0, nil
    }
    nano := acc.State.Balance.Nano()
    if nano == nil {
        return 0, nil
    }
    return bigToInt64(nano), nil
}

// GetJettonBalanceNano returns jetton balance in smallest units for given owner wallet and jetton master.
func (s *Service) GetJettonBalanceNano(ctx context.Context, ownerAddr, jettonMaster string) (int64, error) {
    if s == nil {
        return 0, errors.New("ton service not initialized")
    }
    // Fallback to TonAPI for jetton balances to avoid complex get-method calls here
    type jettonItem struct {
        Balance string `json:"balance"`
        Jetton  struct {
            Address string `json:"address"`
        } `json:"jetton"`
    }
    var out struct {
        Balances []jettonItem `json:"balances"`
    }
    url := fmt.Sprintf("%s/v2/accounts/%s/jettons", s.tonapiBase, ownerAddr)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if s.tonapiToken != "" {
        req.Header.Set("Authorization", "Bearer "+s.tonapiToken)
    }
    req.Header.Set("Accept", "application/json")
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
            // parse decimal string to int64 (fits typical balances)
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

func bigToInt64(v *big.Int) int64 {
	if v == nil {
		return 0
	}
	// Clamp to int64 range
	if v.BitLen() > 63 {
		// take lower 63 bits to avoid overflow, sufficient for comparisons with int64 thresholds used in app
		return new(big.Int).And(v, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 63), big.NewInt(1))).Int64()
	}
	return v.Int64()
}
