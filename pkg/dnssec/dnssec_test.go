// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dnssec

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// TestKeyGeneration tests DNSSEC key generation.
func TestKeyGeneration(t *testing.T) {
	km := NewKeyManager("test-node")

	tests := []struct {
		name      string
		zone      string
		algorithm Algorithm
		wantErr   bool
	}{
		{
			name:      "generate ECDSAP256SHA256 key",
			zone:      "example.com.",
			algorithm: AlgorithmECDSAP256SHA256,
			wantErr:   false,
		},
		{
			name:      "generate ECDSAP384SHA384 key",
			zone:      "example.org.",
			algorithm: AlgorithmECDSAP384SHA384,
			wantErr:   false,
		},
		{
			name:      "generate default algorithm key",
			zone:      "example.net.",
			algorithm: "",
			wantErr:   false,
		},
		{
			name:      "unsupported algorithm",
			zone:      "example.io.",
			algorithm: "RSA256",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := km.GenerateKey(tt.zone, tt.algorithm)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify key properties
			if key.Zone != dns.Fqdn(tt.zone) {
				t.Errorf("expected zone %s, got %s", dns.Fqdn(tt.zone), key.Zone)
			}
			if key.KeyTag == 0 {
				t.Error("expected non-zero key tag")
			}
			if key.PublicKey == "" {
				t.Error("expected non-empty public key")
			}
			if key.PrivateKey == "" {
				t.Error("expected non-empty private key")
			}
			if !key.CanSign() {
				t.Error("expected key to be able to sign")
			}
			if key.NodeID != "test-node" {
				t.Errorf("expected node ID 'test-node', got %s", key.NodeID)
			}
		})
	}
}

