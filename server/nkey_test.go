// Copyright 2018 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"bufio"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

// Nonce has to be a string since we used different encoding by default than json.Unmarshal.
type nonceInfo struct {
	Id    string `json:"server_id"`
	CID   uint64 `json:"client_id,omitempty"`
	Nonce string `json:"nonce,omitempty"`
}

// This is a seed for a user. We can extract public and private keys from this for testing.
const seed = "SUAKYRHVIOREXV7EUZTBHUHL7NUMHPMAS7QMDU3GTIUWEI5LDNOXD43IZY"

func nkeyBasicSetup() (*Server, *client, *bufio.Reader, string) {
	kp, _ := nkeys.FromSeed(seed)
	pub, _ := kp.PublicKey()
	opts := defaultServerOptions
	opts.Nkeys = []*NkeyUser{&NkeyUser{Nkey: pub}}
	return rawSetup(opts)
}

func mixedSetup() (*Server, *client, *bufio.Reader, string) {
	kp, _ := nkeys.FromSeed(seed)
	pub, _ := kp.PublicKey()
	opts := defaultServerOptions
	opts.Nkeys = []*NkeyUser{&NkeyUser{Nkey: pub}}
	opts.Users = []*User{&User{Username: "derek", Password: "foo"}}
	return rawSetup(opts)
}

func newClientForServer(s *Server) (*client, *bufio.Reader, string) {
	cli, srv := net.Pipe()
	cr := bufio.NewReaderSize(cli, maxBufSize)
	ch := make(chan *client)
	createClientAsync(ch, s, srv)
	l, _ := cr.ReadString('\n')
	// Grab client
	c := <-ch
	return c, cr, l
}

func TestServerInfoNonce(t *testing.T) {
	_, l := setUpClientWithResponse()
	if !strings.HasPrefix(l, "INFO ") {
		t.Fatalf("INFO response incorrect: %s\n", l)
	}
	// Make sure payload is proper json
	var info nonceInfo
	err := json.Unmarshal([]byte(l[5:]), &info)
	if err != nil {
		t.Fatalf("Could not parse INFO json: %v\n", err)
	}
	if info.Nonce != "" {
		t.Fatalf("Expected an empty nonce with no nkeys defined")
	}

	// Now setup server with auth and nkeys to trigger nonce generation
	s, _, _, l := nkeyBasicSetup()

	if !strings.HasPrefix(l, "INFO ") {
		t.Fatalf("INFO response incorrect: %s\n", l)
	}
	// Make sure payload is proper json
	err = json.Unmarshal([]byte(l[5:]), &info)
	if err != nil {
		t.Fatalf("Could not parse INFO json: %v\n", err)
	}
	if info.Nonce == "" {
		t.Fatalf("Expected a non-empty nonce with nkeys defined")
	}

	// Make sure new clients get new nonces
	oldNonce := info.Nonce

	_, _, l = newClientForServer(s)

	err = json.Unmarshal([]byte(l[5:]), &info)
	if err != nil {
		t.Fatalf("Could not parse INFO json: %v\n", err)
	}
	if info.Nonce == "" {
		t.Fatalf("Expected a non-empty nonce")
	}
	if strings.Compare(oldNonce, info.Nonce) == 0 {
		t.Fatalf("Expected subsequent nonces to be different\n")
	}
}

func TestNkeyClientConnect(t *testing.T) {
	s, c, cr, _ := nkeyBasicSetup()
	// Send CONNECT with no signature or nkey, should fail.
	connectOp := []byte("CONNECT {\"verbose\":true,\"pedantic\":true}\r\n")
	go c.parse(connectOp)
	l, _ := cr.ReadString('\n')
	if !strings.HasPrefix(l, "-ERR ") {
		t.Fatalf("Expected an error")
	}

	kp, _ := nkeys.FromSeed(seed)
	pubKey, _ := kp.PublicKey()

	// Send nkey but no signature
	c, cr, _ = newClientForServer(s)
	cs := fmt.Sprintf("CONNECT {\"nkey\":%q, \"verbose\":true,\"pedantic\":true}\r\n", pubKey)
	connectOp = []byte(cs)
	go c.parse(connectOp)
	l, _ = cr.ReadString('\n')
	if !strings.HasPrefix(l, "-ERR ") {
		t.Fatalf("Expected an error")
	}

	// Now improperly sign etc.
	c, cr, _ = newClientForServer(s)
	cs = fmt.Sprintf("CONNECT {\"nkey\":%q,\"sig\":%q,\"verbose\":true,\"pedantic\":true}\r\n", pubKey, "bad_sig")
	go c.parse([]byte(cs))
	l, _ = cr.ReadString('\n')
	if !strings.HasPrefix(l, "-ERR ") {
		t.Fatalf("Expected an error")
	}

	// Now properly sign the nonce
	c, cr, l = newClientForServer(s)
	// Check for Nonce
	var info nonceInfo
	err := json.Unmarshal([]byte(l[5:]), &info)
	if err != nil {
		t.Fatalf("Could not parse INFO json: %v\n", err)
	}
	if info.Nonce == "" {
		t.Fatalf("Expected a non-empty nonce with nkeys defined")
	}
	sigraw, err := kp.Sign([]byte(info.Nonce))
	if err != nil {
		t.Fatalf("Failed signing nonce: %v", err)
	}
	sig := base64.StdEncoding.EncodeToString(sigraw)

	// PING needed to flush the +OK to us.
	cs = fmt.Sprintf("CONNECT {\"nkey\":%q,\"sig\":\"%s\",\"verbose\":true,\"pedantic\":true}\r\nPING\r\n", pubKey, sig)
	go c.parse([]byte(cs))
	l, _ = cr.ReadString('\n')
	if !strings.HasPrefix(l, "+OK") {
		t.Fatalf("Expected an OK, got: %v", l)
	}
}

