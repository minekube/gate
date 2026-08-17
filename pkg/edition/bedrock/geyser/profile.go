package geyser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	// GeyserMC API for linked accounts and skins
	GEYSER_API_URL = "https://api.geysermc.org/v2/"
)

// SkinResult represents skin data from GeyserMC API.
type SkinResult struct {
	Hash      string `json:"hash"`
	Steve     bool   `json:"is_steve"`
	Signature string `json:"signature"`
	TextureID string `json:"texture_id"`
	Value     string `json:"value"`
}

// ProfileManager handles Bedrock player profiles, linked accounts, and skins.
type ProfileManager struct {
	client *http.Client
}

// NewProfileManager creates a new profile manager.
func NewProfileManager() *ProfileManager {
	return &ProfileManager{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// LinkedAccountResult represents a linked Java account from the GeyserMC
// global link API (https://api.geysermc.org/v2/link/bedrock/<xuid>).
type LinkedAccountResult struct {
	BedrockID      int64     `json:"bedrock_id"`
	JavaID         uuid.UUID `json:"java_id"`
	JavaName       string    `json:"java_name"`
	LastNameUpdate int64     `json:"last_name_update"`
}

// GetLinkedAccount retrieves the linked Java account for a Bedrock XUID from
// the GeyserMC global link API — the official Floodgate linking service
// (GlobalPlayerLinking, enable-global-linking defaults to true) that backend
// Floodgate plugins use in production. It is HTTPS and operated by GeyserMC,
// the same trust basis as the skin API and the Floodgate plugin ecosystem
// itself. It is NOT a per-connection signature: Gate only consults it for
// identity promotion when the operator explicitly enabled the
// backendFloodgate trust boundary, and failures are fail-closed (the XUID
// identity stays). A verified Bedrock principal (Connect path) remains the
// only signed source.
func (pm *ProfileManager) GetLinkedAccount(xuid int64) (*LinkedAccountResult, error) {
	var result LinkedAccountResult
	err := pm.geyserApiGet("link/bedrock/"+strconv.FormatInt(xuid, 10), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get linked account: %w", err)
	}
	return &result, nil
}

// GetSkin retrieves skin data for a Bedrock XUID.
func (pm *ProfileManager) GetSkin(xuid int64) (*SkinResult, error) {
	var result SkinResult
	err := pm.geyserApiGet("skin/"+strconv.FormatInt(xuid, 10), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get skin: %w", err)
	}
	return &result, nil
}

// geyserApiGet performs a GET request to the GeyserMC API.
func (pm *ProfileManager) geyserApiGet(endpoint string, result any) error {
	req, err := http.NewRequest("GET", GEYSER_API_URL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Gate-Proxy/1.0")

	res, err := pm.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("geyser api returned status code %d", res.StatusCode)
	}

	if err := json.NewDecoder(res.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}