// TestKeyManager tests key manager operations.
func TestKeyManager(t *testing.T) {
	km := NewKeyManager("test-node")

	// Generate a key
	key, err := km.GenerateKey("example.com.", AlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Get the key
	retrieved := km.GetKey("example.com.")
	if retrieved == nil {
		t.Fatal("expected to retrieve key")
	}
	if retrieved.KeyTag != key.KeyTag {
		t.Errorf("expected key tag %d, got %d", key.KeyTag, retrieved.KeyTag)
	}

	// Get all keys
	allKeys := km.GetAllKeys()
	if len(allKeys) != 1 {
		t.Errorf("expected 1 key, got %d", len(allKeys))
	}

	// Get zones
	zones := km.GetZones()
	if len(zones) != 1 {
		t.Errorf("expected 1 zone, got %d", len(zones))
	}

	// Remove the key
	km.RemoveKey("example.com.")
	if km.GetKey("example.com.") != nil {
		t.Error("expected key to be removed")
	}
}

// TestDSRecord tests DS record generation.
func TestDSRecord(t *testing.T) {
	km := NewKeyManager("test-node")

	key, err := km.GenerateKey("example.com.", AlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	ds := key.DSRecord()
	if ds == nil {
		t.Fatal("expected DS record")
	}
	if ds.KeyTag != key.KeyTag {
		t.Errorf("expected DS key tag %d, got %d", key.KeyTag, ds.KeyTag)
	}
	if ds.DigestType != dns.SHA256 {
		t.Errorf("expected SHA256 digest type")
	}
	if ds.Digest == "" {
		t.Error("expected non-empty digest")
	}

	dsString := key.DSRecordString()
	if dsString == "" {
		t.Error("expected non-empty DS record string")
	}
}

// TestKeyIsNewerThan tests key age comparison.
func TestKeyIsNewerThan(t *testing.T) {
	now := time.Now()
	older := &KeyPair{CreatedAt: now.Add(-time.Hour)}
	newer := &KeyPair{CreatedAt: now}

	if !newer.IsNewerThan(older) {
		t.Error("expected newer key to be newer than older key")
	}
	if older.IsNewerThan(newer) {
		t.Error("expected older key not to be newer than newer key")
	}
	if !newer.IsNewerThan(nil) {
		t.Error("expected any key to be newer than nil")
	}
}

// TestKeyPairMarshalJSON tests JSON marshaling/unmarshaling of keys.
func TestKeyPairMarshalJSON(t *testing.T) {
	km := NewKeyManager("test-node")

	original, err := km.GenerateKey("example.com.", AlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}

	// Unmarshal
	var restored KeyPair
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("failed to unmarshal key: %v", err)
	}

	// Compare
	if restored.Zone != original.Zone {
		t.Errorf("zone mismatch: %s != %s", restored.Zone, original.Zone)
	}
	if restored.KeyTag != original.KeyTag {
		t.Errorf("key tag mismatch: %d != %d", restored.KeyTag, original.KeyTag)
	}
	if restored.Algorithm != original.Algorithm {
		t.Errorf("algorithm mismatch: %s != %s", restored.Algorithm, original.Algorithm)
	}
}

// TestNSEC3HashGeneration tests NSEC3 hash computation.
func TestNSEC3HashGeneration(t *testing.T) {
	manager, err := NewNSEC3Manager(DefaultNSEC3Config())
	if err != nil {
		t.Fatalf("failed to create NSEC3 manager: %v", err)
	}

	// Set a known salt
	manager.SetSalt("example.com.", "AABBCCDD")

	hash := manager.HashName("www.example.com.", "example.com.")
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	if len(hash) != 32 { // SHA-1 produces 20 bytes, base32hex encodes to 32 chars
		t.Errorf("expected hash length 32, got %d", len(hash))
	}

	// Same input should produce same hash
	hash2 := manager.HashName("www.example.com.", "example.com.")
	if hash != hash2 {
		t.Error("same input should produce same hash")
	}

	// Different input should produce different hash
	hash3 := manager.HashName("mail.example.com.", "example.com.")
	if hash == hash3 {
		t.Error("different input should produce different hash")
	}
}

// TestNSEC3NXDOMAINGeneration tests NSEC3 NXDOMAIN proof generation.
func TestNSEC3NXDOMAINGeneration(t *testing.T) {
	manager, err := NewNSEC3Manager(DefaultNSEC3Config())
	if err != nil {
		t.Fatalf("failed to create NSEC3 manager: %v", err)
	}

	records := manager.GenerateNXDOMAIN("example.com.", "nonexistent.example.com.")
	if len(records) < 2 {
		t.Errorf("expected at least 2 NSEC3 records, got %d", len(records))
	}

	for _, rr := range records {
		nsec3, ok := rr.(*dns.NSEC3)
		if !ok {
			t.Errorf("expected *dns.NSEC3, got %T", rr)
			continue
		}
		if nsec3.Iterations != 10 { // Default iterations
			t.Errorf("expected 10 iterations, got %d", nsec3.Iterations)
		}
	}
}

// TestNSEC3PARAM tests NSEC3PARAM record generation.
func TestNSEC3PARAM(t *testing.T) {
	manager, err := NewNSEC3Manager(DefaultNSEC3Config())
	if err != nil {
		t.Fatalf("failed to create NSEC3 manager: %v", err)
	}

	param := manager.NSEC3PARAM("example.com.")
	if param == nil {
		t.Fatal("expected NSEC3PARAM record")
	}
	if param.Hash != 1 { // SHA-1
		t.Errorf("expected hash algorithm 1, got %d", param.Hash)
	}
	if param.Iterations != 10 {
		t.Errorf("expected 10 iterations, got %d", param.Iterations)
	}
}

// TestSignerCreation tests signer creation and configuration.
func TestSignerCreation(t *testing.T) {
	km := NewKeyManager("test-node")
	nsec3, _ := NewNSEC3Manager(DefaultNSEC3Config())

	signer := NewSigner(SignerConfig{
		KeyManager:   km,
		NSEC3Manager: nsec3,
	})

	if signer == nil {
		t.Fatal("expected signer to be created")
	}
	if signer.config.SignatureTTL != 86400 {
		t.Errorf("expected default TTL 86400, got %d", signer.config.SignatureTTL)
	}
}

// TestSignerWithNoKey tests signing when no key is available.
func TestSignerWithNoKey(t *testing.T) {
	km := NewKeyManager("test-node")
	signer := NewSigner(SignerConfig{
		KeyManager: km,
	})

	// Create a DNS message
	msg := new(dns.Msg)
	msg.SetQuestion("www.example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "www.example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
		A: []byte{1, 2, 3, 4},
	})

	// Sign without a key
	signed, err := signer.SignResponse(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return original message unchanged
	signedMsg := signed.(*dns.Msg)
	if len(signedMsg.Answer) != 1 {
		t.Errorf("expected 1 answer, got %d", len(signedMsg.Answer))
	}
}

// TestSignerWithKey tests signing with a valid key.
func TestSignerWithKey(t *testing.T) {
	km := NewKeyManager("test-node")
	_, err := km.GenerateKey("example.com.", AlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	signer := NewSigner(SignerConfig{
		KeyManager: km,
	})

	// Create a DNS message
	msg := new(dns.Msg)
	msg.SetQuestion("www.example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "www.example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
		A: []byte{1, 2, 3, 4},
	})

	// Sign with a key
	signed, err := signer.SignResponse(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	signedMsg := signed.(*dns.Msg)
	// Should have the original A record plus an RRSIG
	if len(signedMsg.Answer) < 2 {
		t.Errorf("expected at least 2 answers (A + RRSIG), got %d", len(signedMsg.Answer))
	}

	// Check for RRSIG record
	hasRRSIG := false
	for _, rr := range signedMsg.Answer {
		if rr.Header().Rrtype == dns.TypeRRSIG {
			hasRRSIG = true
			break
		}
	}
	if !hasRRSIG {
		t.Error("expected RRSIG record in answer")
	}

	// Check AD flag is set
	if !signedMsg.AuthenticatedData {
		t.Error("expected AuthenticatedData flag to be set")
	}
}

// TestKeySyncerCreation tests key syncer creation.
func TestKeySyncerCreation(t *testing.T) {
	km := NewKeyManager("test-node")
	syncer := NewKeySyncer(KeySyncConfig{
		Peers:        []string{"http://peer1:9090", "http://peer2:9090"},
		PollInterval: time.Hour,
		Timeout:      30 * time.Second,
		KeyManager:   km,
	})

	if syncer == nil {
		t.Fatal("expected syncer to be created")
	}
	if len(syncer.config.Peers) != 2 {
		t.Errorf("expected 2 peers, got %d", len(syncer.config.Peers))
	}
}

// TestKeySyncerStatus tests sync status reporting.
func TestKeySyncerStatus(t *testing.T) {
	km := NewKeyManager("test-node")
	syncer := NewKeySyncer(KeySyncConfig{
		Peers:        []string{"http://peer1:9090"},
		PollInterval: time.Hour,
		KeyManager:   km,
	})

	status := syncer.GetStatus()
	if status.Running {
		t.Error("expected syncer not to be running")
	}
	if status.PollInterval != "1h0m0s" {
		t.Errorf("expected poll interval '1h0m0s', got %s", status.PollInterval)
	}
}

// TestKeySyncerSync tests key sync from peer.
func TestKeySyncerSync(t *testing.T) {
	// Create a mock peer server
	remoteKM := NewKeyManager("remote-node")
	remoteKey, _ := remoteKM.GenerateKey("example.com.", AlgorithmECDSAP256SHA256)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/dnssec/keys" {
			http.NotFound(w, r)
			return
		}

		resp := KeySyncResponse{
			Keys: []*KeyPair{remoteKey},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create local syncer
	localKM := NewKeyManager("local-node")
	syncer := NewKeySyncer(KeySyncConfig{
		Peers:      []string{server.URL},
		Timeout:    5 * time.Second,
		KeyManager: localKM,
	})

	// Sync
	ctx := context.Background()
	syncer.SyncNow(ctx)

	// Verify key was imported
	importedKey := localKM.GetKey("example.com.")
	if importedKey == nil {
		t.Fatal("expected key to be imported")
	}
	if importedKey.KeyTag != remoteKey.KeyTag {
		t.Errorf("expected key tag %d, got %d", remoteKey.KeyTag, importedKey.KeyTag)
	}
	// Private key should be stripped
	if importedKey.PrivateKey != "" {
		t.Error("expected private key to be stripped from synced key")
	}
}

// TestKeySyncerNewestWins tests that newest key wins during sync.
func TestKeySyncerNewestWins(t *testing.T) {
	// Create local key (older)
	localKM := NewKeyManager("local-node")
	localKey, _ := localKM.GenerateKey("example.com.", AlgorithmECDSAP256SHA256)
	localKey.CreatedAt = time.Now().Add(-24 * time.Hour) // Make it older

	// Create remote key (newer)
	remoteKM := NewKeyManager("remote-node")
	remoteKey, _ := remoteKM.GenerateKey("example.com.", AlgorithmECDSAP256SHA256)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := KeySyncResponse{
			Keys: []*KeyPair{remoteKey},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	syncer := NewKeySyncer(KeySyncConfig{
		Peers:      []string{server.URL},
		Timeout:    5 * time.Second,
		KeyManager: localKM,
	})

	ctx := context.Background()
	syncer.SyncNow(ctx)

	// Verify newer key replaced older
	currentKey := localKM.GetKey("example.com.")
	if currentKey.KeyTag == localKey.KeyTag {
		t.Error("expected newer key to replace older key")
	}
	if currentKey.KeyTag != remoteKey.KeyTag {
		t.Errorf("expected key tag %d, got %d", remoteKey.KeyTag, currentKey.KeyTag)
	}
}

// TestDefaultNSEC3Config tests default NSEC3 configuration.
func TestDefaultNSEC3Config(t *testing.T) {
	config := DefaultNSEC3Config()
	if config.Iterations != 10 {
		t.Errorf("expected 10 iterations, got %d", config.Iterations)
	}
	if config.SaltLength != 8 {
		t.Errorf("expected 8 salt length, got %d", config.SaltLength)
	}
	if config.OptOut {
		t.Error("expected opt-out to be false by default")
	}
}

// TestHashIncrementDecrement tests hash increment/decrement functions.
func TestHashIncrementDecrement(t *testing.T) {
	hash := "ABCDEF0123456789ABCDEF0123456789"

	incremented := incrementHash(hash)
	if incremented == hash {
		t.Error("expected increment to change hash")
	}

	decremented := decrementHash(hash)
	if decremented == hash {
		t.Error("expected decrement to change hash")
	}
}
