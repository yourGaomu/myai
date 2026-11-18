package shortlinks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	filesport "myai/core/port/files"
)

func (s *Service) Download(ctx context.Context, request filesport.DownloadRequest) (filesport.Download, error) {
	if !validID(request.AssetID) || request.MaxBytes < 0 || request.MaxBytes > filesport.MaxFileBytes {
		return filesport.Download{}, errors.New("download requires an asset ID and a byte limit up to 50 MiB")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	lease, err := s.acquire(ctx, "get_asset_download_url")
	if err != nil {
		return filesport.Download{}, err
	}
	defer lease.Close()
	var result struct {
		Asset       assetDTO  `json:"asset"`
		DownloadURL string    `json:"downloadUrl"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	if err := call(ctx, lease, "get_asset_download_url", map[string]any{"assetId": request.AssetID}, &result); err != nil {
		return filesport.Download{}, safeError("get asset download URL", err)
	}
	asset, err := result.Asset.asset(request.AssetID)
	if err != nil {
		return filesport.Download{}, err
	}
	if !result.ExpiresAt.After(time.Now()) {
		return filesport.Download{}, errors.New("short_links returned an expired download URL")
	}
	maxBytes := request.MaxBytes
	if maxBytes == 0 {
		maxBytes = filesport.MaxFileBytes
	}
	data, truncated, err := fetchObject(ctx, lease.Endpoint(), result.DownloadURL, asset, maxBytes)
	if err != nil {
		return filesport.Download{}, err
	}
	return filesport.Download{Asset: asset, Data: data, Truncated: truncated}, nil
}

// Public objects require HTTPS. Internal MinIO may use HTTP and a different
// port, but its hostname and resolved private address must match the configured
// MCP origin. The object client has no MCP headers, cookies, session or proxy.
type objectPolicy struct {
	originHost string
	originIPs  map[netip.Addr]bool
	mu         sync.Mutex
	validated  map[string][]netip.Addr
}

func newObjectPolicy(ctx context.Context, endpoint string) (*objectPolicy, error) {
	policy := &objectPolicy{originIPs: make(map[netip.Addr]bool), validated: make(map[string][]netip.Addr)}
	if endpoint == "" {
		return policy, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("invalid configured MCP origin")
	}
	policy.originHost = strings.ToLower(u.Hostname())
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil {
		return nil, safeError("resolve configured MCP object origin", err)
	}
	for _, address := range addresses {
		policy.originIPs[address.Unmap()] = true
	}
	return policy, nil
}

func privateObjectIP(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsPrivate() || address.IsLoopback() || netip.MustParsePrefix("100.64.0.0/10").Contains(address)
}

func forbiddenObjectIP(address netip.Addr) bool {
	address = address.Unmap()
	return !address.IsValid() || !address.IsGlobalUnicast() && !address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() ||
		netip.MustParsePrefix("0.0.0.0/8").Contains(address) || netip.MustParsePrefix("240.0.0.0/4").Contains(address)
}

func (p *objectPolicy) validateURL(u *url.URL) error {
	if u == nil || len(u.String()) > 16384 || u.Hostname() == "" || u.Opaque != "" || u.User != nil || u.Fragment != "" ||
		(u.Scheme != "https" && u.Scheme != "http") || strings.Contains(u.Hostname(), "%") {
		return errors.New("unsafe asset download URL")
	}
	if u.Scheme == "http" && !strings.EqualFold(u.Hostname(), p.originHost) {
		return errors.New("asset download requires HTTPS or the configured internal MCP host")
	}
	return nil
}

func (p *objectPolicy) addresses(ctx context.Context, host string, allowHTTP bool) ([]netip.Addr, error) {
	key := strings.ToLower(host)
	p.mu.Lock()
	cached := p.validated[key]
	p.mu.Unlock()
	addresses := cached
	if len(addresses) == 0 {
		var err error
		addresses, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("cannot resolve asset download address")
		}
	}
	for index, address := range addresses {
		address = address.Unmap()
		addresses[index] = address
		if forbiddenObjectIP(address) {
			return nil, errors.New("unsafe asset download address")
		}
		boundInternal := strings.EqualFold(host, p.originHost) && p.originIPs[address] && privateObjectIP(address)
		if privateObjectIP(address) && !boundInternal || allowHTTP && !boundInternal {
			return nil, errors.New("asset download address is outside the configured object origin")
		}
	}
	p.mu.Lock()
	if len(p.validated[key]) == 0 {
		p.validated[key] = addresses
	}
	p.mu.Unlock()
	return addresses, nil
}

func fetchObject(ctx context.Context, endpoint, rawURL string, asset filesport.Asset, maxBytes int64) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	policy, err := newObjectPolicy(ctx, endpoint)
	if err != nil {
		return nil, false, err
	}
	u, err := url.Parse(rawURL)
	if err != nil || policy.validateURL(u) != nil {
		return nil, false, errors.New("unsafe asset download URL")
	}
	// Validate each hop before dispatch and again at dial time. The dial uses
	// the validated IP directly, so a second DNS resolution cannot rebind it.
	transport := &http.Transport{
		Proxy: nil, DisableCompression: true, DisableKeepAlives: true,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.New("invalid object download address")
			}
			addresses, err := policy.addresses(ctx, host, false)
			if err != nil {
				return nil, err
			}
			var last error
			for _, ip := range addresses {
				connection, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return connection, nil
				}
				last = err
			}
			return nil, last
		},
	}
	defer transport.CloseIdleConnections()
	validate := func(ctx context.Context, u *url.URL) error {
		if err := policy.validateURL(u); err != nil {
			return err
		}
		_, err := policy.addresses(ctx, u.Hostname(), u.Scheme == "http")
		return err
	}
	if err := validate(ctx, u); err != nil {
		return nil, false, err
	}
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > 3 || via[len(via)-1].URL.Scheme == "https" && request.URL.Scheme != "https" {
				return errors.New("asset download redirect limit or scheme violation")
			}
			request.Header = make(http.Header)
			return validate(request.Context(), request.URL)
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false, errors.New("invalid asset download request")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, false, safeError("fetch asset object", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" {
		return nil, false, fmt.Errorf("fetch asset object returned an invalid response (status %d)", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != asset.Size {
		return nil, false, errors.New("asset object length does not match its metadata")
	}
	limit := min(maxBytes, asset.Size)
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, false, safeError("read asset object", err)
	}
	if asset.Size <= limit {
		if int64(len(data)) != asset.Size {
			return nil, false, errors.New("asset object size does not match its metadata")
		}
		if asset.Checksum != "" {
			checksum := sha256.Sum256(data)
			if hex.EncodeToString(checksum[:]) != strings.ToLower(asset.Checksum) {
				return nil, false, errors.New("asset object checksum does not match its metadata")
			}
		}
		return data, false, nil
	}
	if int64(len(data)) != limit+1 {
		return nil, false, errors.New("asset object ended before its declared size")
	}
	return data[:limit], true, nil
}