func TestMixedClientConnect(t *testing.T) {
	s, c, cr, _ := mixedSetup()
	// Normal user/pass
	// PING needed to flush the +OK to us.
	go c.parse([]byte("CONNECT {\"user\":\"derek\",\"pass\":\"foo\",\"verbose\":true,\"pedantic\":true}\r\nPING\r\n"))
	l, _ := cr.ReadString('\n')
	if !strings.HasPrefix(l, "+OK") {
		t.Fatalf("Expected an OK, got: %v", l)
	}

	kp, _ := nkeys.FromSeed(seed)
	pubKey, _ := kp.PublicKey()

	c, cr, l = newClientForServer(s)
	// Check for Nonce
	var info nonceInfo
	err := json.Unmarshal([]byte(l[5:]), &info)
	if err != nil {
		t.Fatalf("Could not parse INFO json: %v\n", err)
	}
	if info.Nonce == "" {
		t.Fatalf("Expected a non-empty nonce with nkeys defined")
	}
	sigraw, err := kp.Sign([]byte(info.Nonce))
	if err != nil {
		t.Fatalf("Failed signing nonce: %v", err)
	}
	sig := base64.StdEncoding.EncodeToString(sigraw)

	// PING needed to flush the +OK to us.
	cs := fmt.Sprintf("CONNECT {\"nkey\":%q,\"sig\":\"%s\",\"verbose\":true,\"pedantic\":true}\r\nPING\r\n", pubKey, sig)
	go c.parse([]byte(cs))
	l, _ = cr.ReadString('\n')
	if !strings.HasPrefix(l, "+OK") {
		t.Fatalf("Expected an OK, got: %v", l)
	}
}

func TestMixedClientConfig(t *testing.T) {
	confFileName := createConfFile(t, []byte(`
    authorization {
      users = [
        {nkey: "UDKTV7HZVYJFJN64LLMYQBUR6MTNNYCDC3LAZH4VHURW3GZLL3FULBXV"}
        {user: alice, password: foo}
      ]
    }`))
	defer os.Remove(confFileName)
	opts, err := ProcessConfigFile(confFileName)
	if err != nil {
		t.Fatalf("Received an error processing config file: %v", err)
	}
	if len(opts.Nkeys) != 1 {
		t.Fatalf("Expected 1 nkey, got %d", len(opts.Nkeys))
	}
	if len(opts.Users) != 1 {
		t.Fatalf("Expected 1 user, got %d", len(opts.Users))
	}
}

func BenchmarkCryptoRandGeneration(b *testing.B) {
	data := make([]byte, 16)
	for i := 0; i < b.N; i++ {
		crand.Read(data)
	}
}

func BenchmarkMathRandGeneration(b *testing.B) {
	data := make([]byte, 16)
	prng := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	for i := 0; i < b.N; i++ {
		prng.Read(data)
	}
}

func BenchmarkNonceGeneration(b *testing.B) {
	data := make([]byte, nonceRawLen)
	b64 := make([]byte, nonceLen)
	prand := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	for i := 0; i < b.N; i++ {
		prand.Read(data)
		base64.StdEncoding.Encode(b64, data)
	}
}

func BenchmarkPublicVerify(b *testing.B) {
	data := make([]byte, nonceRawLen)
	nonce := make([]byte, nonceLen)
	mrand.Read(data)
	base64.StdEncoding.Encode(nonce, data)

	user, err := nkeys.CreateUser()
	if err != nil {
		b.Fatalf("Error creating User Nkey: %v", err)
	}
	sig, err := user.Sign(nonce)
	if err != nil {
		b.Fatalf("Error sigining nonce: %v", err)
	}
	pk, err := user.PublicKey()
	if err != nil {
		b.Fatalf("Could not extract public key from user: %v", err)
	}
	pub, err := nkeys.FromPublicKey(pk)
	if err != nil {
		b.Fatalf("Could not create public key pair from public key string: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := pub.Verify(nonce, sig); err != nil {
			b.Fatalf("Error verifying nonce: %v", err)
		}
	}
}
