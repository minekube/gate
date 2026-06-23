package qsnm

import (
	"slices"
	"testing"
	"time"
)

func TestMonthlyNetworkKeyRotatesByUTCMonth(t *testing.T) {
	secret := []byte("metric-secret")
	fingerprint := "gate-installation:abc123"

	june, err := MonthlyNetworkKey(secret, fingerprint, time.Date(2026, 6, 30, 23, 59, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	july, err := MonthlyNetworkKey(secret, fingerprint, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	again, err := MonthlyNetworkKey(secret, fingerprint, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if june == july {
		t.Fatal("network key did not rotate by month")
	}
	if june != again {
		t.Fatal("network key was not stable within the same month")
	}
}

func TestGateNetworkSuccessPropertiesAreAllowlisted(t *testing.T) {
	props, err := (GateNetworkSuccess{
		OccurredAt:         time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC),
		NetworkKey:         "qsnm_2026-06_hash",
		EvidenceID:         "server-post-connect:abc123",
		GateVersion:        "1.0.0",
		IntegrationFamily:  "gate",
		IntegrationVersion: "1.0.0",
		PlayerCountBucket:  "1-5",
		ServerCountBucket:  "1",
	}).Properties()
	if err != nil {
		t.Fatal(err)
	}

	for key := range props {
		if !slices.Contains(GateNetworkSuccessAllowlist, key) {
			t.Fatalf("property %q is not allowlisted", key)
		}
	}
	for _, forbidden := range []string{"ip", "host", "domain", "email", "config"} {
		if _, ok := props[forbidden]; ok {
			t.Fatalf("forbidden raw property %q was emitted", forbidden)
		}
	}
	if props[PropQSNMQualifyingEvent] != true {
		t.Fatal("event was not marked as QSNM qualifying")
	}
}
