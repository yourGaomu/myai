package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// MobileMCPStore is operator-owned. A workspace cannot opt its own endpoint
// into remote review. Each grant names one exact, non-secret endpoint.
type MobileMCPStore struct{ file *Store }

type MobileMCPScope struct {
	Workspace string `json:"workspace"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
}

type MobileMCPGrant struct {
	Scope      MobileMCPScope `json:"scope"`
	PairingID  string         `json:"pairing_id"`
	Endpoint   string         `json:"endpoint"`
	Generation string         `json:"generation"`
}

type mobileMCPRecords struct {
	Version int              `json:"version"`
	Grants  []MobileMCPGrant `json:"grants"`
}

func NewMobileMCPStore(path string) *MobileMCPStore { return &MobileMCPStore{file: NewStore(path)} }

func DefaultMobileMCPStore() (*MobileMCPStore, error) {
	store, err := DefaultStore()
	if err != nil {
		return nil, err
	}
	return NewMobileMCPStore(filepath.Join(filepath.Dir(store.Path()), "mcp-mobile-admin.json")), nil
}

func ValidMobilePairingID(id string) bool {
	value, err := hex.DecodeString(id)
	return err == nil && len(value) == sha256.Size && strings.ToLower(id) == id
}

// SafeMobileMCPEndpoint is deliberately narrower than the HTTP MCP transport.
// The operator must additionally confirm the entire URL is safe to display.
func SafeMobileMCPEndpoint(endpoint string) bool {
	if len(endpoint) > 512 || strings.ContainsAny(endpoint, "?#%\\") {
		return false
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.String() != endpoint || parsed.User != nil || parsed.Opaque != "" || parsed.Hostname() == "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") {
		return false
	}
	return !strings.ContainsFunc(endpoint, func(r rune) bool { return r < 33 || r > 126 })
}

func (s *MobileMCPStore) withRecords(scope MobileMCPScope, fn func(MobileMCPScope, *mobileMCPRecords) error) error {
	validID := func(id string) bool {
		return id != "" && len(id) <= 256 && strings.TrimSpace(id) == id && !strings.ContainsFunc(id, unicode.IsControl)
	}
	if !validID(scope.UserID) || !validID(scope.DeviceID) {
		return errors.New("exact Agent user and device are required")
	}
	request, err := NewRequest(KindMCP, scope.Workspace, ".", struct{}{})
	if err != nil {
		return err
	}
	scope.Workspace = request.Workspace
	unlock, err := s.file.lock(request)
	if err != nil {
		return err
	}
	defer unlock()
	data := mobileMCPRecords{Version: 1}
	if err := s.file.readJSON(&data); err != nil {
		return err
	}
	if data.Version != 1 {
		return errors.New("invalid mobile MCP grant store")
	}
	return fn(scope, &data)
}

// WithGrants holds the local grant lock through the caller's approval write,
// ordering local CLI revoke/regrant against in-flight approvals.
func (s *MobileMCPStore) WithGrants(scope MobileMCPScope, pairingID string, fn func([]MobileMCPGrant) error) error {
	if pairingID != "" && !ValidMobilePairingID(pairingID) {
		return errors.New("invalid pairing ID")
	}
	return s.withRecords(scope, func(scope MobileMCPScope, data *mobileMCPRecords) error {
		var grants []MobileMCPGrant
		for _, grant := range data.Grants {
			if grant.Scope == scope && (pairingID == "" || grant.PairingID == pairingID) && SafeMobileMCPEndpoint(grant.Endpoint) && grant.Generation != "" {
				grants = append(grants, grant)
			}
		}
		return fn(grants)
	})
}

func (s *MobileMCPStore) Grant(scope MobileMCPScope, pairingID, endpoint string) error {
	if !ValidMobilePairingID(pairingID) || !SafeMobileMCPEndpoint(endpoint) {
		return errors.New("provide a complete pairing ID and an exact non-secret HTTP(S) endpoint")
	}
	return s.withRecords(scope, func(scope MobileMCPScope, data *mobileMCPRecords) error {
		count := 0
		for _, grant := range data.Grants {
			if grant.Scope == scope && grant.PairingID == pairingID {
				count++
				if grant.Endpoint == endpoint {
					return nil
				}
			}
		}
		if count >= 32 {
			return errors.New("too many endpoints for this pairing")
		}
		data.Grants = append(data.Grants, MobileMCPGrant{Scope: scope, PairingID: pairingID, Endpoint: endpoint, Generation: uuid.NewString()})
		return s.file.write(data)
	})
}

func (s *MobileMCPStore) Revoke(scope MobileMCPScope, pairingID string) error {
	if !ValidMobilePairingID(pairingID) {
		return errors.New("provide a complete pairing ID")
	}
	return s.withRecords(scope, func(scope MobileMCPScope, data *mobileMCPRecords) error {
		n := len(data.Grants)
		data.Grants = deleteMobileMCPGrants(data.Grants, scope, pairingID)
		if len(data.Grants) == n {
			return nil
		}
		return s.file.write(data)
	})
}

func deleteMobileMCPGrants(grants []MobileMCPGrant, scope MobileMCPScope, pairingID string) []MobileMCPGrant {
	next := grants[:0]
	for _, grant := range grants {
		if grant.Scope != scope || grant.PairingID != pairingID {
			next = append(next, grant)
		}
	}
	return next
}
